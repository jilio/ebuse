package ebuse

import (
	"os"
	"strconv"
	"time"
)

// ProductionConfig holds all production configuration
type ProductionConfig struct {
	// Server
	Port              string
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration

	// Database
	DBPath            string
	StoreBackend      string  // "sqlite" or "pebble"

	// Rate Limiting
	RateLimit         int
	RateBurst         int

	// Features
	EnableGzip        bool

	// API
	APIKey            string
}

// LoadConfigFromEnv loads configuration from environment variables with production defaults
func LoadConfigFromEnv() *ProductionConfig {
	return &ProductionConfig{
		// Server defaults
		Port:            getEnv("PORT", "8080"),
		ReadTimeout:     parseDuration("READ_TIMEOUT", 30*time.Second),
		WriteTimeout:    parseDuration("WRITE_TIMEOUT", 60*time.Second),
		IdleTimeout:     parseDuration("IDLE_TIMEOUT", 120*time.Second),
		ShutdownTimeout: parseDuration("SHUTDOWN_TIMEOUT", 30*time.Second),

		// Database defaults
		DBPath:          getEnv("DB_PATH", "events.db"),
		StoreBackend:    getEnv("STORE_BACKEND", "pebble"),

		// Rate limiting defaults (per IP)
		RateLimit:       parseInt("RATE_LIMIT", 100),
		RateBurst:       parseInt("RATE_BURST", 200),

		// Features
		EnableGzip:      parseBool("ENABLE_GZIP", true),

		// Required
		APIKey:          os.Getenv("API_KEY"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

func parseInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func parseBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}
