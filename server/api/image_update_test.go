package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/moby/moby/client"
	"poke/pkg/images"
)

type imageUpdateTransport func(*http.Request) (*http.Response, error)

func (f imageUpdateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUpdateCheckExposesActionableAuthenticationError(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"message": "unknown: <html><title>404 Not Found</title></html>"})
	cli, err := client.New(client.WithHost("tcp://docker.test:2375"), client.WithAPIVersion("1.44"), client.WithHTTPClient(&http.Client{Transport: imageUpdateTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body))), Request: r}, nil
	})}))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	request := httptest.NewRequest("POST", "/api/images/check-update", strings.NewReader(`{"image":"registry.example/team/app:latest"}`))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(images.WithCredentials(context.Background(), nil))
	recorder := httptest.NewRecorder()
	h := &Handler{ImageService: images.NewService(cli)}
	if err = h.CheckImageUpdate(echo.New().NewContext(request, recorder)); err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err = json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != 200 || result["errorCode"] != "REGISTRY_AUTH_MISSING" || result["authConfigured"] != false || result["registry"] != "registry.example" || result["error"] == "" || result["details"] == nil {
		t.Fatal("API did not preserve authentication diagnosis")
	}
}
