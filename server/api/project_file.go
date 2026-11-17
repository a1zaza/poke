package api

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const composeFilename = "docker-compose.yml"

// Pin directory handles so replacing path components after validation cannot
// redirect a read or write outside the managed project tree.
func openProjectRoot(root, projectDir string) (*os.Root, error) {
	if _, err := validateManagedPath(root, projectDir); err != nil {
		return nil, err
	}
	managed, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer managed.Close()
	rel, err := filepath.Rel(root, projectDir)
	if err != nil {
		return nil, err
	}
	return managed.OpenRoot(rel)
}

func readProjectCompose(root, projectDir string) ([]byte, error) {
	project, err := openProjectRoot(root, projectDir)
	if err != nil {
		return nil, err
	}
	defer project.Close()
	// Nonblocking prevents a malicious FIFO from hanging the request. Check the
	// opened handle, rather than a pathname that could be replaced concurrently.
	directory, err := project.Open(".")
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	// Root.OpenFile may resolve in-root symlinks itself, so use openat on the
	// pinned directory with a fixed basename to reject even internal links.
	fd, err := unix.Openat(int(directory.Fd()), composeFilename, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), composeFilename)
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("compose file is not a regular file")
	}
	return io.ReadAll(f)
}

func writeProjectCompose(root, projectDir string, content []byte) error {
	project, err := openProjectRoot(root, projectDir)
	if err != nil {
		return err
	}
	defer project.Close()
	if info, err := project.Lstat(composeFilename); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("compose file is not a regular file")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	// Exclusive creation and atomic replacement avoid following either a
	// symlink or an existing hard link, including one swapped in after Lstat.
	temporary := ".compose-" + rand.Text()
	f, err := project.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer project.Remove(temporary)
	if _, err = f.Write(content); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return project.Rename(temporary, composeFilename)
}
