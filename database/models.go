package database

import "time"

// Session tracks WhatsApp session configuration per Chatwoot account/inbox pair.
type Session struct {
	ID            int        `json:"id"`
	SessionID     string     `json:"session_id"`
	QR            *string    `json:"qr"`
	State         *string    `json:"state"`
	IsContactSync bool       `json:"is_contact_sync"`
	BotToken      *string    `json:"bot_token"`
	UserToken     *string    `json:"user_token"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// Contact caches WhatsApp contact info for mention resolution.
type Contact struct {
	ID          int        `json:"id"`
	WhatsAppID  string     `json:"whatsapp_id"`
	WhatsAppLID *string    `json:"whatsapp_lid"`
	Name        *string    `json:"name"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Chat maps WhatsApp chat IDs to Chatwoot conversations.
type Chat struct {
	ID              int        `json:"id"`
	WhatsAppID      string     `json:"whatsapp_id"`
	AccountID       int        `json:"account_id"`
	InboxID         int        `json:"inbox_id"`
	ConversationID  *int       `json:"conversation_id"`
	ContactID       *int       `json:"contact_id"`
	SessionID       string     `json:"session_id"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Message maps Chatwoot message IDs to WhatsApp message IDs for reply threading.
type Message struct {
	ID          int        `json:"id"`
	ChatwootID  int        `json:"chatwoot_id"`
	WhatsAppID  string     `json:"whatsapp_id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
