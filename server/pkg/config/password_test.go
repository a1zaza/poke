package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArgon2PasswordHash(t *testing.T) {
	encoded, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hashPassword() error = %v", err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$") {
		t.Fatalf("hashPassword() = %q, want Argon2id format", encoded)
	}

	valid, needsMigration, err := verifyPasswordHash("correct horse battery staple", encoded)
	if err != nil || !valid || needsMigration {
		t.Fatalf("verifyPasswordHash(correct) = %v, %v, %v", valid, needsMigration, err)
	}
	valid, _, err = verifyPasswordHash("wrong", encoded)
	if err != nil || valid {
		t.Fatalf("verifyPasswordHash(wrong) = %v, _, %v", valid, err)
	}
}

func TestVerifyPasswordMigratesLegacyPBKDF2(t *testing.T) {
	useTemporaryConfigStorage(t)
	legacyHash := simpleHash("legacy-password", strings.Repeat("a", 32))
	cfg := sampleConfig()
	cfg.Auth.Password = legacyHash
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig() error = %v", err)
	}

	if err := Initialize(); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if !VerifyPassword("legacy-password") {
		t.Fatal("VerifyPassword() rejected the correct legacy password")
	}
	if got := GetConfig().Auth.Password; !strings.HasPrefix(got, "$argon2id$") {
		t.Fatalf("legacy password was not migrated: %q", got)
	}
	if !VerifyPassword("legacy-password") || VerifyPassword("wrong") {
		t.Fatal("migrated password verification returned an unexpected result")
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(data), legacyHash) || !strings.Contains(string(data), "$argon2id$") {
		t.Fatalf("persisted password was not migrated: %s", data)
	}
}

func TestFailedMigrationPreservesLegacyHash(t *testing.T) {
	useTemporaryConfigStorage(t)
	legacyHash := simpleHash("legacy-password", strings.Repeat("b", 32))
	cfg := sampleConfig()
	cfg.Auth.Password = legacyHash
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig() error = %v", err)
	}
	if err := Initialize(); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	configFile = filepath.Join(filepath.Dir(configFile), "missing", "config.json")
	if !VerifyPassword("legacy-password") {
		t.Fatal("VerifyPassword() rejected a valid password when migration persistence failed")
	}
	if got := GetConfig().Auth.Password; got != legacyHash {
		t.Fatalf("failed migration changed the in-memory hash: %q", got)
	}
}

func TestVerifyPasswordHashRejectsUnsafeArgon2Parameters(t *testing.T) {
	encoded := "$argon2id$v=19$m=999999999,t=3,p=1$MTIzNDU2Nzg5MDEyMzQ1Ng$MTIzNDU2Nzg5MDEyMzQ1Ng"
	if valid, _, err := verifyPasswordHash("password", encoded); err == nil || valid {
		t.Fatalf("verifyPasswordHash() = %v, _, %v; want unsafe-parameter error", valid, err)
	}
}
