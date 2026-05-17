package model

// ============================================================
// Chatwoot Webhook Payload Types
// ============================================================

// ChatwootWebhookPayload is the top-level payload from a Chatwoot webhook.
type ChatwootWebhookPayload struct {
	Event            string                `json:"event"`
	Account          ChatwootAccount       `json:"account"`
	Conversation     ChatwootConversation  `json:"conversation"`
	MessageType      string                `json:"message_type"` // "incoming" or "outgoing"
	Content          string                `json:"content"`
	ContentType      string                `json:"content_type"`
	Attachments      []ChatwootAttachment  `json:"attachments"`
	ID               int                   `json:"id"`   // message id
	Sender           *ChatwootSender       `json:"sender"`
	CreatedAt        interface{}           `json:"created_at"`
	Private          bool                  `json:"private"`
	SourceID         *string               `json:"source_id"`
	Inbox            *ChatwootInbox        `json:"inbox"`
	ContentAttributes map[string]interface{} `json:"content_attributes"`
	AdditionalAttributes map[string]interface{} `json:"additional_attributes"`
}

// ChatwootAccount represents a Chatwoot account.
type ChatwootAccount struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ChatwootConversation represents a Chatwoot conversation in the webhook payload.
type ChatwootConversation struct {
	ID                   int                    `json:"id"`
	InboxID              int                    `json:"inbox_id"`
	ContactInbox         *ChatwootContactInbox  `json:"contact_inbox"`
	Messages             []ChatwootMessage      `json:"messages"`
	Meta                 *ChatwootMeta          `json:"meta"`
	Labels               []string               `json:"labels"`
	Status               string                 `json:"status"`
	AdditionalAttributes map[string]interface{} `json:"additional_attributes"`
	CustomAttributes     map[string]interface{} `json:"custom_attributes"`
	Channel              string                 `json:"channel"`
}

// ChatwootContactInbox represents the contact_inbox in the conversation.
type ChatwootContactInbox struct {
	ID        int    `json:"id"`
	ContactID int    `json:"contact_id"`
	InboxID   int    `json:"inbox_id"`
	SourceID  string `json:"source_id"`
}

// ChatwootMessage represents a message in the conversation.
type ChatwootMessage struct {
	ID                      int                    `json:"id"`
	Content                 string                 `json:"content"`
	AccountID               int                    `json:"account_id"`
	InboxID                 int                    `json:"inbox_id"`
	ConversationID          int                    `json:"conversation_id"`
	MessageType             int                    `json:"message_type"` // 0=incoming, 1=outgoing
	Private                 bool                   `json:"private"`
	Status                  string                 `json:"status"`
	ContentType             string                 `json:"content_type"`
	ProcessedMessageContent string                 `json:"processed_message_content"`
	Sender                  *ChatwootSender        `json:"sender"`
	ContentAttributes       map[string]interface{} `json:"content_attributes"`
}

// ChatwootMeta contains conversation metadata including sender info.
type ChatwootMeta struct {
	Sender   *ChatwootMetaSender `json:"sender"`
	Assignee *ChatwootSender     `json:"assignee"`
}

// ChatwootMetaSender is the sender info from conversation meta.
type ChatwootMetaSender struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	PhoneNumber string `json:"phone_number"`
	Email       string `json:"email"`
	Identifier  string `json:"identifier"`
	Thumbnail   string `json:"thumbnail"`
}

// ChatwootSender represents a message sender.
type ChatwootSender struct {
	ID               int    `json:"id"`
	Name             string `json:"name"`
	AvailableName    string `json:"available_name"`
	Email            string `json:"email"`
	Type             string `json:"type"` // "user" or "contact"
	AvatarURL        string `json:"avatar_url"`
}

// ChatwootInbox represents inbox info.
type ChatwootInbox struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ChatwootAttachment represents a file attachment.
type ChatwootAttachment struct {
	ID          int    `json:"id"`
	MessageID   int    `json:"message_id"`
	FileType    string `json:"file_type"`
	AccountID   int    `json:"account_id"`
	DataURL     string `json:"data_url"`
	ThumbURL    string `json:"thumb_url"`
	FileSize    int    `json:"file_size"`
}

// ============================================================
// Chatwoot API Response Types
// ============================================================

// ChatwootContactResponse is the response from creating a contact.
type ChatwootContactResponse struct {
	Payload struct {
		Contact struct {
			ID         int    `json:"id"`
			Name       string `json:"name"`
			Identifier string `json:"identifier"`
		} `json:"contact"`
	} `json:"payload"`
}

// ChatwootConversationResponse is the response from creating a conversation.
type ChatwootConversationResponse struct {
	ID   int              `json:"id"`
	Meta *ChatwootMeta    `json:"meta"`
}

// ChatwootMessageResponse is the response from sending a message.
type ChatwootMessageResponse struct {
	ID      int    `json:"id"`
	Content string `json:"content"`
}
