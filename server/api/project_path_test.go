package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateProjectName(t *testing.T) {
	valid := []string{"app", "app-1", "app_1", "app.test", "A1", strings.Repeat("a", 64)}
	for _, name := range valid {
		if err := validateProjectName(name); err != nil {
			t.Errorf("validateProjectName(%q) error = %v", name, err)
		}
	}

	invalid := []string{"", ".", "..", "../escape", "a/b", `a\b`, "%2e%2e", "..%2fescape", "项目", "-app", strings.Repeat("a", 65)}
	for _, name := range invalid {
		if err := validateProjectName(name); err == nil {
			t.Errorf("validateProjectName(%q) error = nil", name)
		}
	}
}

func TestIsPathWithinRoot(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "poke", "projects")
	tests := []struct {
		path string
		want bool
	}{
		{path: filepath.Join(root, "app"), want: true},
		{path: filepath.Join(root, "nested", "app"), want: true},
		{path: root, want: false},
		{path: filepath.Clean(filepath.Join(root, "..")), want: false},
		{path: root + "-evil/app", want: false},
	}

	for _, tt := range tests {
		if got := isPathWithinRoot(root, tt.path); got != tt.want {
			t.Errorf("isPathWithinRoot(%q, %q) = %v, want %v", root, tt.path, got, tt.want)
		}
	}
}

func TestValidateManagedPathRejectsSymlinkEscape(t *testing.T) {
	tempDir := t.TempDir()
	root := filepath.Join(tempDir, "projects")
	projectDir := filepath.Join(root, "app")
	outsideDir := filepath.Join(tempDir, "outside")
	outsideChild := filepath.Join(outsideDir, "child")

	for _, dir := range []string{projectDir, outsideChild} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", dir, err)
		}
	}

	if got, err := validateManagedPath(root, projectDir); err != nil || got != projectDir {
		t.Fatalf("validateManagedPath() = %q, %v; want %q, nil", got, err, projectDir)
	}

	if _, err := validateManagedPath(root, root+"-evil"); err == nil {
		t.Fatal("prefix-confusion path was accepted")
	}

	symlinkPath := filepath.Join(root, "linked")
	if err := os.Symlink(outsideDir, symlinkPath); err != nil {
		t.Fatalf("Symlink(): %v", err)
	}
	if _, err := validateManagedPath(root, symlinkPath); err == nil {
		t.Fatal("symbolic-link escape was accepted")
	}

	nestedLink := filepath.Join(root, "nested-link")
	if err := os.Symlink(outsideDir, nestedLink); err != nil {
		t.Fatalf("Symlink(): %v", err)
	}
	if _, err := validateManagedPath(root, filepath.Join(nestedLink, "child")); err == nil {
		t.Fatal("nested symbolic-link escape was accepted")
	}
}
