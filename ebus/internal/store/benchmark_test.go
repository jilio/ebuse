package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

func BenchmarkSave(b *testing.B) {
	dbPath := "bench_save.db"
	defer os.Remove(dbPath)

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	event := &StoredEvent{
		Type:      "BenchmarkEvent",
		Data:      json.RawMessage(`{"message":"test"}`),
		Timestamp: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := store.Save(ctx, event); err != nil {
			b.Fatalf("Save failed: %v", err)
		}
	}
}

func BenchmarkSaveBatch(b *testing.B) {
	sizes := []int{10, 100, 1000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			dbPath := fmt.Sprintf("bench_batch_%d.db", size)
			defer os.Remove(dbPath)

			store, err := NewSQLiteStore(dbPath)
			if err != nil {
				b.Fatalf("Failed to create store: %v", err)
			}
			defer store.Close()

			ctx := context.Background()

			// Prepare batch
			events := make([]*StoredEvent, size)
			for i := 0; i < size; i++ {
				events[i] = &StoredEvent{
					Type:      "BenchmarkEvent",
					Data:      json.RawMessage(`{"message":"test"}`),
					Timestamp: time.Now(),
				}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := store.SaveBatch(ctx, events); err != nil {
					b.Fatalf("SaveBatch failed: %v", err)
				}
			}
		})
	}
}

func BenchmarkLoad(b *testing.B) {
	dbPath := "bench_load.db"
	defer os.Remove(dbPath)

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Seed with 10k events
	for i := 0; i < 10000; i++ {
		event := &StoredEvent{
			Type:      "BenchmarkEvent",
			Data:      json.RawMessage(`{"message":"test"}`),
			Timestamp: time.Now(),
		}
		store.Save(ctx, event)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.Load(ctx, 1, -1)
		if err != nil {
			b.Fatalf("Load failed: %v", err)
		}
	}
}

func BenchmarkLoadStream(b *testing.B) {
	dbPath := "bench_stream.db"
	defer os.Remove(dbPath)

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Seed with 10k events
	for i := 0; i < 10000; i++ {
		event := &StoredEvent{
			Type:      "BenchmarkEvent",
			Data:      json.RawMessage(`{"message":"test"}`),
			Timestamp: time.Now(),
		}
		store.Save(ctx, event)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := store.LoadStream(ctx, 1, 1000, func(batch []*StoredEvent) error {
			return nil
		})
		if err != nil {
			b.Fatalf("LoadStream failed: %v", err)
		}
	}
}

func BenchmarkConcurrentSave(b *testing.B) {
	dbPath := "bench_concurrent.db"
	defer os.Remove(dbPath)

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		event := &StoredEvent{
			Type:      "BenchmarkEvent",
			Data:      json.RawMessage(`{"message":"test"}`),
			Timestamp: time.Now(),
		}
		for pb.Next() {
			if err := store.Save(ctx, event); err != nil {
				b.Fatalf("Save failed: %v", err)
			}
		}
	})
}

func BenchmarkGetPosition(b *testing.B) {
	dbPath := "bench_position.db"
	defer os.Remove(dbPath)

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Seed with some events
	for i := 0; i < 1000; i++ {
		event := &StoredEvent{
			Type:      "BenchmarkEvent",
			Data:      json.RawMessage(`{"message":"test"}`),
			Timestamp: time.Now(),
		}
		store.Save(ctx, event)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.GetPosition(ctx)
		if err != nil {
			b.Fatalf("GetPosition failed: %v", err)
		}
	}
}
