package service

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// LoggingRoundTripper wraps a standard http.RoundTripper to intercept and log outgoing HTTP traffic.
type LoggingRoundTripper struct {
	Proxied http.RoundTripper
	Name    string
}

// RoundTrip executes a single HTTP transaction, logging both the request and response.
func (lrt *LoggingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	// Capture the request body safely
	var reqBody []byte
	if req.Body != nil {
		var err error
		reqBody, err = io.ReadAll(req.Body)
		if err == nil {
			req.Body = io.NopCloser(bytes.NewBuffer(reqBody))
		}
	}

	proxiedTransport := lrt.Proxied
	if proxiedTransport == nil {
		proxiedTransport = http.DefaultTransport
	}

	resp, err := proxiedTransport.RoundTrip(req)
	duration := time.Since(start)

	if err != nil {
		log.Printf("\n--- [Outgoing HTTP Request Failure (%s)] ---\n"+
			"Method:      %s\n"+
			"URL:         %s\n"+
			"Headers:     %v\n"+
			"Payload:\n%s\n"+
			"Error:       %v\n"+
			"Duration:    %v\n"+
			"-------------------------------------------",
			lrt.Name, req.Method, req.URL.String(), req.Header, string(reqBody), err, duration)
		return nil, err
	}

	// Capture the response body safely
	var respBody []byte
	if resp.Body != nil {
		var err error
		respBody, err = io.ReadAll(resp.Body)
		if err == nil {
			resp.Body = io.NopCloser(bytes.NewBuffer(respBody))
		}
	}

	// Prevent logging large binary/media response bodies (like QR codes or page screenshots)
	displayRespBody := string(respBody)
	contentType := resp.Header.Get("Content-Type")
	if len(respBody) > 2000 || (contentType != "" && !bytes.Contains([]byte(contentType), []byte("json")) && !bytes.Contains([]byte(contentType), []byte("text"))) {
		displayRespBody = fmt.Sprintf("<Binary or large response payload of %d bytes, Content-Type: %s>", len(respBody), contentType)
	}

	// Prevent logging large binary/media request bodies (like upload attachments)
	displayReqBody := string(reqBody)
	reqContentType := req.Header.Get("Content-Type")
	if len(reqBody) > 2000 || (reqContentType != "" && bytes.Contains([]byte(reqContentType), []byte("multipart"))) {
		displayReqBody = fmt.Sprintf("<Multipart or large request payload of %d bytes, Content-Type: %s>", len(reqBody), reqContentType)
	}

	log.Printf("\n--- [Outgoing HTTP Request Sent (%s)] ---\n"+
		"Method:      %s\n"+
		"URL:         %s\n"+
		"Headers:     %v\n"+
		"Payload:\n%s\n"+
		"--- [Outgoing HTTP Response Received (%s)] ---\n"+
		"Status:      %d %s\n"+
		"Headers:     %v\n"+
		"Payload:\n%s\n"+
		"Duration:    %v\n"+
		"---------------------------------------------",
		lrt.Name, req.Method, req.URL.String(), req.Header, displayReqBody,
		lrt.Name, resp.StatusCode, http.StatusText(resp.StatusCode), resp.Header, displayRespBody,
		duration)

	return resp, nil
}
