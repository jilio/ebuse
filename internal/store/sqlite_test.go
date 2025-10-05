package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestSQLiteStore(t *testing.T) {
	// Create temporary database
	dbPath := "test_events.db"
	defer os.Remove(dbPath)

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Test Save
	t.Run("Save", func(t *testing.T) {
		event := &StoredEvent{
			Type:      "TestEvent",
			Data:      json.RawMessage(`{"message":"hello"}`),
			Timestamp: time.Now(),
		}

		if err := store.Save(ctx, event); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		if event.Position == 0 {
			t.Error("Expected position to be set")
		}
	})

	// Test GetPosition
	t.Run("GetPosition", func(t *testing.T) {
		pos, err := store.GetPosition(ctx)
		if err != nil {
			t.Fatalf("GetPosition failed: %v", err)
		}

		if pos == 0 {
			t.Error("Expected position > 0")
		}
	})

	// Test Load
	t.Run("Load", func(t *testing.T) {
		// Save multiple events
		for i := 0; i < 3; i++ {
			event := &StoredEvent{
				Type:      "TestEvent",
				Data:      json.RawMessage(`{"index":` + string(rune(i+'0')) + `}`),
				Timestamp: time.Now(),
			}
			if err := store.Save(ctx, event); err != nil {
				t.Fatalf("Save failed: %v", err)
			}
		}

		events, err := store.Load(ctx, 1, -1)
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		if len(events) < 3 {
			t.Errorf("Expected at least 3 events, got %d", len(events))
		}
	})

	// Test Subscription Position
	t.Run("SubscriptionPosition", func(t *testing.T) {
		subID := "test-subscription"
		position := int64(42)

		if err := store.SaveSubscriptionPosition(ctx, subID, position); err != nil {
			t.Fatalf("SaveSubscriptionPosition failed: %v", err)
		}

		loaded, err := store.LoadSubscriptionPosition(ctx, subID)
		if err != nil {
			t.Fatalf("LoadSubscriptionPosition failed: %v", err)
		}

		if loaded != position {
			t.Errorf("Expected position %d, got %d", position, loaded)
		}
	})

	// Test Load with range
	t.Run("LoadRange", func(t *testing.T) {
		events, err := store.Load(ctx, 1, 2)
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		if len(events) != 2 {
			t.Errorf("Expected 2 events in range, got %d", len(events))
		}
	})
}

func TestEmptyStore(t *testing.T) {
	dbPath := "test_empty.db"
	defer os.Remove(dbPath)

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	t.Run("GetPositionEmpty", func(t *testing.T) {
		pos, err := store.GetPosition(ctx)
		if err != nil {
			t.Fatalf("GetPosition failed: %v", err)
		}

		if pos != 0 {
			t.Errorf("Expected position 0 for empty store, got %d", pos)
		}
	})

	t.Run("LoadEmpty", func(t *testing.T) {
		events, err := store.Load(ctx, 0, -1)
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		if len(events) != 0 {
			t.Errorf("Expected 0 events, got %d", len(events))
		}
	})

	t.Run("LoadSubscriptionPositionMissing", func(t *testing.T) {
		pos, err := store.LoadSubscriptionPosition(ctx, "missing")
		if err != nil {
			t.Fatalf("LoadSubscriptionPosition failed: %v", err)
		}

		if pos != 0 {
			t.Errorf("Expected position 0 for missing subscription, got %d", pos)
		}
	})
}
