package main

import (
	"context"
	"flag"
	"log/slog"
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

	// Setup structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("Starting ebuse server")

	// Load configuration from environment
	config := ebuse.LoadConfigFromEnv()

	var httpHandler http.Handler

	// Check if running in multi-tenant mode
	if *configPath != "" {
		slog.Info("Running in multi-tenant mode", "config_file", *configPath)
		tenantsConfig, err := ebuse.LoadTenantsConfig(*configPath)
		if err != nil {
			slog.Error("Failed to load tenants config", "error", err)
			os.Exit(1)
		}

		tenantManager, err := ebuse.NewTenantManager(tenantsConfig)
		if err != nil {
			slog.Error("Failed to create tenant manager", "error", err)
			os.Exit(1)
		}
		defer tenantManager.Close()

		tenants := tenantManager.GetAllTenants()
		slog.Info("Initialized multi-tenant mode",
			"tenant_count", len(tenantsConfig.Tenants),
			"tenants", tenants,
			"data_dir", tenantsConfig.DataDir)

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
			slog.Error("API_KEY environment variable must be set (or use -config for multi-tenant mode)")
			os.Exit(1)
		}

		slog.Info("Running in single-tenant mode",
			"db_path", config.DBPath,
			"store_backend", config.StoreBackend)

		// Create store based on backend type
		var eventStore store.EventStore
		var err error

		if config.StoreBackend == "sqlite" {
			eventStore, err = store.NewSQLiteStore(config.DBPath)
			if err != nil {
				slog.Error("Failed to create SQLite store", "error", err, "db_path", config.DBPath)
				os.Exit(1)
			}
		} else if config.StoreBackend == "pebble" {
			eventStore, err = store.NewPebbleStore(config.DBPath)
			if err != nil {
				slog.Error("Failed to create PebbleDB store", "error", err, "db_path", config.DBPath)
				os.Exit(1)
			}
		} else {
			slog.Error("Invalid STORE_BACKEND", "backend", config.StoreBackend)
			os.Exit(1)
		}
		defer eventStore.Close()

		// Create server with configuration
		serverConfig := &server.Config{
			RateLimit:  config.RateLimit,
			RateBurst:  config.RateBurst,
			EnableGzip: config.EnableGzip,
		}

		srv := server.NewWithConfig(eventStore, serverConfig, config.APIKey)
		defer srv.Close()
		httpHandler = srv
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
		slog.Info("Server started",
			"port", config.Port,
			"rate_limit", config.RateLimit,
			"rate_burst", config.RateBurst,
			"gzip_enabled", config.EnableGzip,
			"read_timeout", config.ReadTimeout,
			"write_timeout", config.WriteTimeout)

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	slog.Info("Received shutdown signal", "signal", sig.String())

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
	} else {
		slog.Info("Server stopped gracefully")
	}
}
