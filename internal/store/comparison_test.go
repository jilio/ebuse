package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// Benchmark SQLite vs PebbleDB for batch writes
func BenchmarkSQLite_BatchWrite(b *testing.B) {
	store, err := NewSQLiteStore(b.TempDir() + "/sqlite.db")
	if err != nil {
		b.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		events := make([]*StoredEvent, 100)
		for j := 0; j < 100; j++ {
			events[j] = &StoredEvent{
				Type: "BenchEvent",
				Data: json.RawMessage(fmt.Sprintf(`{"index": %d}`, j)),
			}
		}
		if err := store.SaveBatch(ctx, events); err != nil {
			b.Fatalf("SaveBatch failed: %v", err)
		}
	}
}

func BenchmarkPebble_BatchWrite(b *testing.B) {
	store, err := NewPebbleStore(b.TempDir() + "/pebble.db")
	if err != nil {
		b.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		events := make([]*StoredEvent, 100)
		for j := 0; j < 100; j++ {
			events[j] = &StoredEvent{
				Type: "BenchEvent",
				Data: json.RawMessage(fmt.Sprintf(`{"index": %d}`, j)),
			}
		}
		if err := store.SaveBatch(ctx, events); err != nil {
			b.Fatalf("SaveBatch failed: %v", err)
		}
	}
}

// Benchmark single event writes
func BenchmarkSQLite_SingleWrite(b *testing.B) {
	store, err := NewSQLiteStore(b.TempDir() + "/sqlite.db")
	if err != nil {
		b.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		event := &StoredEvent{
			Type: "BenchEvent",
			Data: json.RawMessage(fmt.Sprintf(`{"index": %d}`, i)),
		}
		if err := store.Save(ctx, event); err != nil {
			b.Fatalf("Save failed: %v", err)
		}
	}
}

func BenchmarkPebble_SingleWrite(b *testing.B) {
	store, err := NewPebbleStore(b.TempDir() + "/pebble.db")
	if err != nil {
		b.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		event := &StoredEvent{
			Type: "BenchEvent",
			Data: json.RawMessage(fmt.Sprintf(`{"index": %d}`, i)),
		}
		if err := store.Save(ctx, event); err != nil {
			b.Fatalf("Save failed: %v", err)
		}
	}
}

// Benchmark sequential reads
func BenchmarkSQLite_SequentialRead(b *testing.B) {
	store, err := NewSQLiteStore(b.TempDir() + "/sqlite.db")
	if err != nil {
		b.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Pre-populate with 1000 events
	events := make([]*StoredEvent, 1000)
	for i := 0; i < 1000; i++ {
		events[i] = &StoredEvent{
			Type: "BenchEvent",
			Data: json.RawMessage(fmt.Sprintf(`{"index": %d}`, i)),
		}
	}
	store.SaveBatch(ctx, events)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.Load(ctx, 1, 100)
		if err != nil {
			b.Fatalf("Load failed: %v", err)
		}
	}
}

func BenchmarkPebble_SequentialRead(b *testing.B) {
	store, err := NewPebbleStore(b.TempDir() + "/pebble.db")
	if err != nil {
		b.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Pre-populate with 1000 events
	events := make([]*StoredEvent, 1000)
	for i := 0; i < 1000; i++ {
		events[i] = &StoredEvent{
			Type: "BenchEvent",
			Data: json.RawMessage(fmt.Sprintf(`{"index": %d}`, i)),
		}
	}
	store.SaveBatch(ctx, events)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.Load(ctx, 1, 100)
		if err != nil {
			b.Fatalf("Load failed: %v", err)
		}
	}
}
