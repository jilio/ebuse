package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
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
	GetStore(apiKey string) (store.EventStore, string, bool)
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
	// Apply middleware chain: logging -> rate limit -> auth -> compression -> handler
	s.mux.HandleFunc("/events", s.chain(s.handleEvents, s.config.EnableGzip))
	s.mux.HandleFunc("/events/batch", s.chain(s.handleBatchEvents, s.config.EnableGzip))
	s.mux.HandleFunc("/events/stream", s.chain(s.handleStreamEvents, s.config.EnableGzip))
	s.mux.HandleFunc("/position", s.chain(s.handlePosition, false))
	s.mux.HandleFunc("/subscriptions/", s.chain(s.handleSubscriptions, false))
	s.mux.HandleFunc("/health", loggingMiddleware(s.handleHealth))
	s.mux.HandleFunc("/metrics", loggingMiddleware(s.authMiddleware(s.handleMetrics)))
	s.mux.HandleFunc("/tenants", loggingMiddleware(s.authMiddleware(s.handleTenants)))
}

// chain applies middleware in order: logging -> rate limit -> auth -> optional compression
func (s *MultiTenantServer) chain(handler http.HandlerFunc, enableCompression bool) http.HandlerFunc {
	h := handler
	if enableCompression {
		h = compressionMiddleware(h)
	}
	h = s.authMiddleware(h)
	h = s.rateLimiter.middleware(h)
	h = loggingMiddleware(h)
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

		// Extract IP for logging
		ip := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			ip = strings.Split(forwarded, ",")[0]
		}

		if apiKey == "" {
			slog.Warn("Authentication failed - no API key provided",
				"ip", ip,
				"path", r.URL.Path,
				"method", r.Method)
			http.Error(w, "API key required", http.StatusUnauthorized)
			return
		}

		// Get store for this API key
		tenantStore, tenantName, ok := s.tenantManager.GetStore(apiKey)
		if !ok {
			slog.Warn("Authentication failed - invalid API key",
				"ip", ip,
				"path", r.URL.Path,
				"method", r.Method)
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
func getTenantStore(r *http.Request) (store.EventStore, string, bool) {
	tenantStore, ok := r.Context().Value("tenant_store").(store.EventStore)
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
	saveEventHandler(w, r, tenantStore)
}

func (s *MultiTenantServer) loadEvents(w http.ResponseWriter, r *http.Request) {
	tenantStore, _, ok := getTenantStore(r)
	if !ok {
		http.Error(w, "Internal server error: tenant context missing", http.StatusInternalServerError)
		return
	}
	loadEventsHandler(w, r, tenantStore)
}

func (s *MultiTenantServer) handleBatchEvents(w http.ResponseWriter, r *http.Request) {
	tenantStore, _, ok := getTenantStore(r)
	if !ok {
		http.Error(w, "Internal server error: tenant context missing", http.StatusInternalServerError)
		return
	}
	batchEventsHandler(w, r, tenantStore)
}

func (s *MultiTenantServer) handleStreamEvents(w http.ResponseWriter, r *http.Request) {
	tenantStore, _, ok := getTenantStore(r)
	if !ok {
		http.Error(w, "Internal server error: tenant context missing", http.StatusInternalServerError)
		return
	}
	streamEventsHandler(w, r, tenantStore)
}

func (s *MultiTenantServer) handlePosition(w http.ResponseWriter, r *http.Request) {
	tenantStore, _, ok := getTenantStore(r)
	if !ok {
		http.Error(w, "Internal server error: tenant context missing", http.StatusInternalServerError)
		return
	}
	positionHandler(w, r, tenantStore)
}

func (s *MultiTenantServer) handleSubscriptions(w http.ResponseWriter, r *http.Request) {
	tenantStore, _, ok := getTenantStore(r)
	if !ok {
		http.Error(w, "Internal server error: tenant context missing", http.StatusInternalServerError)
		return
	}
	subscriptionsHandler(w, r, tenantStore)
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
