package main

import (
	"context"
	"fmt"
	"log"
	"time"

	eventbus "github.com/jilio/ebu"
	"github.com/jilio/ebuse/pkg/client"
)

// Example event types
type OrderPlaced struct {
	OrderID  string    `json:"order_id"`
	UserID   string    `json:"user_id"`
	Amount   float64   `json:"amount"`
	PlacedAt time.Time `json:"placed_at"`
}

type OrderShipped struct {
	OrderID    string    `json:"order_id"`
	TrackingID string    `json:"tracking_id"`
	ShippedAt  time.Time `json:"shipped_at"`
}

type OrderDelivered struct {
	OrderID     string    `json:"order_id"`
	DeliveredAt time.Time `json:"delivered_at"`
	Signature   string    `json:"signature"`
}

func main() {
	fmt.Println("=== ebuse + ebu Integration Demo ===")

	// Create remote event store adapter for ebu
	remoteStore := client.NewEventStoreAdapter(
		"http://localhost:8080",
		"test-secret-key",
	)

	// Create event bus with remote persistence
	bus := eventbus.New(
		eventbus.WithStore(remoteStore),
	)

	fmt.Println("✅ Event bus created with remote storage")

	// Subscribe to events
	eventbus.Subscribe(bus, func(e OrderPlaced) {
		fmt.Printf("📦 Order Placed: %s by user %s for $%.2f\n", e.OrderID, e.UserID, e.Amount)
	})

	eventbus.Subscribe(bus, func(e OrderShipped) {
		fmt.Printf("🚚 Order Shipped: %s (tracking: %s)\n", e.OrderID, e.TrackingID)
	})

	eventbus.Subscribe(bus, func(e OrderDelivered) {
		fmt.Printf("✅ Order Delivered: %s at %s (signed by: %s)\n",
			e.OrderID, e.DeliveredAt.Format("15:04"), e.Signature)
	})

	fmt.Println("✅ Subscribed to order events")

	// Publish some events
	fmt.Println("Publishing events...")

	eventbus.Publish(bus, OrderPlaced{
		OrderID:  "ORD-123",
		UserID:   "user-alice",
		Amount:   99.99,
		PlacedAt: time.Now(),
	})

	time.Sleep(100 * time.Millisecond)

	eventbus.Publish(bus, OrderPlaced{
		OrderID:  "ORD-124",
		UserID:   "user-bob",
		Amount:   149.50,
		PlacedAt: time.Now(),
	})

	time.Sleep(100 * time.Millisecond)

	eventbus.Publish(bus, OrderShipped{
		OrderID:    "ORD-123",
		TrackingID: "TRACK-ABC123",
		ShippedAt:  time.Now(),
	})

	time.Sleep(100 * time.Millisecond)

	eventbus.Publish(bus, OrderDelivered{
		OrderID:     "ORD-123",
		DeliveredAt: time.Now(),
		Signature:   "Alice",
	})

	// Wait for all async handlers
	bus.Wait()

	fmt.Println("\n--- Remote Storage Verification ---")

	// Check what was persisted using the adapter
	ctx := context.Background()
	position, err := remoteStore.GetPosition(ctx)
	if err != nil {
		log.Fatalf("Failed to get position: %v", err)
	}

	fmt.Printf("📊 Total events in remote storage: %d\n", position)

	// Load and display events from remote storage
	events, err := remoteStore.Load(ctx, 1, -1)
	if err != nil {
		log.Fatalf("Failed to load events: %v", err)
	}

	fmt.Println("\n🗄️  Events in remote storage:")
	for _, event := range events {
		fmt.Printf("  [%d] %s - %s\n", event.Position, event.Type, event.Timestamp.Format("15:04:05"))
	}

	fmt.Println("\n✅ Integration successful!")
	fmt.Println("\n💡 Key takeaways:")
	fmt.Println("   1. Events are published to local subscribers immediately")
	fmt.Println("   2. Events are persisted to remote ebuse server automatically")
	fmt.Println("   3. Events can be replayed from remote storage")
	fmt.Println("   4. Perfect for event sourcing and CQRS patterns")
}
