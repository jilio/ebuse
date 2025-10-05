package main

import (
	"context"
	"flag"
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
	// Parse command-line flags
	configPath := flag.String("config", "", "Path to tenants.yaml for multi-tenant mode")
	flag.Parse()

	log.Println("=== ebuse Server ===")

	// Load configuration from environment
	config := ebuse.LoadConfigFromEnv()

	var httpHandler http.Handler

	// Check if running in multi-tenant mode
	if *configPath != "" {
		log.Printf("Config file: %s", *configPath)
		tenantsConfig, err := ebuse.LoadTenantsConfig(*configPath)
		if err != nil {
			log.Fatalf("Failed to load tenants config: %v", err)
		}

		tenantManager, err := ebuse.NewTenantManager(tenantsConfig)
		if err != nil {
			log.Fatalf("Failed to create tenant manager: %v", err)
		}
		defer tenantManager.Close()

		log.Printf("Initialized %d tenants: %v", len(tenantsConfig.Tenants), tenantManager.GetAllTenants())
		log.Printf("Data directory: %s", tenantsConfig.DataDir)

		serverConfig := &server.Config{
			RateLimit:  config.RateLimit,
			RateBurst:  config.RateBurst,
			EnableGzip: config.EnableGzip,
		}

		srv := server.NewMultiTenant(tenantManager, serverConfig)
		defer srv.Close()
		httpHandler = srv
	} else {
		// Single-tenant mode
		if config.APIKey == "" {
			log.Fatal("API_KEY environment variable must be set (or use -config for multi-tenant mode)")
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
		httpHandler = srv

		log.Printf("Database: %s", config.DBPath)
	}

	// Create HTTP server
	httpServer := &http.Server{
		Addr:         ":" + config.Port,
		Handler:      httpHandler,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		IdleTimeout:  config.IdleTimeout,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server listening on :%s", config.Port)
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
