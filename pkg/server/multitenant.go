package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jilio/ebuse/internal/store"
)

// MultiTenantServer provides HTTP API with multi-tenant support
type MultiTenantServer struct {
	tenantManager TenantManager
	mux           *http.ServeMux
	rateLimiter   *rateLimiter
	config        *Config
}

// TenantManager interface for managing multiple tenants
type TenantManager interface {
	GetStore(apiKey string) (*store.SQLiteStore, string, bool)
	GetAllTenants() []string
	Close() error
}

// NewMultiTenant creates a new multi-tenant server
func NewMultiTenant(tenantManager TenantManager, config *Config) *MultiTenantServer {
	if config == nil {
		config = DefaultConfig()
	}

	s := &MultiTenantServer{
		tenantManager: tenantManager,
		mux:           http.NewServeMux(),
		rateLimiter:   newRateLimiter(config.RateLimit, config.RateBurst),
		config:        config,
	}

	s.setupRoutes()
	return s
}

func (s *MultiTenantServer) setupRoutes() {
	// Apply middleware chain: rate limit -> auth -> compression -> handler
	s.mux.HandleFunc("/events", s.chain(s.handleEvents, s.config.EnableGzip))
	s.mux.HandleFunc("/events/batch", s.chain(s.handleBatchEvents, s.config.EnableGzip))
	s.mux.HandleFunc("/events/stream", s.chain(s.handleStreamEvents, s.config.EnableGzip))
	s.mux.HandleFunc("/position", s.chain(s.handlePosition, false))
	s.mux.HandleFunc("/subscriptions/", s.chain(s.handleSubscriptions, false))
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/metrics", s.authMiddleware(s.handleMetrics))
	s.mux.HandleFunc("/tenants", s.authMiddleware(s.handleTenants))
}

// chain applies middleware in order: rate limit -> auth -> optional compression
func (s *MultiTenantServer) chain(handler http.HandlerFunc, enableCompression bool) http.HandlerFunc {
	h := handler
	if enableCompression {
		h = compressionMiddleware(h)
	}
	h = s.authMiddleware(h)
	h = s.rateLimiter.middleware(h)
	return h
}

// authMiddleware validates API key and injects tenant context
func (s *MultiTenantServer) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			apiKey = r.Header.Get("Authorization")
			if after, ok := strings.CutPrefix(apiKey, "Bearer "); ok {
				apiKey = after
			}
		}

		if apiKey == "" {
			http.Error(w, "API key required", http.StatusUnauthorized)
			return
		}

		// Get store for this API key
		tenantStore, tenantName, ok := s.tenantManager.GetStore(apiKey)
		if !ok {
			http.Error(w, "Invalid API key", http.StatusUnauthorized)
			return
		}

		// Inject tenant info into context
		ctx := context.WithValue(r.Context(), "tenant_store", tenantStore)
		ctx = context.WithValue(ctx, "tenant_name", tenantName)
		next(w, r.WithContext(ctx))
	}
}

// getTenantStore extracts tenant store from context
func getTenantStore(r *http.Request) (*store.SQLiteStore, string, bool) {
	tenantStore, ok := r.Context().Value("tenant_store").(*store.SQLiteStore)
	if !ok {
		return nil, "", false
	}
	tenantName, ok := r.Context().Value("tenant_name").(string)
	if !ok {
		return nil, "", false
	}
	return tenantStore, tenantName, true
}

// Event handlers (same as single-tenant but use tenant-specific store)

func (s *MultiTenantServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.saveEvent(w, r)
	case http.MethodGet:
		s.loadEvents(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *MultiTenantServer) saveEvent(w http.ResponseWriter, r *http.Request) {
	tenantStore, _, ok := getTenantStore(r)
	if !ok {
		http.Error(w, "Internal server error: tenant context missing", http.StatusInternalServerError)
		return
	}

	var event store.StoredEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := tenantStore.Save(ctx, &event); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save event: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(event)
}

func (s *MultiTenantServer) loadEvents(w http.ResponseWriter, r *http.Request) {
	tenantStore, _, ok := getTenantStore(r)
	if !ok {
		http.Error(w, "Internal server error: tenant context missing", http.StatusInternalServerError)
		return
	}

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

	events, err := tenantStore.Load(ctx, from, to)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load events: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

func (s *MultiTenantServer) handleBatchEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantStore, _, ok := getTenantStore(r)
	if !ok {
		http.Error(w, "Internal server error: tenant context missing", http.StatusInternalServerError)
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

	if err := tenantStore.SaveBatch(ctx, events); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save batch: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"saved":          len(events),
		"first_position": events[0].Position,
		"last_position":  events[len(events)-1].Position,
	})
}

func (s *MultiTenantServer) handleStreamEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantStore, _, ok := getTenantStore(r)
	if !ok {
		http.Error(w, "Internal server error: tenant context missing", http.StatusInternalServerError)
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

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Transfer-Encoding", "chunked")

	w.Write([]byte("["))
	first := true

	err = tenantStore.LoadStream(ctx, from, batchSize, func(batch []*store.StoredEvent) error {
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

func (s *MultiTenantServer) handlePosition(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantStore, _, ok := getTenantStore(r)
	if !ok {
		http.Error(w, "Internal server error: tenant context missing", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	position, err := tenantStore.GetPosition(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get position: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{"position": position})
}

func (s *MultiTenantServer) handleSubscriptions(w http.ResponseWriter, r *http.Request) {
	tenantStore, _, ok := getTenantStore(r)
	if !ok {
		http.Error(w, "Internal server error: tenant context missing", http.StatusInternalServerError)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/subscriptions/")
	parts := strings.Split(path, "/")

	if len(parts) != 2 || parts[1] != "position" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	subscriptionID := parts[0]

	switch r.Method {
	case http.MethodPost, http.MethodPut:
		s.saveSubscriptionPosition(w, r, tenantStore, subscriptionID)
	case http.MethodGet:
		s.loadSubscriptionPosition(w, r, tenantStore, subscriptionID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *MultiTenantServer) saveSubscriptionPosition(w http.ResponseWriter, r *http.Request, tenantStore *store.SQLiteStore, subscriptionID string) {
	var req struct {
		Position int64 `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := tenantStore.SaveSubscriptionPosition(ctx, subscriptionID, req.Position); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save subscription position: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *MultiTenantServer) loadSubscriptionPosition(w http.ResponseWriter, r *http.Request, tenantStore *store.SQLiteStore, subscriptionID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	position, err := tenantStore.LoadSubscriptionPosition(ctx, subscriptionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load subscription position: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{"position": position})
}

func (s *MultiTenantServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

func (s *MultiTenantServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	tenantStore, tenantName, ok := getTenantStore(r)
	if !ok {
		http.Error(w, "Internal server error: tenant context missing", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	position, _ := tenantStore.GetPosition(ctx)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"tenant":       tenantName,
		"total_events": position,
		"timestamp":    time.Now().Unix(),
	})
}

func (s *MultiTenantServer) handleTenants(w http.ResponseWriter, r *http.Request) {
	tenants := s.tenantManager.GetAllTenants()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"tenants": tenants,
		"count":   len(tenants),
	})
}

func (s *MultiTenantServer) Close() error {
	if s.rateLimiter != nil {
		s.rateLimiter.Stop()
	}
	return s.tenantManager.Close()
}

func (s *MultiTenantServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
