package ebuse

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/jilio/ebuse/internal/store"
)

// TenantConfig represents a single tenant with their API key and database
type TenantConfig struct {
	Name   string `yaml:"name"`
	APIKey string `yaml:"api_key"`
}

// TenantsConfig holds all tenant configurations
type TenantsConfig struct {
	Tenants []TenantConfig `yaml:"tenants"`
	DataDir string         `yaml:"data_dir,omitempty"` // Optional: directory for databases
}

// TenantManager manages multiple tenants and their isolated databases
type TenantManager struct {
	mu      sync.RWMutex
	tenants map[string]*TenantStore // API key -> TenantStore
	dataDir string
}

// TenantStore holds a tenant's database and metadata
type TenantStore struct {
	Name  string
	Store *store.SQLiteStore
}

// LoadTenantsConfig loads tenant configuration from YAML file
func LoadTenantsConfig(configPath string) (*TenantsConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var config TenantsConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	if len(config.Tenants) == 0 {
		return nil, fmt.Errorf("no tenants configured")
	}

	// Default data directory
	if config.DataDir == "" {
		config.DataDir = "data"
	}

	return &config, nil
}

// NewTenantManager creates a new tenant manager from config
func NewTenantManager(config *TenantsConfig) (*TenantManager, error) {
	tm := &TenantManager{
		tenants: make(map[string]*TenantStore),
		dataDir: config.DataDir,
	}

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(config.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	// Initialize each tenant's database
	for _, tenant := range config.Tenants {
		if tenant.Name == "" {
			return nil, fmt.Errorf("tenant name cannot be empty")
		}
		if tenant.APIKey == "" {
			return nil, fmt.Errorf("tenant %s: API key cannot be empty", tenant.Name)
		}

		// Check for duplicate API keys
		if _, exists := tm.tenants[tenant.APIKey]; exists {
			return nil, fmt.Errorf("duplicate API key for tenant: %s", tenant.Name)
		}

		// Create database for tenant
		dbPath := filepath.Join(config.DataDir, fmt.Sprintf("%s.db", tenant.Name))
		sqliteStore, err := store.NewSQLiteStore(dbPath)
		if err != nil {
			return nil, fmt.Errorf("create store for tenant %s: %w", tenant.Name, err)
		}

		tm.tenants[tenant.APIKey] = &TenantStore{
			Name:  tenant.Name,
			Store: sqliteStore,
		}
	}

	return tm, nil
}

// GetStore returns the store for a given API key
func (tm *TenantManager) GetStore(apiKey string) (*store.SQLiteStore, string, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tenant, ok := tm.tenants[apiKey]
	if !ok {
		return nil, "", false
	}

	return tenant.Store, tenant.Name, true
}

// GetAllTenants returns a list of all tenant names
func (tm *TenantManager) GetAllTenants() []string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	names := make([]string, 0, len(tm.tenants))
	for _, tenant := range tm.tenants {
		names = append(names, tenant.Name)
	}
	return names
}

// Close closes all tenant databases
func (tm *TenantManager) Close() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	var lastErr error
	for _, tenant := range tm.tenants {
		if err := tenant.Store.Close(); err != nil {
			lastErr = err
		}
	}

	return lastErr
}
