package store

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/cockroachdb/pebble"
)

// PebbleStore implements EventStore using PebbleDB (LSM-tree based key-value store)
type PebbleStore struct {
	db       *pebble.DB
	mu       sync.RWMutex
	position atomic.Int64 // Atomic counter for event positions
}

// Key prefixes for different data types
const (
	eventPrefix        = byte(0x01) // event:<position> -> event data
	positionKey        = "meta:position"
	subscriptionPrefix = byte(0x02) // sub:<subscription_id> -> position
)

// NewPebbleStore creates a new PebbleDB-based event store
func NewPebbleStore(dbPath string) (*PebbleStore, error) {
	opts := &pebble.Options{
		// Memory and cache settings (optimized for write-heavy workloads)
		MemTableSize:             64 << 20,  // 64MB memtable
		MemTableStopWritesThreshold: 4,
		L0CompactionThreshold:    2,
		L0StopWritesThreshold:    12,
		LBaseMaxBytes:            128 << 20, // 128MB
		MaxOpenFiles:             1000,

		// Write buffer and compaction
		BytesPerSync:             512 << 10, // 512KB
		MaxConcurrentCompactions: func() int { return 3 },

		// Disable WAL for maximum write performance (data in memtable)
		// Note: This sacrifices durability for performance
		DisableWAL:               false, // Keep WAL enabled for safety
	}

	db, err := pebble.Open(dbPath, opts)
	if err != nil {
		return nil, fmt.Errorf("open pebble db: %w", err)
	}

	s := &PebbleStore{
		db: db,
	}

	// Initialize position counter from existing data
	if err := s.initializePosition(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize position: %w", err)
	}

	return s, nil
}

func (s *PebbleStore) initializePosition() error {
	// Find the highest position by seeking to the last event
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte{eventPrefix},
		UpperBound: []byte{eventPrefix + 1},
	})
	if err != nil {
		return err
	}
	defer iter.Close()

	// Seek to last event
	if iter.Last() {
		key := iter.Key()
		if len(key) >= 9 { // prefix(1) + position(8)
			pos := int64(binary.BigEndian.Uint64(key[1:]))
			s.position.Store(pos)
		}
	}

	return nil
}

func eventKey(position int64) []byte {
	key := make([]byte, 9) // 1 byte prefix + 8 bytes position
	key[0] = eventPrefix
	binary.BigEndian.PutUint64(key[1:], uint64(position))
	return key
}

func subscriptionKey(subscriptionID string) []byte {
	key := make([]byte, 1+len(subscriptionID))
	key[0] = subscriptionPrefix
	copy(key[1:], subscriptionID)
	return key
}

// Save implements EventStore.Save
func (s *PebbleStore) Save(ctx context.Context, event *StoredEvent) error {
	// Assign next position atomically
	position := s.position.Add(1)
	event.Position = position

	// Serialize event
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	// Write to PebbleDB
	if err := s.db.Set(eventKey(position), data, pebble.Sync); err != nil {
		return fmt.Errorf("write event: %w", err)
	}

	return nil
}

// SaveBatch saves multiple events in a single batch for better performance
func (s *PebbleStore) SaveBatch(ctx context.Context, events []*StoredEvent) error {
	if len(events) == 0 {
		return nil
	}

	batch := s.db.NewBatch()
	defer batch.Close()

	for _, event := range events {
		// Assign next position atomically
		position := s.position.Add(1)
		event.Position = position

		// Serialize event
		data, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("marshal event: %w", err)
		}

		// Add to batch
		if err := batch.Set(eventKey(position), data, nil); err != nil {
			return fmt.Errorf("batch set: %w", err)
		}
	}

	// Commit batch with sync
	if err := batch.Commit(pebble.Sync); err != nil {
		return fmt.Errorf("commit batch: %w", err)
	}

	return nil
}

// Load implements EventStore.Load
func (s *PebbleStore) Load(ctx context.Context, from, to int64) ([]*StoredEvent, error) {
	var events []*StoredEvent

	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: eventKey(from),
		UpperBound: eventKey(to + 1), // Exclusive upper bound
	})
	if err != nil {
		return nil, fmt.Errorf("create iterator: %w", err)
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		var event StoredEvent
		if err := json.Unmarshal(iter.Value(), &event); err != nil {
			return nil, fmt.Errorf("unmarshal event: %w", err)
		}
		events = append(events, &event)
	}

	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("iterator error: %w", err)
	}

	return events, nil
}

// LoadStream implements EventStore.LoadStream for efficient streaming
func (s *PebbleStore) LoadStream(ctx context.Context, from int64, batchSize int, handler func([]*StoredEvent) error) error {
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: eventKey(from),
		UpperBound: []byte{eventPrefix + 1},
	})
	if err != nil {
		return fmt.Errorf("create iterator: %w", err)
	}
	defer iter.Close()

	batch := make([]*StoredEvent, 0, batchSize)

	for iter.First(); iter.Valid(); iter.Next() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var event StoredEvent
		if err := json.Unmarshal(iter.Value(), &event); err != nil {
			return fmt.Errorf("unmarshal event: %w", err)
		}

		batch = append(batch, &event)

		if len(batch) >= batchSize {
			if err := handler(batch); err != nil {
				return err
			}
			batch = batch[:0] // Reset slice
		}
	}

	// Handle remaining events
	if len(batch) > 0 {
		if err := handler(batch); err != nil {
			return err
		}
	}

	return iter.Error()
}

// GetPosition implements EventStore.GetPosition
func (s *PebbleStore) GetPosition(ctx context.Context) (int64, error) {
	return s.position.Load(), nil
}

// SaveSubscriptionPosition implements EventStore.SaveSubscriptionPosition
func (s *PebbleStore) SaveSubscriptionPosition(ctx context.Context, subscriptionID string, position int64) error {
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, uint64(position))

	if err := s.db.Set(subscriptionKey(subscriptionID), data, pebble.Sync); err != nil {
		return fmt.Errorf("save subscription position: %w", err)
	}

	return nil
}

// LoadSubscriptionPosition implements EventStore.LoadSubscriptionPosition
func (s *PebbleStore) LoadSubscriptionPosition(ctx context.Context, subscriptionID string) (int64, error) {
	data, closer, err := s.db.Get(subscriptionKey(subscriptionID))
	if err == pebble.ErrNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get subscription position: %w", err)
	}
	defer closer.Close()

	if len(data) != 8 {
		return 0, fmt.Errorf("invalid subscription data length: %d", len(data))
	}

	position := int64(binary.BigEndian.Uint64(data))
	return position, nil
}

// Close implements EventStore.Close
func (s *PebbleStore) Close() error {
	return s.db.Close()
}
