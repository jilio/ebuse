package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jilio/ebuse"
	"github.com/jilio/ebuse/internal/store"
	"github.com/jilio/ebuse/pkg/server"
)

func main() {
	log.Println("=== ebuse Server ===")

	// Load configuration from environment
	config := ebuse.LoadConfigFromEnv()

	if config.APIKey == "" {
		log.Fatal("API_KEY environment variable must be set")
	}

	// Create SQLite store
	sqliteStore, err := store.NewSQLiteStore(config.DBPath)
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}
	defer sqliteStore.Close()

	// Create server with configuration
	serverConfig := &server.Config{
		RateLimit:  config.RateLimit,
		RateBurst:  config.RateBurst,
		EnableGzip: config.EnableGzip,
	}

	srv := server.NewWithConfig(sqliteStore, serverConfig, config.APIKey)
	defer srv.Close()

	// Create HTTP server
	httpServer := &http.Server{
		Addr:         ":" + config.Port,
		Handler:      srv,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		IdleTimeout:  config.IdleTimeout,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server listening on :%s", config.Port)
		log.Printf("Database: %s", config.DBPath)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}
