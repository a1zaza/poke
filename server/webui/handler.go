package webui

import (
	"bytes"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

var pagePath = regexp.MustCompile(`^/(?:login|overview|containers(?:/[^/]+(?:/(?:logs|exec))?)?|images|networks|volumes|projects(?:/[^/]+)?|logs|settings|registries|nodes)?/?$`)
var hashedAsset = regexp.MustCompile(`^assets/(?:.*/)?[^/]+-[A-Za-z0-9_-]{8,}\.[^/]+$`)

// Register serves embedded UI in production and a dynamic logo in both modes.
func Register(e *echo.Echo, configDir string) error {
	files, production, err := assets()
	if err != nil {
		return err
	}
	return register(e, files, configDir, production)
}

func register(e *echo.Echo, files fs.FS, configDir string, production bool) error {
	if production {
		data, err := fs.ReadFile(files, "index.html")
		if err != nil || len(data) == 0 {
			return fmt.Errorf("前端入口资源不可用: %v", err)
		}
	}
	logo := func(c echo.Context) error {
		c.Response().Header().Set("Cache-Control", "no-store")
		data, err := os.ReadFile(filepath.Join(configDir, "logo.png"))
		if os.IsNotExist(err) {
			return serve(c, files, "default-logo.png")
		}
		if err != nil {
			return err
		}
		contentType := http.DetectContentType(data)
		if !strings.HasPrefix(contentType, "image/") {
			contentType = "application/octet-stream"
		}
		c.Response().Header().Set("Content-Type", contentType)
		c.Response().Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeContent(c.Response(), c.Request(), "logo.png", time.Time{}, bytes.NewReader(data))
		return nil
	}
	e.Match([]string{http.MethodGet, http.MethodHead}, "/logo.png", logo)
	if !production {
		return nil
	}
	handler := func(c echo.Context) error {
		urlPath := c.Request().URL.Path
		if reserved(urlPath) {
			return echo.ErrNotFound
		}
		name := strings.TrimPrefix(urlPath, "/")
		if strings.Contains(name, "\\") || strings.ContainsRune(name, 0) {
			return echo.ErrBadRequest
		}
		for _, part := range strings.Split(name, "/") {
			if part == "." || part == ".." {
				return echo.ErrBadRequest
			}
		}
		if name != "" && fs.ValidPath(name) {
			info, err := fs.Stat(files, name)
			if err == nil && !info.IsDir() {
				cache := "no-cache"
				if hashedAsset.MatchString(name) {
					cache = "public, max-age=31536000, immutable"
				}
				c.Response().Header().Set("Cache-Control", cache)
				return serve(c, files, name)
			}
			if err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		if !pagePath.MatchString(urlPath) {
			return echo.ErrNotFound
		}
		c.Response().Header().Set("Cache-Control", "no-cache")
		return serve(c, files, "index.html")
	}
	gzip := middleware.GzipWithConfig(middleware.GzipConfig{
		Skipper: func(c echo.Context) bool {
			return reserved(c.Request().URL.Path) || c.Request().Method == http.MethodHead
		},
	})
	e.Match([]string{http.MethodGet, http.MethodHead}, "/", handler, gzip)
	e.Match([]string{http.MethodGet, http.MethodHead}, "/*", handler, gzip)
	return nil
}

func reserved(p string) bool {
	return p == "/api" || strings.HasPrefix(p, "/api/") || p == "/ws" || strings.HasPrefix(p, "/ws/") || p == "/health"
}

func serve(c echo.Context, files fs.FS, name string) error {
	data, err := fs.ReadFile(files, name)
	if os.IsNotExist(err) {
		return echo.ErrNotFound
	}
	if err != nil {
		return err
	}
	http.ServeContent(c.Response(), c.Request(), path.Base(name), time.Time{}, bytes.NewReader(data))
	return nil
}
