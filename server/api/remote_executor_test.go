package api

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"poke/internal/cluster"
)

func TestRemotePreparationFreezesSourceAndHidesNodeToken(t *testing.T) {
	bin := t.TempDir()
	script := `#!/bin/sh
[ -z "$POKE_NODE_TOKEN" ] || exit 90
case "$*" in
 *"config --format json"*) printf '%s' '{"services":{"web":{"image":"registry.example/app:1","command":"echo $HOME"},"other":{"image":"other.example/tool:2"}}}';;
 *) exit 1;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("POKE_NODE_TOKEN", "node-secret-must-not-reach-compose")
	executor := NewExecutor(&Handler{})
	executor.remoteRoot = t.TempDir()
	source := &cluster.ProjectSource{ID: strings.Repeat("a", 32), NodeID: strings.Repeat("b", 32), Name: "demo", Revision: 1, Content: "services:\n  web:\n    image: registry.example/app:1\n    command: 'echo $$HOME'\n  other:\n    image: other.example/tool:2\n"}
	req := &cluster.Request{Method: "POST", Path: "/api/projects", Project: source}
	prep, err := executor.Prepare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(prep.Domains) != 2 || !strings.Contains(prep.Request.Project.Content, "$$HOME") || strings.Contains(source.Content, `"services"`) {
		t.Fatal("source not frozen or source mutated", prep)
	}
	if _, err := os.Stat(filepath.Join(executor.remoteRoot, "demo", composeFilename)); !os.IsNotExist(err) {
		t.Fatal("preparation wrote Compose runtime file", err)
	}
	req.Path = "/api/projects/demo/action"
	req.Body = []byte(`{"action":"stop"}`)
	prep, err = executor.Prepare(context.Background(), req)
	if err != nil || len(prep.Domains) != 0 {
		t.Fatal("stop received registry credentials", err, prep.Domains)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(executor.remoteRoot, "unsafe")); err != nil {
		t.Fatal(err)
	}
	source.Name = "unsafe"
	if _, err := executor.Prepare(context.Background(), req); err == nil {
		t.Fatal("symlink working directory accepted")
	}
}
