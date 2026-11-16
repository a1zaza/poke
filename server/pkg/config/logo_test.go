package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogoPersistence(t *testing.T) {
	useTemporaryConfigStorage(t)
	path := filepath.Join(Directory(), "logo.png")
	for _, value := range []string{"first", "replacement"} {
		if err := SaveLogo([]byte(value)); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != value {
			t.Fatalf("logo %q: %v", data, err)
		}
		assertFileMode(t, path, 0600)
	}
	if err := ResetLogo(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("logo not removed: %v", err)
	}
	if err := ResetLogo(); err != nil {
		t.Fatal(err)
	}
	// A non-empty directory cannot be overwritten or removed as a logo file.
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "child"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveLogo([]byte("x")); err == nil {
		t.Fatal("save failure ignored")
	}
	if err := ResetLogo(); err == nil {
		t.Fatal("reset failure ignored")
	}
}
