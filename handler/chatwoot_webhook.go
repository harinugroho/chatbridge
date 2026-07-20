package handler

import (
	"chatbridge/service"
	"encoding/json"
	"io"
	"log"
	"net/http"
)

// ChatwootWebhookHandler handles incoming webhooks from Chatwoot.
type ChatwootWebhookHandler struct {
	Bridge *service.Bridge
}

// NewChatwootWebhookHandler creates a new ChatwootWebhookHandler.
func NewChatwootWebhookHandler(bridge *service.Bridge) *ChatwootWebhookHandler {
	return &ChatwootWebhookHandler{Bridge: bridge}
}

// ServeHTTP handles POST /webhook/chatwoot requests.
func (h *ChatwootWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[ChatwootWebhook] Failed to read body: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse the JSON payload
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("[ChatwootWebhook] Failed to parse JSON: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Respond immediately
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))

	// Process in background
	go h.process(payload)
}

// process handles the Chatwoot webhook payload asynchronously.
func (h *ChatwootWebhookHandler) process(payload map[string]interface{}) {
	ctx := contextBackground()

	event, _ := payload["event"].(string)

	log.Printf("[ChatwootWebhook] Received event: %s", event)

	// Extract conversation data
	conversation, _ := payload["conversation"].(map[string]interface{})
	if conversation == nil {
		log.Printf("[ChatwootWebhook] No conversation in payload")
		return
	}

	conversationID := toInt(conversation["id"])
	inboxID := toInt(conversation["inbox_id"])

	// Extract phone number from meta.sender
	phoneNumber := ""
	if meta, ok := conversation["meta"].(map[string]interface{}); ok {
		if sender, ok := meta["sender"].(map[string]interface{}); ok {
			phoneNumber, _ = sender["phone_number"].(string)
		}
	}

	// Extract account
	account, _ := payload["account"].(map[string]interface{})
	accountID := 0
	if account != nil {
		accountID = toInt(account["id"])
	}

	// Build session_id
	sessionID := ""
	if accountID > 0 && inboxID > 0 {
		sessionID = buildSessionID(accountID, inboxID)
	}

	switch event {
	case "conversation_typing_on":
		h.Bridge.HandleChatwootTypingOn(ctx, conversationID, phoneNumber, sessionID)

	case "conversation_typing_off":
		h.Bridge.HandleChatwootTypingOff(ctx, conversationID, phoneNumber, sessionID)

	case "message_created":
		// Extract message details
		messageType, _ := payload["message_type"].(string)
		content, _ := payload["content"].(string)
		messageID := toInt(payload["id"])

		// Contact ID from conversation.contact_inbox.contact_id
		contactID := 0
		if contactInbox, ok := conversation["contact_inbox"].(map[string]interface{}); ok {
			contactID = toInt(contactInbox["contact_id"])
		}

		// If the message is outgoing and the message body is empty,
		// also check conversation.messages for processed_message_content
		if content == "" {
			if messages, ok := conversation["messages"].([]interface{}); ok && len(messages) > 0 {
				for _, m := range messages {
					msg, ok := m.(map[string]interface{})
					if !ok {
						continue
					}
					if toInt(msg["id"]) == messageID {
						if pmc, ok := msg["processed_message_content"].(string); ok {
							content = pmc
						}
						break
					}
				}
			}
		}

		// Extract attachments
		var attachments []map[string]interface{}
		if atts, ok := payload["attachments"].([]interface{}); ok {
			for _, a := range atts {
				if att, ok := a.(map[string]interface{}); ok {
					attachments = append(attachments, att)
				}
			}
		}

		h.Bridge.HandleChatwootMessageCreated(
			ctx, accountID, inboxID, conversationID, contactID, messageID,
			sessionID, phoneNumber, content, messageType, attachments,
		)

	default:
		log.Printf("[ChatwootWebhook] Ignoring event: %s", event)
	}
}
