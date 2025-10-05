package store

import "context"

// EventStore defines the interface for event storage backends
type EventStore interface {
	Save(ctx context.Context, event *StoredEvent) error
	SaveBatch(ctx context.Context, events []*StoredEvent) error
	Load(ctx context.Context, from, to int64) ([]*StoredEvent, error)
	LoadStream(ctx context.Context, from int64, batchSize int, handler func([]*StoredEvent) error) error
	GetPosition(ctx context.Context) (int64, error)
	SaveSubscriptionPosition(ctx context.Context, subscriptionID string, position int64) error
	LoadSubscriptionPosition(ctx context.Context, subscriptionID string) (int64, error)
	Close() error
}
