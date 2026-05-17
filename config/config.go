package config

import (
	"fmt"
	"os"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	// Database
	DatabaseURL string

	// Chatwoot
	ChatwootBaseURL string

	// WhatsApp (wwebjs-api)
	WhatsAppBaseURL string
	WhatsAppToken   string

	// Application
	ListenPort        string
	SystemPhoneNumber string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		DatabaseURL:       getEnv("DATABASE_URL", "postgres://chatbridge:chatbridge@postgres:5432/chatbridge?sslmode=disable"),
		ChatwootBaseURL:   getEnv("CHATWOOT_BASE_URL", "https://chat.indieapps.id"),
		WhatsAppBaseURL:   getEnv("WHATSAPP_BASE_URL", "http://wa.indieapps.id"),
		WhatsAppToken:     getEnv("WHATSAPP_TOKEN", ""),
		ListenPort:        getEnv("LISTEN_PORT", "8080"),
		SystemPhoneNumber: getEnv("SYSTEM_PHONE_NUMBER", "+62111111"),
	}
}

// Validate checks that required configuration values are set.
func (c *Config) Validate() error {
	if c.WhatsAppToken == "" {
		return fmt.Errorf("WHATSAPP_TOKEN is required")
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
