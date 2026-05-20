package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`      // "APP", "WEBHOOK_IN", "API_OUT"
	Method    string    `json:"method"`    // "GET", "POST", "INFO", "ERROR"
	Path      string    `json:"path"`      // endpoint path or URL
	Status    string    `json:"status"`    // "200 OK", etc.
	Message   string    `json:"message"`   // short summary
	Details   string    `json:"details"`   // expanded details (headers, payload, etc.)
	IsError   bool      `json:"is_error"`
}

type Hub struct {
	mu         sync.RWMutex
	logs       []LogEntry
	maxLogs    int
	clients    map[*websocket.Conn]bool
	broadcast  chan LogEntry
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
}

var globalHub *Hub
var once sync.Once

// GetHub returns the singleton Hub instance
func GetHub() *Hub {
	once.Do(func() {
		globalHub = &Hub{
			logs:       make([]LogEntry, 0, 500),
			maxLogs:    500,
			clients:    make(map[*websocket.Conn]bool),
			broadcast:  make(chan LogEntry, 100),
			register:   make(chan *websocket.Conn),
			unregister: make(chan *websocket.Conn),
		}
		go globalHub.run()
	})
	return globalHub
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			// Send existing logs
			for _, entry := range h.logs {
				data, err := json.Marshal(entry)
				if err == nil {
					_ = client.WriteMessage(websocket.TextMessage, data)
				}
			}
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.Close()
			}
			h.mu.Unlock()
		case entry := <-h.broadcast:
			h.mu.Lock()
			// Append to buffer
			if len(h.logs) >= h.maxLogs {
				h.logs = h.logs[1:]
			}
			h.logs = append(h.logs, entry)

			// Broadcast to all clients
			data, err := json.Marshal(entry)
			if err == nil {
				for client := range h.clients {
					err := client.WriteMessage(websocket.TextMessage, data)
					if err != nil {
						client.Close()
						delete(h.clients, client)
					}
				}
			}
			h.mu.Unlock()
		}
	}
}

func (h *Hub) Register(conn *websocket.Conn) {
	h.register <- conn
}

func (h *Hub) Unregister(conn *websocket.Conn) {
	h.unregister <- conn
}

// AddEntry adds a new log entry to the hub
func (h *Hub) AddEntry(entry LogEntry) {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	h.broadcast <- entry
}

// CustomWriter routes standard logs to the Hub
type CustomWriter struct {
	Fallback io.Writer
}

func (cw *CustomWriter) Write(p []byte) (n int, err error) {
	// Print to fallback (usually os.Stderr)
	n, err = cw.Fallback.Write(p)

	// Clean the trailing newline and register to Hub
	msg := string(p)
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}

	GetHub().AddEntry(LogEntry{
		Timestamp: time.Now(),
		Type:      "APP",
		Method:    "INFO",
		Message:   msg,
		Details:   msg,
		IsError:   false,
	})

	return n, err
}

// InitLogging sets up the global log capture
func InitLogging() {
	GetHub()
	// Set log output to our CustomWriter wrapping os.Stderr
	log.SetOutput(&CustomWriter{Fallback: os.Stderr})
	log.Println("[Logger] Custom logging system initialized")
}

// LogWebhook structured log for inbound webhook calls
func LogWebhook(method, path, remoteAddr string, headers http.Header, reqBody []byte, statusCode int, respBody []byte, duration time.Duration) {
	now := time.Now()
	statusText := http.StatusText(statusCode)
	statusStr := fmt.Sprintf("%d %s", statusCode, statusText)
	isError := statusCode >= 400

	// Format multi-line logs for console/docker logs
	consoleMsg := fmt.Sprintf("\n--- [HTTP Webhook Request Received] ---\n"+
		"Method:      %s\n"+
		"Path:        %s\n"+
		"RemoteAddr:  %s\n"+
		"Headers:     %v\n"+
		"Payload:\n%s\n"+
		"--- [HTTP Webhook Response Sent] ---\n"+
		"Status:      %s\n"+
		"Payload:\n%s\n"+
		"Duration:    %v\n"+
		"------------------------------------",
		method, path, remoteAddr, headers, string(reqBody), statusStr, string(respBody), duration)

	// Print directly to os.Stderr (so CustomWriter doesn't intercept it and duplicate it)
	timeStr := now.Format("2006/01/02 15:04:05")
	fmt.Fprintf(os.Stderr, "%s %s\n", timeStr, consoleMsg)

	// Build structured Details JSON for web UI
	detailsMap := map[string]interface{}{
		"request": map[string]interface{}{
			"method":      method,
			"path":        path,
			"remote_addr": remoteAddr,
			"headers":     headers,
			"payload":     safeJSON(reqBody),
		},
		"response": map[string]interface{}{
			"status":   statusStr,
			"duration": duration.String(),
			"payload":  safeJSON(respBody),
		},
	}

	detailsJSON, _ := json.MarshalIndent(detailsMap, "", "  ")

	// Add to WebSocket Hub
	GetHub().AddEntry(LogEntry{
		Timestamp: now,
		Type:      "WEBHOOK_IN",
		Method:    method,
		Path:      path,
		Status:    statusStr,
		Message:   fmt.Sprintf("Webhook received from %s", remoteAddr),
		Details:   string(detailsJSON),
		IsError:   isError,
	})
}

// LogClientRequest structured log for outbound API calls
func LogClientRequest(name, method, urlStr string, headers http.Header, reqBody []byte, statusCode int, respHeaders http.Header, respBody []byte, duration time.Duration, err error) {
	now := time.Now()
	var statusStr string
	isError := err != nil

	if err != nil {
		statusStr = "ERROR"
	} else {
		statusStr = fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode))
		if statusCode >= 400 {
			isError = true
		}
	}

	// Format console message
	var consoleMsg string
	if err != nil {
		consoleMsg = fmt.Sprintf("\n--- [Outgoing HTTP Request Failure (%s)] ---\n"+
			"Method:      %s\n"+
			"URL:         %s\n"+
			"Headers:     %v\n"+
			"Payload:\n%s\n"+
			"Error:       %v\n"+
			"Duration:    %v\n"+
			"-------------------------------------------",
			name, method, urlStr, headers, string(reqBody), err, duration)
	} else {
		// Prevent logging large binary/media response bodies (same as original code)
		displayRespBody := string(respBody)
		contentType := respHeaders.Get("Content-Type")
		if len(respBody) > 2000 || (contentType != "" && !containsJSONOrText(contentType)) {
			displayRespBody = fmt.Sprintf("<Binary or large response payload of %d bytes, Content-Type: %s>", len(respBody), contentType)
		}

		// Prevent logging large binary/media request bodies (same as original code)
		displayReqBody := string(reqBody)
		reqContentType := headers.Get("Content-Type")
		if len(reqBody) > 2000 || (reqContentType != "" && containsMultipart(reqContentType)) {
			displayReqBody = fmt.Sprintf("<Multipart or large request payload of %d bytes, Content-Type: %s>", len(reqBody), reqContentType)
		}

		consoleMsg = fmt.Sprintf("\n--- [Outgoing HTTP Request Sent (%s)] ---\n"+
			"Method:      %s\n"+
			"URL:         %s\n"+
			"Headers:     %v\n"+
			"Payload:\n%s\n"+
			"--- [Outgoing HTTP Response Received (%s)] ---\n"+
			"Status:      %s\n"+
			"Headers:     %v\n"+
			"Payload:\n%s\n"+
			"Duration:    %v\n"+
			"---------------------------------------------",
			name, method, urlStr, headers, displayReqBody,
			name, statusStr, respHeaders, displayRespBody,
			duration)
	}

	// Print directly to os.Stderr
	timeStr := now.Format("2006/01/02 15:04:05")
	fmt.Fprintf(os.Stderr, "%s %s\n", timeStr, consoleMsg)

	// Build structured Details JSON for web UI
	reqPayload := safeJSON(reqBody)
	var respPayload interface{}
	if err == nil {
		respPayload = safeJSON(respBody)
	} else {
		respPayload = err.Error()
	}

	detailsMap := map[string]interface{}{
		"client": name,
		"request": map[string]interface{}{
			"method":  method,
			"url":     urlStr,
			"headers": headers,
			"payload": reqPayload,
		},
		"response": map[string]interface{}{
			"status":   statusStr,
			"headers":  respHeaders,
			"payload":  respPayload,
			"duration": duration.String(),
		},
	}

	detailsJSON, _ := json.MarshalIndent(detailsMap, "", "  ")

	// Add to WebSocket Hub
	GetHub().AddEntry(LogEntry{
		Timestamp: now,
		Type:      "API_OUT",
		Method:    method,
		Path:      urlStr,
		Status:    statusStr,
		Message:   fmt.Sprintf("API Request to %s", name),
		Details:   string(detailsJSON),
		IsError:   isError,
	})
}

func containsJSONOrText(contentType string) bool {
	for _, s := range []string{"json", "text", "plain", "xml", "html"} {
		if containsSubstring(contentType, s) {
			return true
		}
	}
	return false
}

func containsMultipart(contentType string) bool {
	return containsSubstring(contentType, "multipart")
}

func containsSubstring(s, sub string) bool {
	// Simple case-insensitive substring search
	lenS, lenSub := len(s), len(sub)
	if lenSub == 0 {
		return true
	}
	if lenS < lenSub {
		return false
	}
	for i := 0; i <= lenS-lenSub; i++ {
		match := true
		for j := 0; j < lenSub; j++ {
			c1 := s[i+j]
			c2 := sub[j]
			if c1 >= 'A' && c1 <= 'Z' {
				c1 = c1 + 32
			}
			if c2 >= 'A' && c2 <= 'Z' {
				c2 = c2 + 32
			}
			if c1 != c2 {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// safeJSON attempts to unmarshal bytes as JSON to avoid double escaping in UI
func safeJSON(b []byte) interface{} {
	if len(b) == 0 {
		return ""
	}
	var js interface{}
	if err := json.Unmarshal(b, &js); err == nil {
		return js
	}
	// Return string representation if not valid JSON
	if len(b) > 2000 {
		return fmt.Sprintf("<payload too large: %d bytes>", len(b))
	}
	return string(b)
}
