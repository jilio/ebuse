package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jilio/ebuse/internal/store"
)

func setupTestServer(t *testing.T) (*Server, func()) {
	dbPath := "test_server.db"

	sqliteStore, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Set API key for testing
	os.Setenv("API_KEY", "test-key-123")

	srv := New(sqliteStore)

	cleanup := func() {
		sqliteStore.Close()
		os.Remove(dbPath)
		os.Unsetenv("API_KEY")
	}

	return srv, cleanup
}

func TestAuthMiddleware(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	tests := []struct {
		name       string
		apiKey     string
		wantStatus int
	}{
		{
			name:       "Valid API Key",
			apiKey:     "test-key-123",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Invalid API Key",
			apiKey:     "wrong-key",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "No API Key",
			apiKey:     "",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/position", nil)
			if tt.apiKey != "" {
				req.Header.Set("X-API-Key", tt.apiKey)
			}

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("Expected status %d, got %d", tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestSaveEvent(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	event := &store.StoredEvent{
		Type:      "TestEvent",
		Data:      json.RawMessage(`{"message":"test"}`),
		Timestamp: time.Now(),
	}

	body, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal event: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
	req.Header.Set("X-API-Key", "test-key-123")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var savedEvent store.StoredEvent
	if err := json.NewDecoder(rr.Body).Decode(&savedEvent); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if savedEvent.Position == 0 {
		t.Error("Expected position to be set")
	}
}

func TestLoadEvents(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Save some events first
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		event := &store.StoredEvent{
			Type:      "TestEvent",
			Data:      json.RawMessage(`{"index":` + string(rune(i+'0')) + `}`),
			Timestamp: time.Now(),
		}
		srv.store.Save(ctx, event)
	}

	req := httptest.NewRequest(http.MethodGet, "/events?from=1", nil)
	req.Header.Set("X-API-Key", "test-key-123")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var events []*store.StoredEvent
	if err := json.NewDecoder(rr.Body).Decode(&events); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(events) != 3 {
		t.Errorf("Expected 3 events, got %d", len(events))
	}
}

func TestGetPosition(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/position", nil)
	req.Header.Set("X-API-Key", "test-key-123")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var result map[string]int64
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if _, ok := result["position"]; !ok {
		t.Error("Expected position in response")
	}
}

func TestSubscriptionPosition(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	subID := "test-sub"

	// Save position
	t.Run("Save", func(t *testing.T) {
		body := bytes.NewBufferString(`{"position":42}`)
		req := httptest.NewRequest(http.MethodPost, "/subscriptions/"+subID+"/position", body)
		req.Header.Set("X-API-Key", "test-key-123")
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Errorf("Expected status %d, got %d: %s", http.StatusNoContent, rr.Code, rr.Body.String())
		}
	})

	// Load position
	t.Run("Load", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/subscriptions/"+subID+"/position", nil)
		req.Header.Set("X-API-Key", "test-key-123")

		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}

		var result map[string]int64
		if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["position"] != 42 {
			t.Errorf("Expected position 42, got %d", result["position"])
		}
	})
}
