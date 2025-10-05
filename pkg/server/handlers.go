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

// Shared handler implementations used by both single-tenant and multi-tenant servers

func saveEventHandler(w http.ResponseWriter, r *http.Request, st *store.SQLiteStore) {
	var event store.StoredEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := st.Save(ctx, &event); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save event: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(event)
}

func loadEventsHandler(w http.ResponseWriter, r *http.Request, st *store.SQLiteStore) {
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

	events, err := st.Load(ctx, from, to)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load events: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

func batchEventsHandler(w http.ResponseWriter, r *http.Request, st *store.SQLiteStore) {
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

	if err := st.SaveBatch(ctx, events); err != nil {
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

func streamEventsHandler(w http.ResponseWriter, r *http.Request, st *store.SQLiteStore) {
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

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Transfer-Encoding", "chunked")

	w.Write([]byte("["))
	first := true

	err = st.LoadStream(ctx, from, batchSize, func(batch []*store.StoredEvent) error {
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

func positionHandler(w http.ResponseWriter, r *http.Request, st *store.SQLiteStore) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	position, err := st.GetPosition(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get position: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{"position": position})
}

func subscriptionsHandler(w http.ResponseWriter, r *http.Request, st *store.SQLiteStore) {
	path := strings.TrimPrefix(r.URL.Path, "/subscriptions/")
	parts := strings.Split(path, "/")

	if len(parts) != 2 || parts[1] != "position" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	subscriptionID := parts[0]

	switch r.Method {
	case http.MethodPost, http.MethodPut:
		saveSubscriptionPositionHandler(w, r, st, subscriptionID)
	case http.MethodGet:
		loadSubscriptionPositionHandler(w, r, st, subscriptionID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func saveSubscriptionPositionHandler(w http.ResponseWriter, r *http.Request, st *store.SQLiteStore, subscriptionID string) {
	var req struct {
		Position int64 `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := st.SaveSubscriptionPosition(ctx, subscriptionID, req.Position); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save subscription position: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func loadSubscriptionPositionHandler(w http.ResponseWriter, r *http.Request, st *store.SQLiteStore, subscriptionID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	position, err := st.LoadSubscriptionPosition(ctx, subscriptionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load subscription position: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{"position": position})
}
