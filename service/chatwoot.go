package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// ChatwootClient is an HTTP client for the Chatwoot API.
type ChatwootClient struct {
	BaseURL   string
	client    *http.Client
}

// NewChatwootClient creates a new Chatwoot API client.
func NewChatwootClient(baseURL string) *ChatwootClient {
	return &ChatwootClient{
		BaseURL:   baseURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &LoggingRoundTripper{
				Proxied: http.DefaultTransport,
				Name:    "Chatwoot",
			},
		},
	}
}

// SendMessage sends a text message to a Chatwoot conversation.
func (c *ChatwootClient) SendMessage(accountID, conversationID int, content, messageType string, private bool, inReplyTo *int, botToken string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/accounts/%d/conversations/%d/messages", c.BaseURL, accountID, conversationID)

	body := map[string]interface{}{
		"content":      content,
		"message_type": messageType,
		"private":      private,
	}
	if inReplyTo != nil {
		body["content_attributes"] = map[string]interface{}{
			"in_reply_to": *inReplyTo,
		}
	}

	return c.doJSON("POST", url, body, botToken)
}

// SendMessageWithAttachment sends a message with a binary attachment to Chatwoot.
// The attachment is provided as raw bytes with its filename and content type.
func (c *ChatwootClient) SendMessageWithAttachment(accountID, conversationID int, content, messageType string, private bool, attachmentData []byte, filename, contentType string, inReplyTo *int, botToken string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/accounts/%d/conversations/%d/messages", c.BaseURL, accountID, conversationID)

	// Build multipart form
	var buf bytes.Buffer
	writer := NewMultipartWriter(&buf)

	_ = writer.WriteField("message_type", messageType)
	_ = writer.WriteField("private", fmt.Sprintf("%v", private))
	if content != "" {
		_ = writer.WriteField("content", content)
	}
	if inReplyTo != nil {
		_ = writer.WriteField("content_attributes", fmt.Sprintf(`{"in_reply_to":%d}`, *inReplyTo))
	}

	part, err := writer.CreateFormFile("attachments[]", filename, contentType)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(attachmentData); err != nil {
		return nil, fmt.Errorf("failed to write attachment data: %w", err)
	}
	writer.Close()

	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("api_access_token", botToken)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chatwoot request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(respBody, &result)

	if resp.StatusCode >= 400 {
		return result, fmt.Errorf("chatwoot returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return result, nil
}

// CreateContact creates a new contact in Chatwoot.
func (c *ChatwootClient) CreateContact(accountID int, name, phone, identifier, avatarURL string, userToken string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/accounts/%d/contacts", c.BaseURL, accountID)

	body := map[string]interface{}{
		"name":       name,
		"identifier": identifier,
	}
	if phone != "" {
		body["phone_number"] = phone
	}
	if avatarURL != "" {
		body["avatar_url"] = avatarURL
	}

	return c.doJSON("POST", url, body, userToken)
}

// CreateConversation creates a new conversation in Chatwoot.
func (c *ChatwootClient) CreateConversation(accountID, inboxID, contactID int, status, messageContent string, userToken string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/accounts/%d/conversations", c.BaseURL, accountID)

	body := map[string]interface{}{
		"inbox_id":   inboxID,
		"contact_id": contactID,
		"status":     status,
		"message": map[string]interface{}{
			"content":      messageContent,
			"message_type": "incoming",
		},
	}

	return c.doJSON("POST", url, body, userToken)
}

// GetContact retrieves a contact from Chatwoot.
func (c *ChatwootClient) GetContact(accountID, contactID int, userToken string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/accounts/%d/contacts/%d", c.BaseURL, accountID, contactID)
	return c.doJSON("GET", url, nil, userToken)
}

// GetConversation retrieves a conversation from Chatwoot.
func (c *ChatwootClient) GetConversation(accountID, conversationID int, userToken string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/accounts/%d/conversations/%d", c.BaseURL, accountID, conversationID)
	return c.doJSON("GET", url, nil, userToken)
}

// doJSON performs an HTTP request with JSON body and returns the response as a map.
func (c *ChatwootClient) doJSON(method, url string, body interface{}, token string) (map[string]interface{}, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("api_access_token", token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chatwoot request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[Chatwoot] %s %s → %d", method, url, resp.StatusCode)

	var result map[string]interface{}
	json.Unmarshal(respBody, &result)

	if resp.StatusCode >= 400 {
		return result, fmt.Errorf("chatwoot returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return result, nil
}
