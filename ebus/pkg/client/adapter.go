package client

import (
	"context"

	eventbus "github.com/jilio/ebu"
	"github.com/jilio/ebus/internal/store"
)

// EventStoreAdapter adapts HTTPClient to implement ebu's EventStore interface
type EventStoreAdapter struct {
	client *HTTPClient
}

// NewEventStoreAdapter creates an adapter that implements ebu's EventStore interface
func NewEventStoreAdapter(baseURL, apiKey string) eventbus.EventStore {
	return &EventStoreAdapter{
		client: New(baseURL, apiKey),
	}
}

// Save implements eventbus.EventStore
func (a *EventStoreAdapter) Save(ctx context.Context, event *eventbus.StoredEvent) error {
	// Convert from ebu's StoredEvent to internal StoredEvent
	storeEvent := &store.StoredEvent{
		Position:  event.Position,
		Type:      event.Type,
		Data:      event.Data,
		Timestamp: event.Timestamp,
	}

	err := a.client.Save(ctx, storeEvent)
	if err != nil {
		return err
	}

	// Update position from server response
	event.Position = storeEvent.Position
	return nil
}

// Load implements eventbus.EventStore
func (a *EventStoreAdapter) Load(ctx context.Context, from, to int64) ([]*eventbus.StoredEvent, error) {
	storeEvents, err := a.client.Load(ctx, from, to)
	if err != nil {
		return nil, err
	}

	// Convert from internal StoredEvent to ebu's StoredEvent
	events := make([]*eventbus.StoredEvent, len(storeEvents))
	for i, se := range storeEvents {
		events[i] = &eventbus.StoredEvent{
			Position:  se.Position,
			Type:      se.Type,
			Data:      se.Data,
			Timestamp: se.Timestamp,
		}
	}

	return events, nil
}

// GetPosition implements eventbus.EventStore
func (a *EventStoreAdapter) GetPosition(ctx context.Context) (int64, error) {
	return a.client.GetPosition(ctx)
}

// SaveSubscriptionPosition implements eventbus.EventStore
func (a *EventStoreAdapter) SaveSubscriptionPosition(ctx context.Context, subscriptionID string, position int64) error {
	return a.client.SaveSubscriptionPosition(ctx, subscriptionID, position)
}

// LoadSubscriptionPosition implements eventbus.EventStore
func (a *EventStoreAdapter) LoadSubscriptionPosition(ctx context.Context, subscriptionID string) (int64, error) {
	return a.client.LoadSubscriptionPosition(ctx, subscriptionID)
}
