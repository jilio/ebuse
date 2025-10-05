package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jilio/ebuse/internal/store"
	"github.com/jilio/ebuse/pkg/client"
)

// Example event types
type UserCreated struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type UserUpdated struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updated_at"`
}

func main() {
	// Create remote event store client
	// Make sure ebuse server is running: API_KEY=secret-key go run ./cmd/ebuse
	remoteStore := client.New(
		"http://localhost:8080",
		"secret-key",
	)

	// Test 1: Save events
	fmt.Println("=== Saving Events ===")
	events := []struct {
		Type string
		Data any
	}{
		{"UserCreated", UserCreated{ID: "1", Name: "Alice", CreatedAt: time.Now()}},
		{"UserCreated", UserCreated{ID: "2", Name: "Bob", CreatedAt: time.Now()}},
		{"UserUpdated", UserUpdated{ID: "1", Name: "Alice Smith", UpdatedAt: time.Now()}},
	}

	ctx := context.Background()
	for _, e := range events {
		data, _ := json.Marshal(e.Data)
		stored := &store.StoredEvent{
			Type:      e.Type,
			Data:      data,
			Timestamp: time.Now(),
		}

		if err := remoteStore.Save(ctx, stored); err != nil {
			log.Fatalf("Failed to save event: %v", err)
		}
		fmt.Printf("Saved %s with position %d\n", e.Type, stored.Position)
	}

	// Test 2: Get current position
	fmt.Println("\n=== Current Position ===")
	pos, err := remoteStore.GetPosition(ctx)
	if err != nil {
		log.Fatalf("Failed to get position: %v", err)
	}
	fmt.Printf("Current position: %d\n", pos)

	// Test 3: Load events
	fmt.Println("\n=== Loading Events ===")
	loadedEvents, err := remoteStore.Load(ctx, 1, -1)
	if err != nil {
		log.Fatalf("Failed to load events: %v", err)
	}

	for _, e := range loadedEvents {
		fmt.Printf("Position %d: %s - %s\n", e.Position, e.Type, string(e.Data))
	}

	// Test 4: Subscription positions
	fmt.Println("\n=== Subscription Positions ===")
	subID := "user-events-processor"

	// Save subscription position
	if err := remoteStore.SaveSubscriptionPosition(ctx, subID, 2); err != nil {
		log.Fatalf("Failed to save subscription position: %v", err)
	}
	fmt.Printf("Saved subscription position for '%s'\n", subID)

	// Load subscription position
	lastPos, err := remoteStore.LoadSubscriptionPosition(ctx, subID)
	if err != nil {
		log.Fatalf("Failed to load subscription position: %v", err)
	}
	fmt.Printf("Last processed position for '%s': %d\n", subID, lastPos)

	// Load events since last position
	fmt.Println("\n=== Events Since Last Position ===")
	newEvents, err := remoteStore.Load(ctx, lastPos+1, -1)
	if err != nil {
		log.Fatalf("Failed to load new events: %v", err)
	}

	if len(newEvents) > 0 {
		for _, e := range newEvents {
			fmt.Printf("New event at position %d: %s\n", e.Position, e.Type)
		}
	} else {
		fmt.Println("No new events since last position")
	}
}
