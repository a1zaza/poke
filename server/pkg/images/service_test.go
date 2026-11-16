package images

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"poke/pkg/config"
	"testing"
)

func TestParseRepositoryInfo(t *testing.T) {
	tests := []struct {
		name       string
		ref        string
		wantDomain string
		wantPath   string
		wantTag    string
	}{
		{name: "official image", ref: "nginx", wantDomain: "docker.io", wantPath: "library/nginx", wantTag: "latest"},
		{name: "docker hub namespace", ref: "team/app:1.2", wantDomain: "docker.io", wantPath: "team/app", wantTag: "1.2"},
		{name: "explicit docker hub", ref: "docker.io/library/nginx:stable", wantDomain: "docker.io", wantPath: "library/nginx", wantTag: "stable"},
		{name: "custom registry", ref: "registry.example/team/app:2", wantDomain: "registry.example", wantPath: "team/app", wantTag: "2"},
		{name: "local registry", ref: "localhost:5000/app:dev", wantDomain: "localhost:5000", wantPath: "app", wantTag: "dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domain, path, tag := ParseRepositoryInfo(tt.ref)
			if domain != tt.wantDomain || path != tt.wantPath || tag != tt.wantTag {
				t.Fatalf("ParseRepositoryInfo(%q) = %q, %q, %q; want %q, %q, %q", tt.ref, domain, path, tag, tt.wantDomain, tt.wantPath, tt.wantTag)
			}
		})
	}
}

func TestScopedRegistryCredentials(t *testing.T) {
	service := NewService(nil)
	ctx := WithCredentials(context.Background(), []config.RegistryConfig{{Domain: "registry.example", Username: "worker", Password: "scoped-secret"}})
	if auth := service.GetRegistryAuth(ctx, "https://registry.example/"); auth == nil || auth.Password != "scoped-secret" {
		t.Fatalf("credential: %+v", auth)
	}
	empty := WithCredentials(context.Background(), nil)
	if auth := service.GetRegistryAuth(empty, "registry.example"); auth != nil {
		t.Fatal("credentials leaked between operations")
	}
	if auth := service.GetRegistryAuth(ctx, "other.example"); auth != nil {
		t.Fatal("unrelated registry received credentials")
	}
}
func TestComposeOnlyWritesNeededCredentialsAndCleansUp(t *testing.T) {
	bin := t.TempDir()
	script := []byte("#!/bin/sh\n[ \"$1 $2 $3\" = \"compose config --images\" ] || exit 1\nprintf 'registry.example/team/app:1\\ndocker.io/library/alpine:latest\\n'\n")
	if err := os.WriteFile(filepath.Join(bin, "docker"), script, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx := WithCredentials(context.Background(), []config.RegistryConfig{{Domain: "registry.example", Username: "u", Password: "secret"}, {Domain: "unused.example", Password: "unrelated-secret"}})
	dir, cleanup, err := NewService(nil).PrepareAuthConfigDir(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("registry.example")) || bytes.Contains(data, []byte("unused.example")) {
		t.Fatalf("scope: %s", data)
	}
	info, _ := os.Stat(filepath.Join(dir, "config.json"))
	if info.Mode().Perm() != 0600 {
		t.Fatal("credentials file is public")
	}
	cleanup()
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("credentials not cleaned")
	}
	if err = os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, cleanup, err = NewService(nil).PrepareAuthConfigDir(ctx, t.TempDir()); err == nil {
		cleanup()
		t.Fatal("Compose parse failure ignored")
	}
}
