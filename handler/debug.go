package handler

import (
	"chatbridge/logger"
	"embed"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

//go:embed debug.html
var debugTemplates embed.FS

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow WebSocket connection from any origin for debugging convenience.
		// TODO(security): If deploying to production, secure this by matching domain.
		return true
	},
}

// ServeDebugPage handles requests for the live log dashboard
func ServeDebugPage(w http.ResponseWriter, r *http.Request) {
	data, err := debugTemplates.ReadFile("debug.html")
	if err != nil {
		http.Error(w, "Live Debug template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// ServeWebSocket upgrades the connection to WebSocket and streams the logs
func ServeWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WebSocket] Upgrade error: %v", err)
		return
	}

	hub := logger.GetHub()
	hub.Register(conn)

	// Keep-alive read loop to detect client disconnections
	defer func() {
		hub.Unregister(conn)
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}
