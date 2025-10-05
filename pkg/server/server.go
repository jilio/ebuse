package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
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
	// Apply middleware chain: rate limit -> auth -> compression -> handler
	s.mux.HandleFunc("/events", s.chain(s.handleEvents, config.EnableGzip))
	s.mux.HandleFunc("/events/batch", s.chain(s.handleBatchEvents, config.EnableGzip))
	s.mux.HandleFunc("/events/stream", s.chain(s.handleStreamEvents, config.EnableGzip))
	s.mux.HandleFunc("/position", s.chain(s.handlePosition, false))
	s.mux.HandleFunc("/subscriptions/", s.chain(s.handleSubscriptions, false))
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/metrics", s.authMiddleware(s.handleMetrics))
}

// chain applies middleware in order: rate limit -> auth -> optional compression
func (s *Server) chain(handler http.HandlerFunc, enableCompression bool) http.HandlerFunc {
	h := handler
	if enableCompression {
		h = compressionMiddleware(h)
	}
	h = s.authMiddleware(h)
	h = s.rateLimiter.middleware(h)
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
	var event store.StoredEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := s.store.Save(ctx, &event); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save event: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(event)
}

func (s *Server) loadEvents(w http.ResponseWriter, r *http.Request) {
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	from, err := strconv.ParseInt(fromStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid 'from' parameter", http.StatusBadRequest)
		return
	}

	to := int64(-1)
	if toStr != "" {
		to, err = strconv.ParseInt(toStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid 'to' parameter", http.StatusBadRequest)
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	events, err := s.store.Load(ctx, from, to)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load events: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

// handleBatchEvents handles batch event insertion
func (s *Server) handleBatchEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var events []*store.StoredEvent
	if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if len(events) > 1000 {
		http.Error(w, "Batch size limited to 1000 events", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := s.store.SaveBatch(ctx, events); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save batch: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"saved": len(events),
		"first_position": events[0].Position,
		"last_position": events[len(events)-1].Position,
	})
}

// handleStreamEvents streams events for large replays
func (s *Server) handleStreamEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fromStr := r.URL.Query().Get("from")
	batchSizeStr := r.URL.Query().Get("batch_size")

	from, err := strconv.ParseInt(fromStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid 'from' parameter", http.StatusBadRequest)
		return
	}

	batchSize := 1000
	if batchSizeStr != "" {
		bs, err := strconv.Atoi(batchSizeStr)
		if err == nil && bs > 0 && bs <= 5000 {
			batchSize = bs
		}
	}

	ctx := r.Context()

	// Set headers for streaming
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Transfer-Encoding", "chunked")

	// Use JSON array streaming
	w.Write([]byte("["))
	first := true

	err = s.store.LoadStream(ctx, from, batchSize, func(batch []*store.StoredEvent) error {
		for _, event := range batch {
			if !first {
				w.Write([]byte(","))
			}
			first = false

			data, err := json.Marshal(event)
			if err != nil {
				return err
			}
			w.Write(data)

			// Flush to client
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		return nil
	})

	if err != nil {
		log.Printf("Stream error: %v", err)
	}

	w.Write([]byte("]"))
}

func (s *Server) handlePosition(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	position, err := s.store.GetPosition(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get position: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{"position": position})
}

func (s *Server) handleSubscriptions(w http.ResponseWriter, r *http.Request) {
	// Extract subscription ID from path: /subscriptions/{id}/position
	path := strings.TrimPrefix(r.URL.Path, "/subscriptions/")
	parts := strings.Split(path, "/")

	if len(parts) != 2 || parts[1] != "position" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	subscriptionID := parts[0]

	switch r.Method {
	case http.MethodPost, http.MethodPut:
		s.saveSubscriptionPosition(w, r, subscriptionID)
	case http.MethodGet:
		s.loadSubscriptionPosition(w, r, subscriptionID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) saveSubscriptionPosition(w http.ResponseWriter, r *http.Request, subscriptionID string) {
	var req struct {
		Position int64 `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := s.store.SaveSubscriptionPosition(ctx, subscriptionID, req.Position); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save subscription position: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) loadSubscriptionPosition(w http.ResponseWriter, r *http.Request, subscriptionID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	position, err := s.store.LoadSubscriptionPosition(ctx, subscriptionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load subscription position: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{"position": position})
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
	json.NewEncoder(w).Encode(map[string]interface{}{
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
