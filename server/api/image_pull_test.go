package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/moby/moby/client"
	"poke/pkg/images"
)

type pullTransport func(*http.Request) (*http.Response, error)

func (f pullTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPullImageReportsDockerStreamErrors(t *testing.T) {
	for _, tc := range []struct {
		name, stream string
		status       int
	}{
		{"complete", `{"status":"Downloaded"}` + "\n", 200},
		{"registry denied", `{"error":"pull access denied"}` + "\n", 502},
		{"truncated", `{"status":`, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cli, err := client.New(client.WithHost("http://docker.example"), client.WithAPIVersion("1.43"), client.WithHTTPClient(&http.Client{Transport: pullTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(tc.stream))}, nil
			})}))
			if err != nil {
				t.Fatal(err)
			}
			defer cli.Close()
			h := &Handler{Docker: cli, ImageService: images.NewService(cli)}
			e := echo.New()
			e.POST("/api/images/pull", h.PullImage)
			req := httptest.NewRequest("POST", "/api/images/pull", strings.NewReader(`{"image":"registry.example/app"}`))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(images.WithCredentials(context.Background(), nil))
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d; %s", rec.Code, tc.status, rec.Body)
			}
		})
	}
}
