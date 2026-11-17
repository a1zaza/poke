package config

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func captureConfigOutput(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = previous; _ = writer.Close(); _ = reader.Close() }()
	run()
	_ = writer.Close()
	os.Stdout = previous
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func reloadAccount(t *testing.T) {
	t.Helper()
	mu.Lock()
	instance = nil
	configLoadErr = nil
	mu.Unlock()
	if err := Initialize(); err != nil {
		t.Fatal(err)
	}
}

func TestInitialUsernameAndPasswordPrintedOnlyOnce(t *testing.T) {
	useTemporaryConfigStorage(t)
	output := captureConfigOutput(t, func() {
		if err := Initialize(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "Initial Admin Username: admin\n") {
		t.Fatal("initial username was not printed")
	}
	_, passwordLine, found := strings.Cut(output, "Initial Admin Password: ")
	if !found {
		t.Fatal("initial password was not printed")
	}
	password := strings.SplitN(passwordLine, "\n", 2)[0]
	if !VerifyCredentials("admin", password) || VerifyCredentials("", password) || VerifyCredentials("other", password) || VerifyCredentials("admin", "wrong") {
		t.Fatal("username/password verification failed")
	}
	if strings.Contains(GetConfig().Auth.Password, password) {
		t.Fatal("plaintext password persisted")
	}
	restarted := captureConfigOutput(t, func() { reloadAccount(t) })
	if strings.Contains(restarted, "Initial Admin") {
		t.Fatal("credentials printed again after restart")
	}
	if !VerifyCredentials("admin", password) {
		t.Fatal("restart changed administrator credentials")
	}
	assertFileMode(t, configFile, 0600)
}

func TestLegacyConfigGetsDefaultUsernameWithoutPasswordReset(t *testing.T) {
	useTemporaryConfigStorage(t)
	password, err := hashPassword("legacy-login-password")
	if err != nil {
		t.Fatal(err)
	}
	legacy := sampleConfig()
	legacy.Auth.Password = password
	legacy.Auth.Username = ""
	if err := saveConfig(legacy); err != nil {
		t.Fatal(err)
	}
	output := captureConfigOutput(t, func() { reloadAccount(t) })
	if strings.Contains(output, "Initial Admin") {
		t.Fatal("migration printed initialization credentials")
	}
	if cfg := GetConfig(); cfg.Auth.Username != "admin" || cfg.Auth.Password != password {
		t.Fatal("migration changed the password or missed the username")
	}
	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatal(err)
	}
	var persisted Config
	if err = json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Auth.Username != "admin" || !VerifyCredentials("admin", "legacy-login-password") {
		t.Fatal("default username migration was not persisted")
	}
}

func TestRenameAdministratorPersistsWithoutChangingPassword(t *testing.T) {
	useTemporaryConfigStorage(t)
	captureConfigOutput(t, func() {
		if err := Initialize(); err != nil {
			t.Fatal(err)
		}
	})
	if err := UpdateConfig(map[string]interface{}{"password": "test-password"}); err != nil {
		t.Fatal(err)
	}
	before := GetConfig().Auth.Password
	if err := UpdateConfig(map[string]interface{}{"username": "operator.dev"}); err != nil {
		t.Fatal(err)
	}
	reloadAccount(t)
	if GetConfig().Auth.Password != before || !VerifyCredentials("operator.dev", "test-password") || VerifyCredentials("admin", "test-password") {
		t.Fatal("username change did not preserve the password or revoke the old login name")
	}
	for _, invalid := range []interface{}{"", " ", "a b", "../admin", strings.Repeat("x", 65), "admin\x00", 42} {
		if err := UpdateConfig(map[string]interface{}{"username": invalid, "hostname": "must-not-change"}); err == nil {
			t.Fatal("invalid username accepted")
		}
		if cfg := GetConfig(); cfg.Auth.Username != "operator.dev" || cfg.System.Hostname == "must-not-change" {
			t.Fatal("invalid update modified settings")
		}
	}
}

func TestRenameRollbackOnPersistenceFailure(t *testing.T) {
	useTemporaryConfigStorage(t)
	captureConfigOutput(t, func() {
		if err := Initialize(); err != nil {
			t.Fatal(err)
		}
	})
	mu.Lock()
	configFile += "/invalid-child"
	mu.Unlock()
	if err := UpdateConfig(map[string]interface{}{"username": "new-admin"}); err == nil {
		t.Fatal("expected persistence error")
	}
	if GetConfig().Auth.Username != "admin" {
		t.Fatal("failed save changed in-memory username")
	}
}
