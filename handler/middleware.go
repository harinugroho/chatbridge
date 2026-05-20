package handler

import (
	"bytes"
	"chatbridge/logger"
	"io"
	"net/http"
	"time"
)

// loggingResponseWriter wraps a standard http.ResponseWriter to capture the status code and response body.
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
	body       []byte
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	lrw.body = append(lrw.body, b...)
	return lrw.ResponseWriter.Write(b)
}

// LoggingMiddleware intercepts, logs, and outputs HTTP request and response payloads.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Read and buffer the request body so we can log it and keep it available for down-stream handlers
		var reqBody []byte
		if r.Body != nil {
			var err error
			reqBody, err = io.ReadAll(r.Body)
			if err == nil {
				r.Body = io.NopCloser(bytes.NewBuffer(reqBody))
			}
		}

		// Initialize our custom ResponseWriter
		lrw := &loggingResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK, // default status code
		}

		next.ServeHTTP(lrw, r)

		duration := time.Since(start)

		logger.LogWebhook(r.Method, r.URL.Path, r.RemoteAddr, r.Header, reqBody, lrw.statusCode, lrw.body, duration)
	})
}
