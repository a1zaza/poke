//go:build embed_frontend

package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

func assets() (fs.FS, bool, error) {
	files, err := fs.Sub(embedded, "dist")
	return files, true, err
}
