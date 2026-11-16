//go:build !embed_frontend

package webui

import (
	"io/fs"
	"os"
)

func assets() (fs.FS, bool, error) {
	// Local development runs from server/; also support the repository root.
	for _, dir := range []string{"../public", "public"} {
		if _, err := os.Stat(dir + "/default-logo.png"); err == nil {
			return os.DirFS(dir), false, nil
		}
	}
	return os.DirFS("../public"), false, nil
}
