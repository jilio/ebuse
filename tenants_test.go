package ebuse

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTenantsConfig(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "tenants.yaml")

	configData := `
tenants:
  - name: tenant1
    api_key: key1
  - name: tenant2
    api_key: key2
data_dir: /tmp/test-data
`
	if err := os.WriteFile(configPath, []byte(configData), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	config, err := LoadTenantsConfig(configPath)
	if err != nil {
		t.Fatalf("LoadTenantsConfig failed: %v", err)
	}

	if len(config.Tenants) != 2 {
		t.Errorf("expected 2 tenants, got %d", len(config.Tenants))
	}

	if config.Tenants[0].Name != "tenant1" {
		t.Errorf("expected tenant1, got %s", config.Tenants[0].Name)
	}

	if config.Tenants[0].APIKey != "key1" {
		t.Errorf("expected key1, got %s", config.Tenants[0].APIKey)
	}

	if config.DataDir != "/tmp/test-data" {
		t.Errorf("expected /tmp/test-data, got %s", config.DataDir)
	}
}

func TestLoadTenantsConfig_DefaultDataDir(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "tenants.yaml")

	configData := `
tenants:
  - name: tenant1
    api_key: key1
`
	if err := os.WriteFile(configPath, []byte(configData), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	config, err := LoadTenantsConfig(configPath)
	if err != nil {
		t.Fatalf("LoadTenantsConfig failed: %v", err)
	}

	if config.DataDir != "data" {
		t.Errorf("expected default data dir 'data', got %s", config.DataDir)
	}
}

func TestLoadTenantsConfig_NoTenants(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "tenants.yaml")

	configData := `
tenants: []
`
	if err := os.WriteFile(configPath, []byte(configData), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := LoadTenantsConfig(configPath)
	if err == nil {
		t.Fatal("expected error for empty tenants, got nil")
	}
}

func TestLoadTenantsConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "tenants.yaml")

	configData := `invalid: [yaml: content`
	if err := os.WriteFile(configPath, []byte(configData), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := LoadTenantsConfig(configPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoadTenantsConfig_FileNotFound(t *testing.T) {
	_, err := LoadTenantsConfig("/nonexistent/path/tenants.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestNewTenantManager(t *testing.T) {
	tmpDir := t.TempDir()

	config := &TenantsConfig{
		Tenants: []TenantConfig{
			{Name: "tenant1", APIKey: "key1"},
			{Name: "tenant2", APIKey: "key2"},
		},
		DataDir: tmpDir,
	}

	tm, err := NewTenantManager(config)
	if err != nil {
		t.Fatalf("NewTenantManager failed: %v", err)
	}
	defer tm.Close()

	// Verify both tenants are loaded
	if len(tm.tenants) != 2 {
		t.Errorf("expected 2 tenants, got %d", len(tm.tenants))
	}

	// Verify database files exist
	db1 := filepath.Join(tmpDir, "tenant1.db")
	db2 := filepath.Join(tmpDir, "tenant2.db")

	if _, err := os.Stat(db1); os.IsNotExist(err) {
		t.Errorf("expected database file %s to exist", db1)
	}

	if _, err := os.Stat(db2); os.IsNotExist(err) {
		t.Errorf("expected database file %s to exist", db2)
	}
}

func TestNewTenantManager_EmptyTenantName(t *testing.T) {
	tmpDir := t.TempDir()

	config := &TenantsConfig{
		Tenants: []TenantConfig{
			{Name: "", APIKey: "key1"},
		},
		DataDir: tmpDir,
	}

	_, err := NewTenantManager(config)
	if err == nil {
		t.Fatal("expected error for empty tenant name, got nil")
	}
}

func TestNewTenantManager_InvalidTenantName(t *testing.T) {
	tmpDir := t.TempDir()

	testCases := []string{
		"../etc/passwd",
		"tenant/name",
		"tenant\\name",
		"tenant name",
		"tenant@name",
	}

	for _, invalidName := range testCases {
		config := &TenantsConfig{
			Tenants: []TenantConfig{
				{Name: invalidName, APIKey: "key1"},
			},
			DataDir: tmpDir,
		}

		_, err := NewTenantManager(config)
		if err == nil {
			t.Errorf("expected error for invalid tenant name %q, got nil", invalidName)
		}
	}
}

func TestNewTenantManager_TenantNameTooLong(t *testing.T) {
	tmpDir := t.TempDir()

	longName := ""
	for i := 0; i < 101; i++ {
		longName += "a"
	}

	config := &TenantsConfig{
		Tenants: []TenantConfig{
			{Name: longName, APIKey: "key1"},
		},
		DataDir: tmpDir,
	}

	_, err := NewTenantManager(config)
	if err == nil {
		t.Fatal("expected error for tenant name too long, got nil")
	}
}

func TestNewTenantManager_EmptyAPIKey(t *testing.T) {
	tmpDir := t.TempDir()

	config := &TenantsConfig{
		Tenants: []TenantConfig{
			{Name: "tenant1", APIKey: ""},
		},
		DataDir: tmpDir,
	}

	_, err := NewTenantManager(config)
	if err == nil {
		t.Fatal("expected error for empty API key, got nil")
	}
}

func TestNewTenantManager_DuplicateAPIKey(t *testing.T) {
	tmpDir := t.TempDir()

	config := &TenantsConfig{
		Tenants: []TenantConfig{
			{Name: "tenant1", APIKey: "same-key"},
			{Name: "tenant2", APIKey: "same-key"},
		},
		DataDir: tmpDir,
	}

	_, err := NewTenantManager(config)
	if err == nil {
		t.Fatal("expected error for duplicate API key, got nil")
	}
}

func TestTenantManager_GetStore(t *testing.T) {
	tmpDir := t.TempDir()

	config := &TenantsConfig{
		Tenants: []TenantConfig{
			{Name: "tenant1", APIKey: "key1"},
		},
		DataDir: tmpDir,
	}

	tm, err := NewTenantManager(config)
	if err != nil {
		t.Fatalf("NewTenantManager failed: %v", err)
	}
	defer tm.Close()

	// Valid API key
	store, name, ok := tm.GetStore("key1")
	if !ok {
		t.Fatal("expected to find tenant with key1")
	}
	if name != "tenant1" {
		t.Errorf("expected tenant1, got %s", name)
	}
	if store == nil {
		t.Error("expected non-nil store")
	}

	// Invalid API key
	_, _, ok = tm.GetStore("invalid-key")
	if ok {
		t.Error("expected not to find tenant with invalid key")
	}
}

func TestTenantManager_GetAllTenants(t *testing.T) {
	tmpDir := t.TempDir()

	config := &TenantsConfig{
		Tenants: []TenantConfig{
			{Name: "tenant1", APIKey: "key1"},
			{Name: "tenant2", APIKey: "key2"},
			{Name: "tenant3", APIKey: "key3"},
		},
		DataDir: tmpDir,
	}

	tm, err := NewTenantManager(config)
	if err != nil {
		t.Fatalf("NewTenantManager failed: %v", err)
	}
	defer tm.Close()

	tenants := tm.GetAllTenants()
	if len(tenants) != 3 {
		t.Errorf("expected 3 tenants, got %d", len(tenants))
	}

	// Check all tenant names are present
	nameSet := make(map[string]bool)
	for _, name := range tenants {
		nameSet[name] = true
	}

	for _, expected := range []string{"tenant1", "tenant2", "tenant3"} {
		if !nameSet[expected] {
			t.Errorf("expected to find tenant %s", expected)
		}
	}
}

func TestTenantManager_Close(t *testing.T) {
	tmpDir := t.TempDir()

	config := &TenantsConfig{
		Tenants: []TenantConfig{
			{Name: "tenant1", APIKey: "key1"},
			{Name: "tenant2", APIKey: "key2"},
		},
		DataDir: tmpDir,
	}

	tm, err := NewTenantManager(config)
	if err != nil {
		t.Fatalf("NewTenantManager failed: %v", err)
	}

	// Close should not return error
	err = tm.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestValidTenantName(t *testing.T) {
	validNames := []string{
		"tenant1",
		"tenant-1",
		"tenant_1",
		"TENANT",
		"Tenant-Name_123",
	}

	for _, name := range validNames {
		if !validTenantName.MatchString(name) {
			t.Errorf("expected %q to be valid", name)
		}
	}

	invalidNames := []string{
		"tenant/1",
		"tenant\\1",
		"tenant 1",
		"tenant@1",
		"tenant.1",
		"../tenant",
		"",
	}

	for _, name := range invalidNames {
		if validTenantName.MatchString(name) {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}
