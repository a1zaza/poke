package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveConfigSeparatesSecretsAndUsesPrivatePermissions(t *testing.T) {
	useTemporaryConfigStorage(t)
	cfg := sampleConfig()

	if err := saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig() error = %v", err)
	}

	configData, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("ReadFile(config) error = %v", err)
	}
	if bytes.Contains(configData, []byte("registry-secret")) {
		t.Fatalf("config file contains plaintext service credentials: %s", configData)
	}

	secretsData, err := os.ReadFile(secretsFile)
	if err != nil {
		t.Fatalf("ReadFile(secrets) error = %v", err)
	}
	if !bytes.Contains(secretsData, []byte("registry-secret")) {
		t.Fatalf("secrets file does not contain expected credentials: %s", secretsData)
	}

	assertFileMode(t, configFile, 0600)
	assertFileMode(t, secretsFile, 0600)
}

func TestInitializeMigratesLegacyInlineSecrets(t *testing.T) {
	useTemporaryConfigStorage(t)
	legacyData, err := json.Marshal(sampleConfig())
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(configFile, legacyData, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := Initialize(); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if got := GetConfig(); got.System.Registries[0].Password != "registry-secret" {
		t.Fatalf("migrated credentials were not restored in memory: %+v", got.System)
	}

	configData, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("ReadFile(config) error = %v", err)
	}
	if bytes.Contains(configData, []byte("registry-secret")) {
		t.Fatalf("legacy credentials remain in config file: %s", configData)
	}
	assertFileMode(t, configFile, 0600)
	assertFileMode(t, secretsFile, 0600)
}

func TestInvalidConfigIsNotOverwritten(t *testing.T) {
	useTemporaryConfigStorage(t)
	invalidData := []byte("{invalid-json")
	if err := os.WriteFile(configFile, invalidData, 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := Initialize(); err == nil {
		t.Fatal("Initialize() accepted invalid JSON")
	}
	if err := UpdateConfig(map[string]interface{}{"hostname": "changed"}); err == nil {
		t.Fatal("UpdateConfig() overwrote an invalid configuration")
	}

	after, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(after, invalidData) {
		t.Fatalf("invalid config was modified: %q", after)
	}
}

func TestInitializeRejectsSymlinkConfig(t *testing.T) {
	useTemporaryConfigStorage(t)
	target := filepath.Join(filepath.Dir(configFile), "outside.json")
	if err := os.WriteFile(target, []byte(`{"auth":{}}`), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Symlink(target, configFile); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	if err := Initialize(); err == nil {
		t.Fatal("Initialize() accepted a symbolic-link config file")
	}
}

func sampleConfig() *Config {
	return &Config{
		Auth: AuthConfig{Password: "admin-hash"},
		System: SystemConfig{
			Registries: []RegistryConfig{{Domain: "registry.example", Username: "user", Password: "registry-secret"}},
		},
	}
}

func useTemporaryConfigStorage(t *testing.T) {
	t.Helper()
	dir := t.TempDir()

	mu.Lock()
	previousConfigFile := configFile
	previousSecretsFile := secretsFile
	previousLoadErr := configLoadErr
	previousInstance := instance
	configFile = filepath.Join(dir, "config.json")
	secretsFile = filepath.Join(dir, "secrets.json")
	configLoadErr = nil
	instance = nil
	mu.Unlock()

	t.Cleanup(func() {
		mu.Lock()
		configFile = previousConfigFile
		secretsFile = previousSecretsFile
		configLoadErr = previousLoadErr
		instance = previousInstance
		mu.Unlock()
	})
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %q = %04o, want %04o", path, got, want)
	}
}
