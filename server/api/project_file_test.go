package api

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestProjectComposeFiles(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "app")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeProjectCompose(root, project, []byte("services: {}")); err != nil {
		t.Fatal(err)
	}
	data, err := readProjectCompose(root, project)
	if err != nil || string(data) != "services: {}" {
		t.Fatalf("read = %q, %v", data, err)
	}
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	compose := filepath.Join(project, composeFilename)
	if err := os.Remove(compose); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, compose); err != nil {
		t.Fatal(err)
	}
	if _, err := readProjectCompose(root, project); err == nil {
		t.Fatal("read followed symlink")
	}
	if err := writeProjectCompose(root, project, []byte("overwrite")); err == nil {
		t.Fatal("write accepted symlink")
	}
	if err := os.Remove(compose); err != nil {
		t.Fatal(err)
	}
	// Atomic replacement must also avoid modifying an existing hard-link target.
	if err := os.Link(outside, compose); err != nil {
		t.Fatal(err)
	}
	if err := writeProjectCompose(root, project, []byte("replacement")); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(outside)
	if err != nil || string(data) != "private" {
		t.Fatalf("outside changed: %q, %v", data, err)
	}
}

func TestProjectComposeConcurrentSymlinkReplacement(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "app")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var worker sync.WaitGroup
	worker.Add(1)
	go func() {
		defer worker.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = os.Remove(filepath.Join(project, composeFilename))
			_ = os.Symlink(outside, filepath.Join(project, composeFilename))
		}
	}()
	for i := 0; i < 100; i++ {
		_ = writeProjectCompose(root, project, []byte("replacement"))
		if data, err := readProjectCompose(root, project); err == nil && string(data) == "private" {
			close(stop)
			worker.Wait()
			t.Fatal("read escaped project")
		}
	}
	close(stop)
	worker.Wait()
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "private" {
		t.Fatalf("outside changed: %q, %v", data, err)
	}
}

func TestProjectComposeRejectsDirectorySymlink(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "app")
	if err := os.Symlink(t.TempDir(), project); err != nil {
		t.Fatal(err)
	}
	if _, err := readProjectCompose(root, project); err == nil {
		t.Fatal("read accepted linked project")
	}
	if err := writeProjectCompose(root, project, []byte("overwrite")); err == nil {
		t.Fatal("write accepted linked project")
	}
}

func TestProjectComposeRejectsInternalFileSymlink(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "app")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "other.yml"), []byte("services: {}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("other.yml", filepath.Join(project, composeFilename)); err != nil {
		t.Fatal(err)
	}
	if _, err := readProjectCompose(root, project); err == nil {
		t.Fatal("read followed internal file symlink")
	}
	if err := writeProjectCompose(root, project, []byte("overwrite")); err == nil {
		t.Fatal("write accepted internal file symlink")
	}
}
