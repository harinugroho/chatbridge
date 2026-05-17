package model

import "encoding/json"

// ============================================================
// WhatsApp (wwebjs) Webhook Payload Types
// ============================================================

// WhatsAppWebhookPayload is the top-level payload from a wwebjs webhook.
type WhatsAppWebhookPayload struct {
	DataType  string                `json:"dataType"`  // "qr", "message_create", etc.
	Data      json.RawMessage       `json:"data"`      // parsed based on DataType
	SessionID string                `json:"sessionId"`
}

// WhatsAppQRData is the data payload for a QR event.
type WhatsAppQRData struct {
	QR string `json:"qr"`
}

// WhatsAppMessageCreateData is the data payload for a message_create event.
type WhatsAppMessageCreateData struct {
	Message WhatsAppMessage `json:"message"`
}

// WhatsAppMessage represents a wwebjs message.
type WhatsAppMessage struct {
	Data          WhatsAppMessageData `json:"_data"`
	ID            WhatsAppMessageID   `json:"id"`
	Ack           int                 `json:"ack"`
	HasMedia      bool                `json:"hasMedia"`
	Body          string              `json:"body"`
	Type          string              `json:"type"` // "chat", "image", "document", "ptt", "video", "sticker"
	Timestamp     int64               `json:"timestamp"`
	From          string              `json:"from"`
	To            string              `json:"to"`
	DeviceType    string              `json:"deviceType"`
	IsForwarded   bool                `json:"isForwarded"`
	IsStatus      bool                `json:"isStatus"`
	IsStarred     bool                `json:"isStarred"`
	FromMe        bool                `json:"fromMe"`
	HasQuotedMsg  bool                `json:"hasQuotedMsg"`
	HasReaction   bool                `json:"hasReaction"`
	VCards        []interface{}       `json:"vCards"`
	MentionedIds  []string            `json:"mentionedIds"`
	GroupMentions []interface{}       `json:"groupMentions"`
	IsGif         bool                `json:"isGif"`
}

// WhatsAppMessageData is the _data field inside a message.
type WhatsAppMessageData struct {
	ID              WhatsAppMessageID `json:"id"`
	Body            string            `json:"body"`
	Type            string            `json:"type"`
	From            string            `json:"from"`
	To              string            `json:"to"`
	NotifyName      string            `json:"notifyName"`
	Caption         string            `json:"caption"`
	QuotedStanzaID  string            `json:"quotedStanzaID"`
	MentionedJidList []string         `json:"mentionedJidList"`
}

// WhatsAppMessageID represents a wwebjs message ID.
type WhatsAppMessageID struct {
	FromMe      bool   `json:"fromMe"`
	Remote      string `json:"remote"`
	ID          string `json:"id"`
	Serialized  string `json:"_serialized"`
	Participant *WhatsAppParticipant `json:"participant,omitempty"`
}

// WhatsAppParticipant is used in group message IDs.
type WhatsAppParticipant struct {
	Server     string `json:"server"`
	User       string `json:"user"`
	Serialized string `json:"_serialized"`
}

// ============================================================
// WhatsApp (wwebjs) API Response Types
// ============================================================

// WhatsAppSessionResponse is a generic session API response.
type WhatsAppSessionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// WhatsAppStatusResponse is the response from session status.
type WhatsAppStatusResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// WhatsAppSendMessageResponse is the response from sending a message.
type WhatsAppSendMessageResponse struct {
	Success bool                     `json:"success"`
	Message *WhatsAppSentMessageInfo `json:"message,omitempty"`
}

// WhatsAppSentMessageInfo contains info about a sent message.
type WhatsAppSentMessageInfo struct {
	Data WhatsAppMessageData `json:"_data"`
	ID   WhatsAppMessageID   `json:"id"`
}

// WhatsAppContactLidResponse is the response from getContactLidAndPhone.
type WhatsAppContactLidResponse struct {
	Success bool                        `json:"success"`
	Data    []WhatsAppContactLidEntry   `json:"data"`
}

// WhatsAppContactLidEntry is one entry in the getContactLidAndPhone response.
type WhatsAppContactLidEntry struct {
	PN  string `json:"pn"`  // phone number in @c.us format
	LID string `json:"lid"` // LID identifier
}

// WhatsAppContactClassInfoResponse is the response from contact/getClassInfo.
type WhatsAppContactClassInfoResponse struct {
	Success bool `json:"success"`
	Contact struct {
		ID struct {
			Serialized string `json:"_serialized"`
		} `json:"id"`
		Name string `json:"name"`
	} `json:"contact"`
}

// WhatsAppGroupInfoResponse is the response from groupChat/getClassInfo.
type WhatsAppGroupInfoResponse struct {
	Success bool `json:"success"`
	Chat    struct {
		Name string `json:"name"`
	} `json:"chat"`
}

// WhatsAppProfilePicResponse is the response from getProfilePicUrl.
type WhatsAppProfilePicResponse struct {
	Success bool   `json:"success"`
	Result  string `json:"result"`
}

// WhatsAppDownloadMediaResponse is the response from downloadMediaAsData.
type WhatsAppDownloadMediaResponse struct {
	Success  bool   `json:"success"`
	Mimetype string `json:"mimetype"`
	Data     string `json:"data"` // base64-encoded data
	Filename string `json:"filename"`
}
