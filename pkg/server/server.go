package server

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jilio/ebuse/internal/store"
)

// Server provides HTTP API for remote event storage
type Server struct {
	store       *store.SQLiteStore
	apiKey      string
	mux         *http.ServeMux
	rateLimiter *rateLimiter
}

// Config holds server configuration
type Config struct {
	RateLimit      int  // Requests per second per IP
	RateBurst      int  // Burst size for rate limiter
	EnableGzip     bool // Enable gzip compression
}

// DefaultConfig returns production-ready defaults
func DefaultConfig() *Config {
	return &Config{
		RateLimit:  100, // 100 req/s per IP
		RateBurst:  200, // Allow bursts up to 200
		EnableGzip: true,
	}
}

// New creates a new event storage server (deprecated: use NewWithConfig)
func New(store *store.SQLiteStore) *Server {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		log.Fatal("API_KEY environment variable must be set")
	}
	return NewWithConfig(store, DefaultConfig(), apiKey)
}

// NewWithConfig creates a server with custom configuration
func NewWithConfig(store *store.SQLiteStore, config *Config, apiKey string) *Server {
	s := &Server{
		store:       store,
		apiKey:      apiKey,
		mux:         http.NewServeMux(),
		rateLimiter: newRateLimiter(config.RateLimit, config.RateBurst),
	}

	s.setupRoutes(config)
	return s
}

func (s *Server) setupRoutes(config *Config) {
	// Apply middleware chain: logging -> rate limit -> auth -> compression -> handler
	s.mux.HandleFunc("/events", s.chain(s.handleEvents, config.EnableGzip))
	s.mux.HandleFunc("/events/batch", s.chain(s.handleBatchEvents, config.EnableGzip))
	s.mux.HandleFunc("/events/stream", s.chain(s.handleStreamEvents, config.EnableGzip))
	s.mux.HandleFunc("/position", s.chain(s.handlePosition, false))
	s.mux.HandleFunc("/subscriptions/", s.chain(s.handleSubscriptions, false))
	s.mux.HandleFunc("/health", loggingMiddleware(s.handleHealth))
	s.mux.HandleFunc("/metrics", loggingMiddleware(s.authMiddleware(s.handleMetrics)))
}

// chain applies middleware in order: logging -> rate limit -> auth -> optional compression
func (s *Server) chain(handler http.HandlerFunc, enableCompression bool) http.HandlerFunc {
	h := handler
	if enableCompression {
		h = compressionMiddleware(h)
	}
	h = s.authMiddleware(h)
	h = s.rateLimiter.middleware(h)
	h = loggingMiddleware(h)
	return h
}

// authMiddleware validates the API_KEY header
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			apiKey = r.Header.Get("Authorization")
			if after, ok := strings.CutPrefix(apiKey, "Bearer "); ok {
				apiKey = after
			}
		}

		if apiKey != s.apiKey {
			// Extract IP for logging
			ip := r.RemoteAddr
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				ip = strings.Split(forwarded, ",")[0]
			}

			slog.Warn("Authentication failed",
				"ip", ip,
				"path", r.URL.Path,
				"method", r.Method)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// handleEvents handles both GET (load) and POST (save) for events
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.saveEvent(w, r)
	case http.MethodGet:
		s.loadEvents(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) saveEvent(w http.ResponseWriter, r *http.Request) {
	saveEventHandler(w, r, s.store)
}

func (s *Server) loadEvents(w http.ResponseWriter, r *http.Request) {
	loadEventsHandler(w, r, s.store)
}

// handleBatchEvents handles batch event insertion
func (s *Server) handleBatchEvents(w http.ResponseWriter, r *http.Request) {
	batchEventsHandler(w, r, s.store)
}

// handleStreamEvents streams events for large replays
func (s *Server) handleStreamEvents(w http.ResponseWriter, r *http.Request) {
	streamEventsHandler(w, r, s.store)
}

func (s *Server) handlePosition(w http.ResponseWriter, r *http.Request) {
	positionHandler(w, r, s.store)
}

func (s *Server) handleSubscriptions(w http.ResponseWriter, r *http.Request) {
	subscriptionsHandler(w, r, s.store)
}


// handleHealth provides health check endpoint
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	// Check database connectivity
	_, err := s.store.GetPosition(ctx)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "unhealthy",
			"error":  err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

// handleMetrics provides basic metrics
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	position, _ := s.store.GetPosition(ctx)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"total_events": position,
		"timestamp":    time.Now().Unix(),
	})
}

// Close stops the server and cleans up resources
func (s *Server) Close() error {
	if s.rateLimiter != nil {
		s.rateLimiter.Stop()
	}
	return nil
}

// ServeHTTP implements http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
