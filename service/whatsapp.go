package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// WhatsAppClient is an HTTP client for the wwebjs API.
type WhatsAppClient struct {
	BaseURL string
	APIKey  string
	client  *http.Client
}

// NewWhatsAppClient creates a new WhatsApp API client.
func NewWhatsAppClient(baseURL, apiKey string) *WhatsAppClient {
	return &WhatsAppClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		client: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &LoggingRoundTripper{
				Proxied: http.DefaultTransport,
				Name:    "WhatsApp",
			},
		},
	}
}

// StartSession starts a WhatsApp session.
func (w *WhatsAppClient) StartSession(sessionID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/session/start/%s", w.BaseURL, sessionID)
	return w.doGet(url)
}

// StopSession stops a WhatsApp session.
func (w *WhatsAppClient) StopSession(sessionID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/session/stop/%s", w.BaseURL, sessionID)
	return w.doGet(url)
}

// TerminateSession terminates a WhatsApp session.
func (w *WhatsAppClient) TerminateSession(sessionID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/session/terminate/%s", w.BaseURL, sessionID)
	return w.doGet(url)
}

// GetSessionStatus gets the status of a WhatsApp session.
func (w *WhatsAppClient) GetSessionStatus(sessionID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/session/status/%s", w.BaseURL, sessionID)
	return w.doGet(url)
}

// GetQRImage fetches the QR code image for session pairing.
func (w *WhatsAppClient) GetQRImage(sessionID string) ([]byte, string, error) {
	url := fmt.Sprintf("%s/session/qr/%s/image", w.BaseURL, sessionID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("x-api-key", w.APIKey)

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("whatsapp request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/png"
	}
	return data, ct, nil
}

// GetScreenshot fetches a screenshot of the session page.
func (w *WhatsAppClient) GetScreenshot(sessionID string) ([]byte, string, error) {
	url := fmt.Sprintf("%s/session/getPageScreenshot/%s", w.BaseURL, sessionID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("x-api-key", w.APIKey)

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("whatsapp request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/png"
	}
	return data, ct, nil
}

// SendMessage sends a message via WhatsApp.
func (w *WhatsAppClient) SendMessage(sessionID, chatID, contentType, content string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/client/sendMessage/%s", w.BaseURL, sessionID)
	body := map[string]interface{}{
		"chatId":      chatID,
		"contentType": contentType,
		"content":     content,
	}
	return w.doPost(url, body)
}

// SendTyping sends typing indicator to a WhatsApp chat.
func (w *WhatsAppClient) SendTyping(sessionID, chatID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/chat/sendStateTyping/%s", w.BaseURL, sessionID)
	body := map[string]interface{}{
		"chatId": chatID,
	}
	return w.doPost(url, body)
}

// ClearState clears typing state for a WhatsApp chat.
func (w *WhatsAppClient) ClearState(sessionID, chatID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/chat/clearState/%s", w.BaseURL, sessionID)
	body := map[string]interface{}{
		"chatId": chatID,
	}
	return w.doPost(url, body)
}

// GetProfilePicURL gets the profile picture URL for a contact.
func (w *WhatsAppClient) GetProfilePicURL(sessionID, contactID string) (string, error) {
	url := fmt.Sprintf("%s/client/getProfilePicUrl/%s", w.BaseURL, sessionID)
	body := map[string]interface{}{
		"contactId": contactID,
	}
	resp, err := w.doPost(url, body)
	if err != nil {
		return "", err
	}
	if result, ok := resp["result"].(string); ok {
		return result, nil
	}
	return "", nil
}

// GetContactLidAndPhone gets the LID and phone number for given user IDs.
func (w *WhatsAppClient) GetContactLidAndPhone(sessionID string, userIDs []string) ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/client/getContactLidAndPhone/%s", w.BaseURL, sessionID)
	body := map[string]interface{}{
		"userIds": userIDs,
	}
	resp, err := w.doPost(url, body)
	if err != nil {
		return nil, err
	}
	if data, ok := resp["data"].([]interface{}); ok {
		var result []map[string]interface{}
		for _, d := range data {
			if m, ok := d.(map[string]interface{}); ok {
				result = append(result, m)
			}
		}
		return result, nil
	}
	return nil, nil
}

// GetContactClassInfo gets detailed contact info.
func (w *WhatsAppClient) GetContactClassInfo(sessionID, contactID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/contact/getClassInfo/%s", w.BaseURL, sessionID)
	body := map[string]interface{}{
		"contactId": contactID,
	}
	return w.doPost(url, body)
}

// GetGroupInfo gets group chat information.
func (w *WhatsAppClient) GetGroupInfo(sessionID, chatID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/groupChat/getClassInfo/%s", w.BaseURL, sessionID)
	body := map[string]interface{}{
		"chatId": chatID,
	}
	return w.doPost(url, body)
}

// MediaResponse holds the result of a media download.
type MediaResponse struct {
	Data        []byte // Raw binary media data
	ContentType string // MIME type from response header
	Filename    string // Optional filename
}

// DownloadMedia downloads media from a message.
// The wwebjs API returns raw binary data with the appropriate Content-Type header.
func (w *WhatsAppClient) DownloadMedia(sessionID, chatID, messageID string) (*MediaResponse, error) {
	reqURL := fmt.Sprintf("%s/message/downloadMediaAsData/%s", w.BaseURL, sessionID)
	bodyData := map[string]interface{}{
		"chatId":    chatID,
		"messageId": messageID,
	}

	b, err := json.Marshal(bodyData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", w.APIKey)

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("whatsapp request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	log.Printf("[WhatsApp] POST %s → %d", reqURL, resp.StatusCode)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("whatsapp returned status %d: %s", resp.StatusCode, string(respBody))
	}

	contentType := resp.Header.Get("Content-Type")

	// If the response is JSON, it may be an error or base64-encoded data
	if strings.HasPrefix(contentType, "application/json") {
		var result map[string]interface{}
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("failed to parse JSON response: %w", err)
		}

		// Check for API-level failure
		if success, ok := result["success"].(bool); ok && !success {
			errMsg, _ := result["message"].(string)
			return nil, fmt.Errorf("whatsapp download media failed: %s", errMsg)
		}

		// Handle base64-encoded data in JSON response
		if dataStr, ok := result["data"].(string); ok && dataStr != "" {
			mimetype, _ := result["mimetype"].(string)
			filename, _ := result["filename"].(string)
			decoded, err := decodeBase64Media(dataStr)
			if err != nil {
				return nil, fmt.Errorf("failed to decode base64 media: %w", err)
			}
			if mimetype == "" {
				mimetype = "application/octet-stream"
			}
			return &MediaResponse{
				Data:        decoded,
				ContentType: mimetype,
				Filename:    filename,
			}, nil
		}

		return nil, fmt.Errorf("JSON response contained no media data: %v", result)
	}

	// Binary response — the body IS the media data
	if len(respBody) == 0 {
		return nil, fmt.Errorf("empty binary response body")
	}

	return &MediaResponse{
		Data:        respBody,
		ContentType: contentType,
		Filename:    "",
	}, nil
}

// doGet performs a GET request with API key.
func (w *WhatsAppClient) doGet(url string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", w.APIKey)

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("whatsapp request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[WhatsApp] GET %s → %d", url, resp.StatusCode)

	var result map[string]interface{}
	json.Unmarshal(respBody, &result)
	return result, nil
}

// doPost performs a POST request with JSON body and API key.
func (w *WhatsAppClient) doPost(url string, body interface{}) (map[string]interface{}, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", w.APIKey)

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("whatsapp request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[WhatsApp] POST %s → %d", url, resp.StatusCode)

	var result map[string]interface{}
	json.Unmarshal(respBody, &result)

	if resp.StatusCode >= 400 {
		return result, fmt.Errorf("whatsapp returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return result, nil
}
