package handler

import (
	"chatbridge/service"
	"encoding/json"
	"io"
	"log"
	"net/http"
)

// WhatsAppWebhookHandler handles incoming webhooks from wwebjs.
type WhatsAppWebhookHandler struct {
	Bridge *service.Bridge
}

// NewWhatsAppWebhookHandler creates a new WhatsAppWebhookHandler.
func NewWhatsAppWebhookHandler(bridge *service.Bridge) *WhatsAppWebhookHandler {
	return &WhatsAppWebhookHandler{Bridge: bridge}
}

// ServeHTTP handles POST /webhook/whatsapp requests.
func (h *WhatsAppWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[WhatsAppWebhook] Failed to read body: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse the JSON payload
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("[WhatsAppWebhook] Failed to parse JSON: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Respond immediately
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))

	// Process in background
	go h.process(payload)
}

// process handles the WhatsApp webhook payload asynchronously.
func (h *WhatsAppWebhookHandler) process(payload map[string]interface{}) {
	ctx := contextBackground()

	dataType, _ := payload["dataType"].(string)
	sessionID, _ := payload["sessionId"].(string)

	log.Printf("[WhatsAppWebhook] Received event: %s for session: %s", dataType, sessionID)

	switch dataType {
	case "ready":
		h.Bridge.SyncSessionHistory(ctx, sessionID)

	case "qr":
		h.handleQR(ctx, payload, sessionID)

	case "message_create":
		h.handleMessageCreate(ctx, payload, sessionID)

	case "message_ack":
		h.handleMessageAck(ctx, payload, sessionID)

	default:
		log.Printf("[WhatsAppWebhook] Ignoring event: %s", dataType)
	}
}

// handleQR processes QR code events.
func (h *WhatsAppWebhookHandler) handleQR(ctx contextType, payload map[string]interface{}, sessionID string) {
	data, ok := payload["data"].(map[string]interface{})
	if !ok {
		log.Printf("[WhatsAppWebhook] No data in QR payload")
		return
	}

	qr, _ := data["qr"].(string)
	if qr == "" {
		log.Printf("[WhatsAppWebhook] Empty QR data")
		return
	}

	h.Bridge.HandleWhatsAppQR(ctx, sessionID, qr)
}

// handleMessageCreate processes message_create events.
func (h *WhatsAppWebhookHandler) handleMessageCreate(ctx contextType, payload map[string]interface{}, sessionID string) {
	data, ok := payload["data"].(map[string]interface{})
	if !ok {
		log.Printf("[WhatsAppWebhook] No data in message payload")
		return
	}

	message, ok := data["message"].(map[string]interface{})
	if !ok {
		log.Printf("[WhatsAppWebhook] No message in data")
		return
	}

	// Extract _data fields
	msgData, _ := message["_data"].(map[string]interface{})
	if msgData == nil {
		msgData = message // fallback
	}

	// Extract ID
	fromMe := false
	messageID := ""
	if idData, ok := msgData["id"].(map[string]interface{}); ok {
		fromMe, _ = idData["fromMe"].(bool)
		messageID, _ = idData["id"].(string)
	}

	// Extract fields
	from, _ := msgData["from"].(string)
	to, _ := msgData["to"].(string)

	// Ignore WhatsApp status updates/broadcasts
	isStatus, _ := message["isStatus"].(bool)
	if !isStatus {
		isStatus, _ = msgData["isStatus"].(bool)
	}
	if isStatus || from == "status@broadcast" || to == "status@broadcast" {
		log.Printf("[WhatsAppWebhook] Ignoring status/broadcast message for session: %s", sessionID)
		return
	}

	notifyName, _ := msgData["notifyName"].(string)
	body, _ := msgData["body"].(string)
	msgType, _ := msgData["type"].(string)
	caption, _ := msgData["caption"].(string)
	quotedStanzaID, _ := msgData["quotedStanzaID"].(string)

	// Determine message content
	messageContent := body
	if msgType != "chat" && caption != "" {
		messageContent = caption
	}

	// Extract mentioned IDs from the top-level message object
	var mentionedIDs []string
	if mids, ok := message["mentionedIds"].([]interface{}); ok {
		for _, mid := range mids {
			if s, ok := mid.(string); ok {
				mentionedIDs = append(mentionedIDs, s)
			}
		}
	}

	h.Bridge.HandleWhatsAppMessage(
		ctx, sessionID, fromMe,
		from, to, notifyName, messageContent,
		msgType, caption, messageID, quotedStanzaID,
		mentionedIDs,
	)
}

// handleMessageAck processes message_ack events.
func (h *WhatsAppWebhookHandler) handleMessageAck(ctx contextType, payload map[string]interface{}, sessionID string) {
	data, ok := payload["data"].(map[string]interface{})
	if !ok {
		log.Printf("[WhatsAppWebhook] No data in message_ack payload")
		return
	}

	message, ok := data["message"].(map[string]interface{})
	if !ok {
		log.Printf("[WhatsAppWebhook] No message in data")
		return
	}

	// Extract _data fields
	msgData, _ := message["_data"].(map[string]interface{})
	if msgData == nil {
		msgData = message // fallback
	}

	// Extract ID
	messageID := ""
	if idData, ok := msgData["id"].(map[string]interface{}); ok {
		messageID, _ = idData["id"].(string)
	}

	// Extract ack code
	ackVal, ok := data["ack"]
	if !ok {
		ackVal, ok = msgData["ack"]
	}
	if !ok {
		log.Printf("[WhatsAppWebhook] No ack value found in payload")
		return
	}

	ack := toInt(ackVal)

	// Extract chat target (to)
	to, _ := msgData["to"].(string)

	h.Bridge.HandleWhatsAppMessageAck(ctx, sessionID, messageID, to, ack)
}

