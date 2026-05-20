package main

import (
	"chatbridge/config"
	"chatbridge/database"
	"chatbridge/handler"
	"chatbridge/logger"
	"chatbridge/service"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	logger.InitLogging()
	log.Println("=== Chatwoot ↔ WhatsApp Bridge ===")

	// Load configuration
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration error: %v", err)
	}
	log.Println("[Config] Loaded successfully")
	log.Printf("[Config] Chatwoot: %s", cfg.ChatwootBaseURL)
	log.Printf("[Config] WhatsApp: %s", cfg.WhatsAppBaseURL)
	log.Printf("[Config] System Phone: %s", cfg.SystemPhoneNumber)

	// Connect to database
	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Database error: %v", err)
	}
	defer db.Close()

	// Create service clients
	chatwootClient := service.NewChatwootClient(cfg.ChatwootBaseURL)
	whatsappClient := service.NewWhatsAppClient(cfg.WhatsAppBaseURL, cfg.WhatsAppToken)
	bridge := service.NewBridge(db, chatwootClient, whatsappClient, cfg)

	// Create handlers
	chatwootHandler := handler.NewChatwootWebhookHandler(bridge)
	whatsappHandler := handler.NewWhatsAppWebhookHandler(bridge)

	// Setup HTTP routes
	mux := http.NewServeMux()
	mux.Handle("/webhook/chatwoot", handler.LoggingMiddleware(chatwootHandler))
	mux.Handle("/webhook/whatsapp", handler.LoggingMiddleware(whatsappHandler))
	mux.HandleFunc("/debug", handler.ServeDebugPage)
	mux.HandleFunc("/debug/ws", handler.ServeWebSocket)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})

	// Start server
	addr := fmt.Sprintf(":%s", cfg.ListenPort)
	log.Printf("[Server] Listening on %s", addr)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("[Server] Shutting down...")
		server.Close()
	}()

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("[Server] Error: %v", err)
	}

	log.Println("[Server] Stopped")
}
