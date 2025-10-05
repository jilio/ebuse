package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jilio/ebuse/internal/store"
)

func TestNew(t *testing.T) {
	c := New("http://localhost:8080", "test-key")
	if c.baseURL != "http://localhost:8080" {
		t.Errorf("expected baseURL http://localhost:8080, got %s", c.baseURL)
	}
	if c.apiKey != "test-key" {
		t.Errorf("expected apiKey test-key, got %s", c.apiKey)
	}
	if c.client.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", c.client.Timeout)
	}
}

func TestSave(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("expected X-API-Key test-key, got %s", r.Header.Get("X-API-Key"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		var event store.StoredEvent
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		// Return event with position
		event.Position = 42
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(event)
	}))
	defer server.Close()

	client := New(server.URL, "test-key")
	event := &store.StoredEvent{
		Type: "TestEvent",
		Data: []byte(`{"test": "data"}`),
	}

	ctx := context.Background()
	if err := client.Save(ctx, event); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if event.Position != 42 {
		t.Errorf("expected position 42, got %d", event.Position)
	}
}

func TestSave_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := New(server.URL, "test-key")
	event := &store.StoredEvent{Type: "test"}

	ctx := context.Background()
	err := client.Save(ctx, event)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestLoad(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("expected X-API-Key test-key, got %s", r.Header.Get("X-API-Key"))
		}
		if r.URL.Query().Get("from") != "0" {
			t.Errorf("expected from=0, got %s", r.URL.Query().Get("from"))
		}

		events := []*store.StoredEvent{
			{Position: 1, Type: "Event1"},
			{Position: 2, Type: "Event2"},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(events)
	}))
	defer server.Close()

	client := New(server.URL, "test-key")
	ctx := context.Background()

	events, err := client.Load(ctx, 0, -1)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestLoad_WithTo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("from") != "10" {
			t.Errorf("expected from=10, got %s", r.URL.Query().Get("from"))
		}
		if r.URL.Query().Get("to") != "20" {
			t.Errorf("expected to=20, got %s", r.URL.Query().Get("to"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]*store.StoredEvent{})
	}))
	defer server.Close()

	client := New(server.URL, "test-key")
	ctx := context.Background()

	_, err := client.Load(ctx, 10, 20)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
}

func TestLoad_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not Found", http.StatusNotFound)
	}))
	defer server.Close()

	client := New(server.URL, "test-key")
	ctx := context.Background()

	_, err := client.Load(ctx, 0, -1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetPosition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("expected X-API-Key test-key, got %s", r.Header.Get("X-API-Key"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int64{"position": 123})
	}))
	defer server.Close()

	client := New(server.URL, "test-key")
	ctx := context.Background()

	position, err := client.GetPosition(ctx)
	if err != nil {
		t.Fatalf("GetPosition failed: %v", err)
	}

	if position != 123 {
		t.Errorf("expected position 123, got %d", position)
	}
}

func TestGetPosition_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := New(server.URL, "test-key")
	ctx := context.Background()

	_, err := client.GetPosition(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSaveSubscriptionPosition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("expected X-API-Key test-key, got %s", r.Header.Get("X-API-Key"))
		}
		if r.URL.Path != "/subscriptions/test-sub/position" {
			t.Errorf("expected /subscriptions/test-sub/position, got %s", r.URL.Path)
		}

		var req map[string]int64
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req["position"] != 99 {
			t.Errorf("expected position 99, got %d", req["position"])
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := New(server.URL, "test-key")
	ctx := context.Background()

	err := client.SaveSubscriptionPosition(ctx, "test-sub", 99)
	if err != nil {
		t.Fatalf("SaveSubscriptionPosition failed: %v", err)
	}
}

func TestSaveSubscriptionPosition_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Bad Request", http.StatusBadRequest)
	}))
	defer server.Close()

	client := New(server.URL, "test-key")
	ctx := context.Background()

	err := client.SaveSubscriptionPosition(ctx, "test-sub", 99)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestLoadSubscriptionPosition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("expected X-API-Key test-key, got %s", r.Header.Get("X-API-Key"))
		}
		if r.URL.Path != "/subscriptions/test-sub/position" {
			t.Errorf("expected /subscriptions/test-sub/position, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int64{"position": 55})
	}))
	defer server.Close()

	client := New(server.URL, "test-key")
	ctx := context.Background()

	position, err := client.LoadSubscriptionPosition(ctx, "test-sub")
	if err != nil {
		t.Fatalf("LoadSubscriptionPosition failed: %v", err)
	}

	if position != 55 {
		t.Errorf("expected position 55, got %d", position)
	}
}

func TestLoadSubscriptionPosition_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	client := New(server.URL, "test-key")
	ctx := context.Background()

	_, err := client.LoadSubscriptionPosition(ctx, "test-sub")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
