package service

import (
	"bytes"
	"chatbridge/logger"
	"io"
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
		logger.LogClientRequest(lrt.Name, req.Method, req.URL.String(), req.Header, reqBody, 0, nil, nil, duration, err)
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

	logger.LogClientRequest(lrt.Name, req.Method, req.URL.String(), req.Header, reqBody, resp.StatusCode, resp.Header, respBody, duration, nil)

	return resp, nil
}
