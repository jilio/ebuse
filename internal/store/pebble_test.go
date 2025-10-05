package store

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPebbleStore_Save(t *testing.T) {
	store, err := NewPebbleStore(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	event := &StoredEvent{
		Type: "TestEvent",
		Data: json.RawMessage(`{"test": "data"}`),
	}

	if err := store.Save(ctx, event); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if event.Position != 1 {
		t.Errorf("expected position 1, got %d", event.Position)
	}
}

func TestPebbleStore_SaveBatch(t *testing.T) {
	store, err := NewPebbleStore(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	events := []*StoredEvent{
		{Type: "Event1", Data: json.RawMessage(`{"index": 1}`)},
		{Type: "Event2", Data: json.RawMessage(`{"index": 2}`)},
		{Type: "Event3", Data: json.RawMessage(`{"index": 3}`)},
	}

	if err := store.SaveBatch(ctx, events); err != nil {
		t.Fatalf("SaveBatch failed: %v", err)
	}

	if events[0].Position != 1 || events[1].Position != 2 || events[2].Position != 3 {
		t.Errorf("incorrect positions: %d, %d, %d", events[0].Position, events[1].Position, events[2].Position)
	}
}

func TestPebbleStore_Load(t *testing.T) {
	store, err := NewPebbleStore(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Save some events
	events := []*StoredEvent{
		{Type: "Event1", Data: json.RawMessage(`{"index": 1}`)},
		{Type: "Event2", Data: json.RawMessage(`{"index": 2}`)},
		{Type: "Event3", Data: json.RawMessage(`{"index": 3}`)},
	}
	store.SaveBatch(ctx, events)

	// Load events
	loaded, err := store.Load(ctx, 1, 2)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("expected 2 events, got %d", len(loaded))
	}
}

func TestPebbleStore_GetPosition(t *testing.T) {
	store, err := NewPebbleStore(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Initially should be 0
	pos, err := store.GetPosition(ctx)
	if err != nil {
		t.Fatalf("GetPosition failed: %v", err)
	}
	if pos != 0 {
		t.Errorf("expected position 0, got %d", pos)
	}

	// Save some events
	store.Save(ctx, &StoredEvent{Type: "Test", Data: json.RawMessage(`{}`)})
	store.Save(ctx, &StoredEvent{Type: "Test", Data: json.RawMessage(`{}`)})

	pos, err = store.GetPosition(ctx)
	if err != nil {
		t.Fatalf("GetPosition failed: %v", err)
	}
	if pos != 2 {
		t.Errorf("expected position 2, got %d", pos)
	}
}

func TestPebbleStore_SubscriptionPosition(t *testing.T) {
	store, err := NewPebbleStore(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Save subscription position
	if err := store.SaveSubscriptionPosition(ctx, "test-sub", 42); err != nil {
		t.Fatalf("SaveSubscriptionPosition failed: %v", err)
	}

	// Load subscription position
	pos, err := store.LoadSubscriptionPosition(ctx, "test-sub")
	if err != nil {
		t.Fatalf("LoadSubscriptionPosition failed: %v", err)
	}
	if pos != 42 {
		t.Errorf("expected position 42, got %d", pos)
	}

	// Load non-existent subscription
	pos, err = store.LoadSubscriptionPosition(ctx, "non-existent")
	if err != nil {
		t.Fatalf("LoadSubscriptionPosition failed: %v", err)
	}
	if pos != 0 {
		t.Errorf("expected position 0 for non-existent subscription, got %d", pos)
	}
}
