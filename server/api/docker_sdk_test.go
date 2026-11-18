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

func TestDockerSDKPreservesResourceResponseShapes(t *testing.T) {
	cli, err := client.New(client.WithHost("http://docker.example"), client.WithAPIVersion("1.44"), client.WithHTTPClient(&http.Client{Transport: pullTransport(func(r *http.Request) (*http.Response, error) {
		path := strings.TrimPrefix(r.URL.Path, "/v1.44")
		body := "{}"
		switch path {
		case "/containers/json":
			body = `[{"Id":"demo","Names":["/app"],"State":"running","Image":"nginx:latest","Labels":{"com.docker.compose.project":"demo","com.docker.compose.project.working_dir":"/poke/projects/demo"},"Ports":[{"IP":"127.0.0.1","PrivatePort":80,"PublicPort":8080,"Type":"tcp"}]}]`
		case "/containers/demo/json":
			body = `{"Id":"demo","Name":"/app","Config":{"Image":"nginx:latest"},"State":{"Status":"running"}}`
		case "/images/json":
			body = `[{"Id":"sha256:fixture","RepoTags":["nginx:latest"],"Size":1024}]`
		case "/networks":
			body = `[{"Id":"net-fixture","Name":"bridge","Driver":"bridge","IPAM":{"Config":[{"Subnet":"172.18.0.0/16"}]}}]`
		case "/volumes":
			body = `{"Volumes":[{"Name":"data","Driver":"local"}],"Warnings":[]}`
		case "/images/prune", "/volumes/prune":
			body = `{"SpaceReclaimed":1024}`
		case "/info":
			body = `{"Name":"fixture-host","NCPU":2,"MemTotal":1024}`
		default:
			t.Errorf("unexpected Docker path %s", path)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	h := &Handler{Docker: cli, ImageService: images.NewService(cli)}
	router := echo.New()
	RegisterResources(router.Group("/api"), h)
	for _, path := range []string{"/api/containers", "/api/images", "/api/networks"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		var rows []map[string]any
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &rows) != nil || len(rows) != 1 {
			t.Fatalf("%s response wrapped or invalid: %d %s", path, response.Code, response.Body)
		}
		if path == "/api/containers" && (rows[0]["Id"] != "demo" || rows[0]["isControlled"] != false) {
			t.Fatal("container summary shape changed", rows)
		}
	}
	for _, tc := range []struct{ method, path, key string }{
		{"GET", "/api/containers/demo", "Config"}, {"GET", "/api/volumes", "Volumes"}, {"POST", "/api/images/prune", "SpaceReclaimed"}, {"POST", "/api/volumes/prune", "SpaceReclaimed"}, {"GET", "/api/system/info", "ports"},
	} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(tc.method, tc.path, nil)
		request = request.WithContext(images.WithCredentials(context.Background(), nil))
		router.ServeHTTP(response, request)
		var body map[string]any
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &body) != nil || body[tc.key] == nil {
			t.Fatalf("%s response wrapped or invalid: %d %s", tc.path, response.Code, response.Body)
		}
		if tc.key == "ports" {
			ports := body["ports"].([]any)
			if len(ports) != 1 || ports[0].(map[string]any)["ip"] != "127.0.0.1" {
				t.Fatal("netip port conversion changed API", ports)
			}
		}
	}
}
