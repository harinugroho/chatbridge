package service

import (
	"chatbridge/config"
	"chatbridge/database"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"mime"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type PendingMessage struct {
	ChatwootID int
	Timestamp  time.Time
}

// Bridge is the core orchestration service that ties together the database,
// Chatwoot, and WhatsApp services.
type Bridge struct {
	DB           *database.DB
	Chatwoot     *ChatwootClient
	WhatsApp     *WhatsAppClient
	Config       *config.Config
	pendingMsgs  map[string][]PendingMessage
	pendingMu    sync.Mutex
	sessionLocks sync.Map // map[string]*sync.Mutex
}

// NewBridge creates a new Bridge service.
func NewBridge(db *database.DB, cw *ChatwootClient, wa *WhatsAppClient, cfg *config.Config) *Bridge {
	return &Bridge{
		DB:          db,
		Chatwoot:    cw,
		WhatsApp:    wa,
		Config:      cfg,
		pendingMsgs: make(map[string][]PendingMessage),
	}
}

// getSessionLock retrieves or creates a Mutex for a specific sessionID.
func (b *Bridge) getSessionLock(sessionID string) *sync.Mutex {
	mu, _ := b.sessionLocks.LoadOrStore(sessionID, &sync.Mutex{})
	return mu.(*sync.Mutex)
}


// cleanupOldPendingMsgs clears entries older than 2 minutes.
// Must be called while holding pendingMu lock.
func (b *Bridge) cleanupOldPendingMsgs() {
	now := time.Now()
	for key, msgs := range b.pendingMsgs {
		var active []PendingMessage
		for _, msg := range msgs {
			if now.Sub(msg.Timestamp) < 2*time.Minute {
				active = append(active, msg)
			}
		}
		if len(active) == 0 {
			delete(b.pendingMsgs, key)
		} else {
			b.pendingMsgs[key] = active
		}
	}
}

// ============================================================
// Chatwoot Webhook Handling
// ============================================================

// HandleChatwootTypingOn handles typing_on events from Chatwoot.
func (b *Bridge) HandleChatwootTypingOn(ctx context.Context, conversationID int, phoneNumber, sessionID string) {
	if phoneNumber == b.Config.SystemPhoneNumber {
		return // skip admin
	}

	var chat *database.Chat
	var err error
	if sessionID != "" {
		chat, err = b.DB.GetChatByConversationID(ctx, conversationID, sessionID)
	} else {
		chat, err = b.DB.GetChatByConversationIDOnly(ctx, conversationID)
	}
	if err != nil || chat == nil {
		log.Printf("[Bridge] Chat not found for conversation %d session %s: %v", conversationID, sessionID, err)
		return
	}

	if _, err := b.WhatsApp.SendTyping(chat.SessionID, chat.WhatsAppID); err != nil {
		log.Printf("[Bridge] Failed to send typing: %v", err)
	}
}

// HandleChatwootTypingOff handles typing_off events from Chatwoot.
func (b *Bridge) HandleChatwootTypingOff(ctx context.Context, conversationID int, phoneNumber, sessionID string) {
	if phoneNumber == b.Config.SystemPhoneNumber {
		return // skip admin
	}

	var chat *database.Chat
	var err error
	if sessionID != "" {
		chat, err = b.DB.GetChatByConversationID(ctx, conversationID, sessionID)
	} else {
		chat, err = b.DB.GetChatByConversationIDOnly(ctx, conversationID)
	}
	if err != nil || chat == nil {
		log.Printf("[Bridge] Chat not found for conversation %d session %s: %v", conversationID, sessionID, err)
		return
	}

	if _, err := b.WhatsApp.ClearState(chat.SessionID, chat.WhatsAppID); err != nil {
		log.Printf("[Bridge] Failed to clear typing: %v", err)
	}
}

// HandleChatwootMessageCreated handles message_created events from Chatwoot.
func (b *Bridge) HandleChatwootMessageCreated(ctx context.Context, accountID, inboxID, conversationID, contactID, messageID int, sessionID, phoneNumber, message, messageType string, attachments []map[string]interface{}) {
	// Only process outgoing messages (from agents)
	if messageType == "incoming" {
		log.Printf("[Bridge] Skipping incoming message %d", messageID)
		return
	}

	// Check if this message was already synced from WhatsApp (exists in database)
	msgMap, err := b.DB.GetMessageByChatwootID(ctx, messageID)
	if err == nil && msgMap != nil {
		log.Printf("[Bridge] Message %d already mapped to WhatsApp ID %s, skipping forwarding to avoid loop", messageID, msgMap.WhatsAppID)
		return
	}


	// Check if this is a system/admin message
	if phoneNumber == b.Config.SystemPhoneNumber {
		b.handleSystemCommand(ctx, accountID, inboxID, conversationID, contactID, sessionID, phoneNumber, message, messageID)
		return
	}

	// Regular outgoing message — forward to WhatsApp
	chat, err := b.DB.GetChatByConversationID(ctx, conversationID, sessionID)
	if err != nil || chat == nil {
		log.Printf("[Bridge] Chat not found for conversation %d session %s: %v", conversationID, sessionID, err)
		return
	}
	// Forward text message
	if message != "" {
		// Register pending outbox message mapping
		b.pendingMu.Lock()
		b.cleanupOldPendingMsgs()
		key := fmt.Sprintf("%s:%s:%s", chat.SessionID, chat.WhatsAppID, message)
		b.pendingMsgs[key] = append(b.pendingMsgs[key], PendingMessage{
			ChatwootID: messageID,
			Timestamp:  time.Now(),
		})
		b.pendingMu.Unlock()

		resp, err := b.WhatsApp.SendMessage(chat.SessionID, chat.WhatsAppID, "string", message)
		if err != nil {
			log.Printf("[Bridge] Failed to send message to WhatsApp: %v", err)
			// Remove on failure
			b.pendingMu.Lock()
			msgs := b.pendingMsgs[key]
			if len(msgs) > 0 {
				b.pendingMsgs[key] = msgs[:len(msgs)-1]
			}
			b.pendingMu.Unlock()
			return
		}

		// Fallback direct save if response contains the message object
		if msgData, ok := resp["message"].(map[string]interface{}); ok {
			var waID string
			if idObj, ok := msgData["id"].(map[string]interface{}); ok {
				waID, _ = idObj["id"].(string)
			}
			if waID == "" {
				if data, ok := msgData["_data"].(map[string]interface{}); ok {
					if idObj, ok := data["id"].(map[string]interface{}); ok {
						waID, _ = idObj["id"].(string)
					}
				}
			}
			if waID != "" {
				// We got it directly, so we can save it and clean up pending outbox mapping
				b.pendingMu.Lock()
				msgs := b.pendingMsgs[key]
				if len(msgs) > 0 {
					b.pendingMsgs[key] = msgs[1:]
				}
				b.pendingMu.Unlock()

				if err := b.DB.InsertMessage(ctx, messageID, waID); err != nil {
					log.Printf("[Bridge] Failed to save message mapping directly: %v", err)
				} else {
					log.Printf("[Bridge] Successfully saved message mapping directly: Chatwoot message %d -> WA message %s", messageID, waID)
				}
			}
		}
	}
	// Forward attachments
	if len(attachments) > 0 {
		for _, att := range attachments {
			dataURL, _ := att["data_url"].(string)
			if dataURL == "" {
				continue
			}
			if _, err := b.WhatsApp.SendMessage(chat.SessionID, chat.WhatsAppID, "MessageMediaFromURL", dataURL); err != nil {
				log.Printf("[Bridge] Failed to send attachment to WhatsApp: %v", err)
			}
		}
	}
}

// handleSystemCommand processes admin bot commands (init, ss, stop).
func (b *Bridge) handleSystemCommand(ctx context.Context, accountID, inboxID, conversationID, contactID int, sessionID, phoneNumber, message string, chatwootMessageID int) {
	message = strings.TrimSpace(message)

	if strings.HasPrefix(message, "init") {
		// init-<user_token>-<bot_token>
		b.handleInitCommand(ctx, accountID, inboxID, conversationID, contactID, sessionID, phoneNumber, message)
	} else if message == "ss" {
		b.handleScreenshotCommand(ctx, accountID, conversationID, sessionID)
	} else if message == "stop" {
		b.handleStopCommand(ctx, accountID, conversationID, sessionID)
	} else {
		botToken, _ := b.getTokens(ctx, sessionID)
		// Send help message
		helpMsg := "Try this helpful command:\n- init-<user token>-<bot token>: for initialize bot \n- ss: for get screenshot\n- stop: for stop bot connection"
		if _, err := b.Chatwoot.SendMessage(accountID, conversationID, helpMsg, "incoming", false, nil, botToken); err != nil {
			log.Printf("[Bridge] Failed to send help message: %v", err)
		}
	}
}

func (b *Bridge) getTokens(ctx context.Context, sessionID string) (botToken string, userToken string) {
	session, err := b.DB.GetSessionBySessionID(ctx, sessionID)
	if err != nil || session == nil {
		return "", ""
	}
	if session.BotToken != nil {
		botToken = *session.BotToken
	}
	if session.UserToken != nil {
		userToken = *session.UserToken
	}
	return botToken, userToken
}

// handleInitCommand starts a WhatsApp session and saves tokens.
func (b *Bridge) handleInitCommand(ctx context.Context, accountID, inboxID, conversationID, contactID int, sessionID, phoneNumber, message string) {
	parts := strings.SplitN(message, "-", 3)
	if len(parts) < 3 {
		botToken, _ := b.getTokens(ctx, sessionID)
		b.Chatwoot.SendMessage(accountID, conversationID, "Invalid init format. Use: init-<user_token>-<bot_token>", "incoming", false, nil, botToken)
		return
	}
	userToken := parts[1]
	botToken := parts[2]

	// Start the WA session
	resp, err := b.WhatsApp.StartSession(sessionID)
	if err != nil {
		// Parse error message
		errMsg := fmt.Sprintf("Error starting session: %v", err)
		log.Printf("[Bridge] %s", errMsg)

		// Check if session already exists
		if resp != nil {
			if msg, ok := resp["message"].(string); ok && strings.Contains(msg, "Session already exists") {
				// Check session status
				statusResp, _ := b.WhatsApp.GetSessionStatus(sessionID)
				if statusResp != nil {
					if statusMsg, ok := statusResp["message"].(string); ok && statusMsg == "session_not_connected" {
						// Session exists but not connected — that's ok
						log.Printf("[Bridge] Session %s exists but not connected", sessionID)
					}
				}
			}
			if msgErr, ok := resp["message"].(string); ok {
				b.Chatwoot.SendMessage(accountID, conversationID, msgErr, "incoming", false, nil, botToken)
			}
		}
		// Continue anyway to save tokens
	} else {
		// Success
		if msg, ok := resp["message"].(string); ok {
			b.Chatwoot.SendMessage(accountID, conversationID, msg, "incoming", false, nil, botToken)
		}
	}

	// Save session tokens
	if err := b.DB.UpsertSession(ctx, sessionID, map[string]interface{}{
		"bot_token":  botToken,
		"user_token": userToken,
	}); err != nil {
		log.Printf("[Bridge] Failed to upsert session: %v", err)
	}

	// Save chat mapping
	if err := b.DB.UpsertChat(ctx, phoneNumber, accountID, inboxID, &conversationID, &contactID, sessionID); err != nil {
		log.Printf("[Bridge] Failed to upsert chat: %v", err)
	}
}

// handleScreenshotCommand fetches a screenshot and sends it to Chatwoot.
func (b *Bridge) handleScreenshotCommand(ctx context.Context, accountID, conversationID int, sessionID string) {
	botToken, _ := b.getTokens(ctx, sessionID)
	data, ct, err := b.WhatsApp.GetScreenshot(sessionID)
	if err != nil {
		log.Printf("[Bridge] Failed to get screenshot: %v", err)
		b.Chatwoot.SendMessage(accountID, conversationID, "Failed to get screenshot", "incoming", false, nil, botToken)
		return
	}

	if _, err := b.Chatwoot.SendMessageWithAttachment(accountID, conversationID, "", "incoming", false, data, "screenshot.png", ct, nil, botToken); err != nil {
		log.Printf("[Bridge] Failed to send screenshot: %v", err)
	}
}

// handleStopCommand stops and terminates a WhatsApp session.
func (b *Bridge) handleStopCommand(ctx context.Context, accountID, conversationID int, sessionID string) {
	if _, err := b.WhatsApp.StopSession(sessionID); err != nil {
		log.Printf("[Bridge] Failed to stop session: %v", err)
	}
	if _, err := b.WhatsApp.TerminateSession(sessionID); err != nil {
		log.Printf("[Bridge] Failed to terminate session: %v", err)
	}

	botToken, _ := b.getTokens(ctx, sessionID)
	b.Chatwoot.SendMessage(accountID, conversationID, "Stop Session Success", "incoming", false, nil, botToken)
}

// ============================================================
// WhatsApp Webhook Handling
// ============================================================

// HandleWhatsAppQR handles QR code events from WhatsApp.
func (b *Bridge) HandleWhatsAppQR(ctx context.Context, sessionID, qrData string) {
	// Get admin chat for this session
	adminChat, err := b.DB.GetChatByWhatsAppIDAndSession(ctx, b.Config.SystemPhoneNumber, sessionID)
	if err != nil || adminChat == nil {
		log.Printf("[Bridge] Admin chat not found for session %s: %v", sessionID, err)
		return
	}

	// Get stored session to check QR difference
	session, err := b.DB.GetSessionBySessionID(ctx, sessionID)
	if err != nil {
		log.Printf("[Bridge] Failed to get session %s: %v", sessionID, err)
		return
	}

	// Check if QR is different from stored
	if session != nil && session.QR != nil && *session.QR == qrData {
		log.Printf("[Bridge] QR unchanged for session %s, skipping", sessionID)
		return
	}

	// Fetch QR image
	qrImage, qrCT, err := b.WhatsApp.GetQRImage(sessionID)
	if err != nil {
		log.Printf("[Bridge] Failed to get QR image: %v", err)
		return
	}

	// Send QR image to admin Chatwoot conversation
	if adminChat.ConversationID != nil {
		botToken, _ := b.getTokens(ctx, sessionID)
		if _, err := b.Chatwoot.SendMessageWithAttachment(
			adminChat.AccountID, *adminChat.ConversationID,
			"", "incoming", false,
			qrImage, "qr.png", qrCT, nil, botToken,
		); err != nil {
			log.Printf("[Bridge] Failed to send QR to Chatwoot: %v", err)
		}
	}

	// Update stored QR
	if err := b.DB.UpsertSession(ctx, sessionID, map[string]interface{}{"qr": qrData}); err != nil {
		log.Printf("[Bridge] Failed to update session QR: %v", err)
	}
}

// HandleWhatsAppMessage handles message_create events from WhatsApp with locking.
func (b *Bridge) HandleWhatsAppMessage(ctx context.Context, sessionID string, fromMe bool, from, to, name, message, msgType, caption, messageID, replyMessageID string, mentionedIDs []string) {
	mu := b.getSessionLock(sessionID)
	mu.Lock()
	defer mu.Unlock()

	b.processWhatsAppMessage(ctx, sessionID, fromMe, from, to, name, message, msgType, caption, messageID, replyMessageID, mentionedIDs)
}

// processWhatsAppMessage handles message_create events from WhatsApp.
func (b *Bridge) processWhatsAppMessage(ctx context.Context, sessionID string, fromMe bool, from, to, name, message, msgType, caption, messageID, replyMessageID string, mentionedIDs []string) {
	// Skip system/notification and status/broadcast message types
	if from == "status@broadcast" || to == "status@broadcast" {
		log.Printf("[Bridge] Ignoring status/broadcast message for session: %s", sessionID)
		return
	}

	switch msgType {
	case "e2e_notification", "gp2", "call_log", "revoked", "chatstate", "status_v3", "notification_template":
		log.Printf("[Bridge] Ignoring system/notification message type: %s for session: %s", msgType, sessionID)
		return
	}

	// Update last active time for the session
	now := time.Now()
	if err := b.DB.UpsertSession(ctx, sessionID, map[string]interface{}{"last_active_at": now}); err != nil {
		log.Printf("[Bridge] Failed to update session last active time: %v", err)
	}

	// Determine chat target
	chatTarget := from
	if fromMe {
		chatTarget = to
	}

	// Look up existing chat
	chat, err := b.DB.GetChatByWhatsAppIDAndSession(ctx, chatTarget, sessionID)
	if err != nil {
		log.Printf("[Bridge] Failed to lookup chat: %v", err)
		return
	}

	// If chat not found or no conversation_id → need to create contact + conversation
	if chat == nil || chat.ConversationID == nil || (chat.ConversationID != nil && *chat.ConversationID == 0) {
		b.handleNewWhatsAppContact(ctx, sessionID, fromMe, from, to, name, message, msgType, caption, messageID, mentionedIDs, chatTarget, chat)
		return
	}	// Skip messages from self
	if fromMe {
		b.pendingMu.Lock()
		key := fmt.Sprintf("%s:%s:%s", sessionID, to, message)
		msgs := b.pendingMsgs[key]
		if len(msgs) > 0 {
			// Get the oldest pending message matching this key
			pending := msgs[0]
			// Shift slice to remove the resolved one
			b.pendingMsgs[key] = msgs[1:]
			if len(b.pendingMsgs[key]) == 0 {
				delete(b.pendingMsgs, key)
			}
			b.pendingMu.Unlock()

			// Save the mapping
			if err := b.DB.InsertMessage(ctx, pending.ChatwootID, messageID); err != nil {
				log.Printf("[Bridge] Failed to save message mapping from outbox cache: %v", err)
			} else {
				log.Printf("[Bridge] Successfully saved message mapping from outbox cache: Chatwoot message %d -> WA message %s", pending.ChatwootID, messageID)
			}
		} else {
			b.pendingMu.Unlock()
			log.Printf("[Bridge] Skipping self-sent message (no pending outbox match for key: %s)", key)
		}
		return
	}

	// Resolve mentions in message text
	resolvedMessage := message
	if len(mentionedIDs) > 0 {
		resolvedMessage = b.resolveMentions(ctx, message, mentionedIDs, sessionID)
	}

	// For group messages, prefix with sender name
	isGroup := strings.Contains(from, "@g.us")
	if isGroup {
		resolvedMessage = fmt.Sprintf("**%s:**\n\n%s", name, resolvedMessage)
	}

	// Lookup reply message ID for threading
	var inReplyTo *int
	if replyMessageID != "" {
		if replyMsg, err := b.DB.GetMessageByWhatsAppID(ctx, replyMessageID); err == nil && replyMsg != nil {
			inReplyTo = &replyMsg.ChatwootID
		}
	}

	// Handle by message type
	switch msgType {
	case "chat":
		botToken, _ := b.getTokens(ctx, sessionID)
		// Send text message to Chatwoot
		resp, err := b.Chatwoot.SendMessage(chat.AccountID, *chat.ConversationID, resolvedMessage, "incoming", false, inReplyTo, botToken)
		if err != nil {
			log.Printf("[Bridge] Failed to send message to Chatwoot: %v", err)
			// Try recovery
			b.handleNotFoundResource(ctx, chat, sessionID, from, to, name, message, msgType, caption, messageID, mentionedIDs, fromMe)
			return
		}
		// Save message mapping
		if id, ok := resp["id"].(float64); ok {
			b.DB.InsertMessage(ctx, int(id), messageID)
		}

	case "image", "document", "ptt", "video", "sticker":
		// Download media
		mediaResp, err := b.WhatsApp.DownloadMedia(sessionID, chat.WhatsAppID, messageID)
		if err != nil {
			log.Printf("[Bridge] Failed to download media: %v", err)
			return
		}

		if len(mediaResp.Data) == 0 {
			log.Printf("[Bridge] Empty media data from WhatsApp API for message %s", messageID)
			return
		}

		mimetype := mediaResp.ContentType
		filename := mediaResp.Filename
		if mimetype == "" {
			mimetype = "application/octet-stream"
		}
		if filename == "" {
			filename = inferFilename(mimetype, msgType)
		} else if filepath.Ext(filename) == "" {
			// Add extension if filename has none
			if exts, _ := mime.ExtensionsByType(mimetype); len(exts) > 0 {
				filename += exts[0]
			}
		}

		log.Printf("[Bridge] Downloaded media: %d bytes, mimetype: %s, filename: %s", len(mediaResp.Data), mimetype, filename)

		// Determine content to send
		var content string
		if caption != "" {
			content = resolvedMessage // use the mention-resolved version
			if isGroup && caption != "" {
				content = fmt.Sprintf("**%s:**\n\n%s", name, caption)
				if len(mentionedIDs) > 0 {
					content = fmt.Sprintf("**%s:**\n\n%s", name, b.resolveMentions(ctx, caption, mentionedIDs, sessionID))
				}
			}
		}

		botToken, _ := b.getTokens(ctx, sessionID)
		// Send to Chatwoot with attachment
		resp, err := b.Chatwoot.SendMessageWithAttachment(
			chat.AccountID, *chat.ConversationID,
			content, "incoming", false,
			mediaResp.Data, filename, mimetype, inReplyTo, botToken,
		)
		if err != nil {
			log.Printf("[Bridge] Failed to send media to Chatwoot: %v", err)
			b.handleNotFoundResource(ctx, chat, sessionID, from, to, name, message, msgType, caption, messageID, mentionedIDs, fromMe)
			return
		}

		// Save message mapping
		if id, ok := resp["id"].(float64); ok {
			b.DB.InsertMessage(ctx, int(id), messageID)
		}

	default:
		log.Printf("[Bridge] Unsupported message type: %s", msgType)
	}
}

// handleNewWhatsAppContact creates a new Chatwoot contact and conversation for an unknown WhatsApp sender.
func (b *Bridge) handleNewWhatsAppContact(ctx context.Context, sessionID string, fromMe bool, from, to, name, message, msgType, caption, messageID string, mentionedIDs []string, chatTarget string, existingChat *database.Chat) {
	if fromMe {
		return // don't create contacts for outgoing messages
	}

	// Get admin chat to find account/inbox info
	adminChat, err := b.DB.GetChatByWhatsAppIDAndSession(ctx, b.Config.SystemPhoneNumber, sessionID)
	if err != nil || adminChat == nil {
		log.Printf("[Bridge] Admin chat not found for session %s", sessionID)
		return
	}

	isGroup := strings.Contains(from, "@g.us")

	// Get phone number for the contact
	var phone string
	if !isGroup {
		lidData, err := b.WhatsApp.GetContactLidAndPhone(sessionID, []string{from})
		if err == nil && len(lidData) > 0 {
			if pn, ok := lidData[0]["pn"].(string); ok {
				phone = "+" + strings.ReplaceAll(pn, "@c.us", "")
			}
		}
	}

	// Get profile picture
	picURL, _ := b.WhatsApp.GetProfilePicURL(sessionID, from)

	// Determine contact name
	contactName := name
	if isGroup {
		groupInfo, err := b.WhatsApp.GetGroupInfo(sessionID, from)
		if err == nil && groupInfo != nil {
			if chatData, ok := groupInfo["chat"].(map[string]interface{}); ok {
				if gName, ok := chatData["name"].(string); ok {
					contactName = gName + " (GROUP)"
				}
			}
		}
	}

	_, userToken := b.getTokens(ctx, sessionID)

	// Create Chatwoot contact
	contactResp, err := b.Chatwoot.CreateContact(adminChat.AccountID, contactName, phone, from, picURL, userToken)
	if err != nil {
		log.Printf("[Bridge] Failed to create contact: %v", err)
		return
	}

	// Extract contact ID
	var chatwootContactID int
	if payload, ok := contactResp["payload"].(map[string]interface{}); ok {
		if contact, ok := payload["contact"].(map[string]interface{}); ok {
			if id, ok := contact["id"].(float64); ok {
				chatwootContactID = int(id)
			}
		}
	}

	if chatwootContactID == 0 {
		log.Printf("[Bridge] Failed to extract contact ID from response")
		return
	}

	// Prepare message content
	msgContent := message
	if msgType != "chat" && caption != "" {
		msgContent = caption
	}
	if isGroup && msgContent != "" {
		msgContent = fmt.Sprintf("**%s:**\n\n%s", name, msgContent)
	}
	if msgContent == "" {
		msgContent = "[media]"
	}

	// Create Chatwoot conversation
	convResp, err := b.Chatwoot.CreateConversation(adminChat.AccountID, adminChat.InboxID, chatwootContactID, "pending", msgContent, userToken)
	if err != nil {
		log.Printf("[Bridge] Failed to create conversation: %v", err)
		return
	}

	// Extract conversation ID
	var conversationID int
	if id, ok := convResp["id"].(float64); ok {
		conversationID = int(id)
	}
	if conversationID == 0 {
		log.Printf("[Bridge] Failed to extract conversation ID from response")
		return
	}

	// Save chat mapping
	senderID := &chatwootContactID
	if err := b.DB.UpsertChat(ctx, chatTarget, adminChat.AccountID, adminChat.InboxID, &conversationID, senderID, sessionID); err != nil {
		log.Printf("[Bridge] Failed to save chat mapping: %v", err)
	}

	log.Printf("[Bridge] Created new contact=%d conversation=%d for %s", chatwootContactID, conversationID, chatTarget)
}

// ============================================================
// Mention Resolution (ported from "Get Mention Name" sub-workflow)
// ============================================================

// resolveMentions replaces LID mentions in the message text with display names.
func (b *Bridge) resolveMentions(ctx context.Context, message string, mentionedIDs []string, sessionID string) string {
	if len(mentionedIDs) == 0 {
		return message
	}

	// For each mentioned ID, look up in contacts table
	var unknownLIDs []string
	resolvedNames := make(map[string]string)

	for _, lid := range mentionedIDs {
		contact, err := b.DB.GetContactByLID(ctx, lid)
		if err == nil && contact != nil && contact.Name != nil && *contact.Name != "" {
			resolvedNames[lid] = *contact.Name
		} else {
			unknownLIDs = append(unknownLIDs, lid)
		}
	}

	// For unknown contacts, fetch from WhatsApp API and save
	if len(unknownLIDs) > 0 {
		lidData, err := b.WhatsApp.GetContactLidAndPhone(sessionID, unknownLIDs)
		if err == nil {
			for _, entry := range lidData {
				pn, _ := entry["pn"].(string)
				if pn == "" {
					continue
				}
				// Get contact class info
				classInfo, err := b.WhatsApp.GetContactClassInfo(sessionID, pn)
				if err == nil && classInfo != nil {
					if contactData, ok := classInfo["contact"].(map[string]interface{}); ok {
						contactName, _ := contactData["name"].(string)
						if idData, ok := contactData["id"].(map[string]interface{}); ok {
							serialized, _ := idData["_serialized"].(string)
							lid, _ := entry["lid"].(string)

							// Save to DB
							b.DB.UpsertContact(ctx, serialized, lid, contactName)
							// Find matching LID in our unknownLIDs
							for _, origLID := range unknownLIDs {
								if origLID == lid+"@lid" || origLID == lid {
									resolvedNames[origLID] = contactName
								}
							}
						}
					}
				}
			}
		}
	}

	// Replace LID references in message with names
	result := message
	for lid, name := range resolvedNames {
		// Replace @<number> patterns. The LID format is like "146918080536781@lid"
		// The message contains the number portion without "@lid"
		lidNumber := strings.ReplaceAll(lid, "@lid", "")
		result = strings.ReplaceAll(result, lidNumber, name)
	}

	return result
}

// ============================================================
// Error Recovery (ported from "Search Not Found Account / Chat" sub-workflow)
// ============================================================

// handleNotFoundResource checks if a Chatwoot contact/conversation still exists.
// If not, it cleans up the chat cache and retries via re-triggering the webhook.
func (b *Bridge) handleNotFoundResource(ctx context.Context, chat *database.Chat, sessionID, from, to, name, message, msgType, caption, messageID string, mentionedIDs []string, fromMe bool) {
	if chat == nil || chat.ConversationID == nil || chat.ContactID == nil {
		return
	}

	log.Printf("[Bridge] Checking for stale resources: contact=%d conversation=%d", *chat.ContactID, *chat.ConversationID)

	_, userToken := b.getTokens(ctx, sessionID)

	// Check if contact still exists
	_, err := b.Chatwoot.GetContact(chat.AccountID, *chat.ContactID, userToken)
	if err != nil {
		// Contact not found → delete chat cache
		log.Printf("[Bridge] Contact %d not found, deleting chat cache", *chat.ContactID)
		b.DB.DeleteChatByContactAndSession(ctx, *chat.ContactID, sessionID)

		// Re-process the message as a new contact
		chatTarget := from
		if fromMe {
			chatTarget = to
		}
		b.handleNewWhatsAppContact(ctx, sessionID, fromMe, from, to, name, message, msgType, caption, messageID, mentionedIDs, chatTarget, nil)
		return
	}

	// Check if conversation still exists
	_, err = b.Chatwoot.GetConversation(chat.AccountID, *chat.ConversationID, userToken)
	if err != nil {
		// Conversation not found → reset conversation_id
		log.Printf("[Bridge] Conversation %d not found, resetting", *chat.ConversationID)
		b.DB.ResetChatConversationID(ctx, sessionID, *chat.ConversationID)

		// Re-process the message
		chatTarget := from
		if fromMe {
			chatTarget = to
		}
		b.handleNewWhatsAppContact(ctx, sessionID, fromMe, from, to, name, message, msgType, caption, messageID, mentionedIDs, chatTarget, nil)
	}
}

// ============================================================
// Helpers
// ============================================================

// decodeBase64Media decodes a base64-encoded media string, handling the
// "data:mimetype;base64," prefix if present.
// It tries StdEncoding first, then falls back to RawStdEncoding (no padding)
// to handle both padded and unpadded base64 from wwebjs.
func decodeBase64Media(data string) ([]byte, error) {
	rawData := data
	// Strip data URI prefix if present (e.g., "data:image/jpeg;base64,...")
	if idx := strings.Index(data, ","); idx != -1 {
		rawData = data[idx+1:]
	}

	// Remove any whitespace/newlines that may be present
	rawData = strings.TrimSpace(rawData)

	if rawData == "" {
		return nil, fmt.Errorf("empty base64 data after stripping prefix")
	}

	// Try standard base64 (with padding) first
	decoded, err := base64.StdEncoding.DecodeString(rawData)
	if err == nil {
		return decoded, nil
	}

	// Fall back to raw standard base64 (without padding)
	decoded, err2 := base64.RawStdEncoding.DecodeString(rawData)
	if err2 == nil {
		return decoded, nil
	}

	// Try URL-safe base64 variants
	decoded, err3 := base64.URLEncoding.DecodeString(rawData)
	if err3 == nil {
		return decoded, nil
	}

	decoded, err4 := base64.RawURLEncoding.DecodeString(rawData)
	if err4 == nil {
		return decoded, nil
	}

	return nil, fmt.Errorf("all base64 decode attempts failed: std=%v, rawStd=%v, url=%v, rawUrl=%v", err, err2, err3, err4)
}

// inferFilename generates a filename with proper extension based on mimetype and message type.
func inferFilename(mimetype, msgType string) string {
	base := "attachment"
	switch msgType {
	case "image":
		base = "image"
	case "video":
		base = "video"
	case "ptt":
		base = "voice"
	case "sticker":
		base = "sticker"
	case "document":
		base = "document"
	}

	// Try to get extension from mimetype
	if exts, _ := mime.ExtensionsByType(mimetype); len(exts) > 0 {
		return base + exts[0]
	}

	// Fallback extensions
	switch {
	case strings.HasPrefix(mimetype, "image/"):
		return base + ".jpg"
	case strings.HasPrefix(mimetype, "video/"):
		return base + ".mp4"
	case strings.HasPrefix(mimetype, "audio/"):
		return base + ".ogg"
	default:
		return base + ".bin"
	}
}

// HandleWhatsAppMessageAck handles message_ack events from WhatsApp.
func (b *Bridge) HandleWhatsAppMessageAck(ctx context.Context, sessionID, waMessageID, to string, ack int) {
	// Only care about delivered (2) and read (3) statuses
	var status string
	switch ack {
	case 2:
		status = "delivered"
	case 3:
		status = "read"
	default:
		return // ignore other status codes
	}

	// Lookup message mapping in database
	msg, err := b.DB.GetMessageByWhatsAppID(ctx, waMessageID)
	if err != nil || msg == nil {
		// Log but don't error out — could be a status for an incoming message or unsaved message
		return
	}

	// Lookup chat to get AccountID and ConversationID
	chat, err := b.DB.GetChatByWhatsAppIDAndSession(ctx, to, sessionID)
	if err != nil || chat == nil || chat.ConversationID == nil {
		log.Printf("[Bridge] Chat not found for ack mapping target %s session %s", to, sessionID)
		return
	}	// Update message status in Chatwoot
	_, userToken := b.getTokens(ctx, sessionID)
	if userToken == "" {
		log.Printf("[Bridge] No user token found for session %s, skipping status update", sessionID)
		return
	}

	_, err = b.Chatwoot.UpdateMessageStatus(chat.AccountID, *chat.ConversationID, msg.ChatwootID, status, userToken)
	if err != nil {
		log.Printf("[Bridge] Failed to update message status in Chatwoot: %v", err)
	} else {
		log.Printf("[Bridge] Updated message %d (WA: %s) status to %s", msg.ChatwootID, waMessageID, status)
	}
}

// HandleChatwootConversationRead handles conversation_read events (inferred from unread_count turning 0) from Chatwoot.
func (b *Bridge) HandleChatwootConversationRead(ctx context.Context, conversationID int, phoneNumber, sessionID string) {
	// Lookup chat to get SessionID and WhatsAppID
	chat, err := b.DB.GetChatByConversationID(ctx, conversationID, sessionID)
	if err != nil || chat == nil {
		log.Printf("[Bridge] Chat not found for conversation %d session %s: %v", conversationID, sessionID, err)
		return
	}

	// Send seen status to WhatsApp
	resp, err := b.WhatsApp.SendSeen(chat.SessionID, chat.WhatsAppID)
	if err != nil {
		log.Printf("[Bridge] Failed to send seen status to WhatsApp for chat %s (session %s): %v", chat.WhatsAppID, chat.SessionID, err)
		return
	}

	log.Printf("[Bridge] Successfully sent seen status to WhatsApp for chat %s (session %s): %v", chat.WhatsAppID, chat.SessionID, resp)
}

// SyncSessionHistory runs catch-up sync for the given session.
func (b *Bridge) SyncSessionHistory(ctx context.Context, sessionID string) {
	mu := b.getSessionLock(sessionID)
	mu.Lock()
	defer mu.Unlock()

	log.Printf("[Bridge] Starting Catch-up Sync for session %s", sessionID)

	session, err := b.DB.GetSessionBySessionID(ctx, sessionID)
	if err != nil {
		log.Printf("[Bridge] Failed to retrieve session %s for sync: %v", sessionID, err)
		return
	}
	if session == nil {
		log.Printf("[Bridge] Session %s not found in DB, skipping sync", sessionID)
		return
	}

	// Calculate boundaries
	now := time.Now()
	var startTime time.Time
	if session.LastActiveAt != nil {
		startTime = *session.LastActiveAt
	} else {
		// Fallback to 24 hours ago
		startTime = now.Add(-24 * time.Hour)
		log.Printf("[Bridge] LastActiveAt is nil, falling back to 24 hours ago: %v", startTime)
	}

	// Cap the lookback to a maximum of 7 days
	sevenDaysAgo := now.Add(-7 * 24 * time.Hour)
	if startTime.Before(sevenDaysAgo) {
		startTime = sevenDaysAgo
		log.Printf("[Bridge] Capping lookback time to 7 days ago: %v", startTime)
	}

	// Fetch chats
	resp, err := b.WhatsApp.GetChats(sessionID)
	if err != nil {
		log.Printf("[Bridge] Failed to fetch chats from WhatsApp for session %s: %v", sessionID, err)
		return
	}

	success, _ := resp["success"].(bool)
	if !success {
		log.Printf("[Bridge] GetChats failed for session %s: %v", sessionID, resp["error"])
		return
	}

	chatsList, ok := resp["chats"].([]interface{})
	if !ok {
		log.Printf("[Bridge] Invalid chats format in response: %v", resp)
		return
	}

	log.Printf("[Bridge] Found %d chats to check for session %s", len(chatsList), sessionID)

	for _, cVal := range chatsList {
		chatMap, ok := cVal.(map[string]interface{})
		if !ok {
			continue
		}

		// Check timestamp
		tsFloat, _ := chatMap["timestamp"].(float64)
		chatTimestamp := time.Unix(int64(tsFloat), 0)

		// Skip if last message is older than startTime
		if chatTimestamp.Before(startTime) {
			continue
		}

		idObj, _ := chatMap["id"].(map[string]interface{})
		if idObj == nil {
			continue
		}
		chatID, _ := idObj["_serialized"].(string)
		if chatID == "" {
			continue
		}

		log.Printf("[Bridge] Chat %s has activity after %v (last message at %v). Fetching history...", chatID, startTime, chatTimestamp)

		// Fetch messages for this chat
		msgResp, err := b.WhatsApp.FetchMessages(sessionID, chatID, 100)
		if err != nil {
			log.Printf("[Bridge] Failed to fetch messages for chat %s: %v", chatID, err)
			continue
		}

		msgSuccess, _ := msgResp["success"].(bool)
		if !msgSuccess {
			log.Printf("[Bridge] FetchMessages failed for chat %s: %v", chatID, msgResp["error"])
			continue
		}

		messagesList, ok := msgResp["messages"].([]interface{})
		if !ok {
			continue
		}

		for _, mVal := range messagesList {
			message, ok := mVal.(map[string]interface{})
			if !ok {
				continue
			}

			// Extract message fields
			msgData, _ := message["_data"].(map[string]interface{})
			if msgData == nil {
				msgData = message // fallback
			}

			tsVal, _ := msgData["timestamp"].(float64)
			msgTimestamp := time.Unix(int64(tsVal), 0)

			// Skip if message was sent before start time
			if msgTimestamp.Before(startTime) {
				continue
			}

			// Extract ID
			messageID := ""
			fromMe := false
			if idData, ok := msgData["id"].(map[string]interface{}); ok {
				fromMe, _ = idData["fromMe"].(bool)
				messageID, _ = idData["id"].(string)
			}

			if messageID == "" {
				continue
			}

			// Check database for duplicates
			existing, err := b.DB.GetMessageByWhatsAppID(ctx, messageID)
			if err == nil && existing != nil {
				// Already processed, skip
				continue
			}

			log.Printf("[Bridge] Syncing missed message %s from chat %s, timestamp: %v", messageID, chatID, msgTimestamp)

			// Extract other fields
			from, _ := msgData["from"].(string)
			to, _ := msgData["to"].(string)

			// Ignore status updates
			isStatus, _ := message["isStatus"].(bool)
			if !isStatus {
				isStatus, _ = msgData["isStatus"].(bool)
			}
			if isStatus || from == "status@broadcast" || to == "status@broadcast" {
				continue
			}

			notifyName, _ := msgData["notifyName"].(string)
			body, _ := msgData["body"].(string)
			msgType, _ := msgData["type"].(string)
			caption, _ := msgData["caption"].(string)
			quotedStanzaID, _ := msgData["quotedStanzaID"].(string)

			messageContent := body
			if msgType != "chat" && caption != "" {
				messageContent = caption
			}

			var mentionedIDs []string
			if mids, ok := message["mentionedIds"].([]interface{}); ok {
				for _, mid := range mids {
					if s, ok := mid.(string); ok {
						mentionedIDs = append(mentionedIDs, s)
					}
				}
			}

			// Process the message (without lock)
			b.processWhatsAppMessage(
				ctx, sessionID, fromMe,
				from, to, notifyName, messageContent,
				msgType, caption, messageID, quotedStanzaID,
				mentionedIDs,
			)
		}
	}

	// Update last_active_at to current time
	if err := b.DB.UpsertSession(ctx, sessionID, map[string]interface{}{"last_active_at": now}); err != nil {
		log.Printf("[Bridge] Failed to update last active timestamp at end of sync: %v", err)
	}

	log.Printf("[Bridge] Catch-up Sync completed for session %s", sessionID)
}
