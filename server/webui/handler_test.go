package webui

import (
	"compress/gzip"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/labstack/echo/v4"
)

func fixture() fstest.MapFS {
	return fstest.MapFS{
		"index.html":              {Data: []byte("<!doctype html><html>页面</html>")},
		"assets/app-12345678.js":  {Data: []byte("console.log('静态资源')")},
		"assets/app-12345678.css": {Data: []byte("body {color:red}")},
		"default-logo.png":        {Data: []byte("default logo")},
		"manifest.json":           {Data: []byte(`{"name":"Poke"}`)},
	}
}

func request(e *echo.Echo, method, target string) *httptest.ResponseRecorder {
	r := httptest.NewRecorder()
	e.ServeHTTP(r, httptest.NewRequest(method, target, nil))
	return r
}

func TestUIRouting(t *testing.T) {
	e := echo.New()
	e.GET("/health", func(c echo.Context) error { return c.String(200, "OK") })
	api := e.Group("/api")
	api.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error { return echo.ErrUnauthorized }
	})
	api.GET("/private", func(c echo.Context) error { return c.NoContent(200) })
	e.POST("/api/auth/login", func(c echo.Context) error { return c.NoContent(204) })
	e.GET("/ws/terminal", func(c echo.Context) error { return c.NoContent(401) })
	if err := register(e, fixture(), t.TempDir(), true); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		method, url string
		status      int
		contains    string
	}{
		{"GET", "/", 200, "页面"},
		{"GET", "/projects/demo.v1", 200, "页面"},
		{"GET", "/containers/abc/exec", 200, "页面"},
		{"GET", "/login", 200, "页面"},
		{"GET", "/assets/app-12345678.js", 200, "console.log"},
		{"GET", "/assets/missing.js", 404, ""},
		{"GET", "/assets", 404, ""},
		{"GET", "/missing.png", 404, ""},
		{"GET", "/missing-page", 404, ""},
		{"POST", "/projects/demo", 405, ""},
		{"GET", "/api/private", 401, ""},
		{"GET", "/api/missing", 401, ""},
		{"POST", "/api/auth/login", 204, ""},
		{"GET", "/api", 401, ""},
		{"GET", "/ws/terminal", 401, ""},
		{"GET", "/ws/missing", 404, ""},
		{"GET", "/ws", 404, ""},
		{"GET", "/health", 200, "OK"},
		{"GET", "/%2e%2e/config.json", 400, ""},
		{"GET", "/assets/../index.html", 400, ""},
		{"GET", "/assets%5csecret", 400, ""},
		{"HEAD", "/projects/demo.v1", 200, ""},
	} {
		t.Run(tt.method+tt.url, func(t *testing.T) {
			r := request(e, tt.method, tt.url)
			if r.Code != tt.status {
				t.Fatalf("status %d: %s; want %d", r.Code, r.Body, tt.status)
			}
			if tt.contains != "" && !strings.Contains(r.Body.String(), tt.contains) {
				t.Fatalf("body: %s", r.Body)
			}
			if tt.status >= 400 && strings.Contains(r.Body.String(), "<!doctype") {
				t.Fatal("error became SPA HTML")
			}
			if tt.method == "HEAD" && r.Body.Len() != 0 {
				t.Fatal("HEAD has body")
			}
		})
	}
	for _, tt := range []struct{ url, mime, cache string }{
		{"/", "text/html", "no-cache"},
		{"/assets/app-12345678.js", "javascript", "immutable"},
		{"/assets/app-12345678.css", "text/css", "immutable"},
		{"/manifest.json", "application/json", "no-cache"},
	} {
		r := request(e, "GET", tt.url)
		if !strings.Contains(r.Header().Get("Content-Type"), tt.mime) || !strings.Contains(r.Header().Get("Cache-Control"), tt.cache) {
			t.Fatalf("%s headers: %v", tt.url, r.Header())
		}
	}
}

func TestCompressionStaysOutOfAPI(t *testing.T) {
	e := echo.New()
	e.GET("/api/logs", func(c echo.Context) error { return c.String(200, "stream") })
	if err := register(e, fixture(), t.TempDir(), true); err != nil {
		t.Fatal(err)
	}
	for _, url := range []string{"/", "/api/logs", "/ws/missing"} {
		req := httptest.NewRequest("GET", url, nil)
		req.Header.Set("Accept-Encoding", "gzip")
		r := httptest.NewRecorder()
		e.ServeHTTP(r, req)
		if url != "/" {
			if r.Header().Get("Content-Encoding") != "" {
				t.Fatalf("%s compressed", url)
			}
			continue
		}
		if r.Header().Get("Content-Encoding") != "gzip" || !strings.Contains(r.Header().Get("Vary"), "Accept-Encoding") {
			t.Fatalf("headers: %v", r.Header())
		}
		reader, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		data, err := io.ReadAll(reader)
		if err != nil || !strings.Contains(string(data), "页面") {
			t.Fatalf("gzip body %q: %v", data, err)
		}
	}
}

func TestLogoLifecycle(t *testing.T) {
	for _, production := range []bool{false, true} {
		e := echo.New()
		dir := t.TempDir()
		if err := register(e, fixture(), dir, production); err != nil {
			t.Fatal(err)
		}
		r := request(e, "GET", "/logo.png")
		if r.Body.String() != "default logo" || r.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("default: %s", r.Body)
		}
		for _, data := range []string{"custom one", "custom two"} {
			if err := os.WriteFile(filepath.Join(dir, "logo.png"), []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			r = request(e, "GET", "/logo.png?v=1")
			if r.Body.String() != data {
				t.Fatalf("custom: %s", r.Body)
			}
		}
		if err := os.Remove(filepath.Join(dir, "logo.png")); err != nil {
			t.Fatal(err)
		}
		if r = request(e, "GET", "/logo.png"); r.Body.String() != "default logo" {
			t.Fatal("reset failed")
		}
		if err := os.Mkdir(filepath.Join(dir, "logo.png"), 0700); err != nil {
			t.Fatal(err)
		}
		if r = request(e, "GET", "/logo.png"); r.Code != 500 {
			t.Fatalf("read failure: %d", r.Code)
		}
		if !production && request(e, "GET", "/login").Code != 404 {
			t.Fatal("dev should use Vite")
		}
	}
}

func TestProductionRequiresIndex(t *testing.T) {
	if err := register(echo.New(), fstest.MapFS{}, t.TempDir(), true); err == nil {
		t.Fatal("missing production index accepted")
	}
}
