package images

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
	"poke/pkg/config"
)

func TestRegistryCheckFailureClassification(t *testing.T) {
	html404 := errors.New("unknown: <html><head><title>404 Not Found</title></head><body>nginx</body></html>")
	for _, tc := range []struct {
		name       string
		configured bool
		err        error
		code       string
	}{
		{"missing auth", false, errdefs.ErrUnauthenticated.WithMessage("unauthorized"), "REGISTRY_AUTH_MISSING"},
		{"bad auth", true, errdefs.ErrUnauthenticated.WithMessage("unauthorized"), "REGISTRY_AUTH_FAILED"},
		{"permission denied", true, errdefs.ErrPermissionDenied.WithMessage("access blocked"), "REGISTRY_AUTH_FAILED"},
		{"permission denied", true, errors.New("denied: access forbidden"), "REGISTRY_AUTH_FAILED"},
		{"token scope denied", true, errors.New("insufficient_scope: authorization failed"), "REGISTRY_AUTH_FAILED"},
		{"html without auth", false, html404, "REGISTRY_AUTH_MISSING"},
		{"html with auth", true, html404, "REGISTRY_ENDPOINT_ERROR"},
		{"not found without auth", false, errdefs.ErrNotFound.WithMessage("manifest unknown"), "REGISTRY_AUTH_MISSING"},
		{"manifest absent", true, errdefs.ErrNotFound.WithMessage("manifest unknown"), "REGISTRY_IMAGE_NOT_FOUND"},
		{"repository absent", true, errors.New("name unknown: repository name not known to registry"), "REGISTRY_IMAGE_NOT_FOUND"},
		{"unclassified 404", true, errdefs.ErrNotFound.WithMessage("404 page not found"), "REGISTRY_ENDPOINT_ERROR"},
		{"dns without auth", false, &net.DNSError{Err: "no such host"}, "REGISTRY_CONNECTION_FAILED"},
		{"tls", true, errors.New("x509: certificate signed by unknown authority"), "REGISTRY_CONNECTION_FAILED"},
		{"timeout", true, context.DeadlineExceeded, "REGISTRY_CONNECTION_FAILED"},
		{"other", false, errors.New("unexpected EOF"), "REGISTRY_CHECK_FAILED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := registryCheckFailure("registry.example", tc.configured, tc.err)
			if got.ErrorCode != tc.code || got.AuthConfigured != tc.configured || got.Registry != "registry.example" || got.Error == "" || got.Details != tc.err.Error() {
				t.Fatalf("incorrect classification: %s", got.ErrorCode)
			}
			if strings.Contains(got.Error, "<html>") {
				t.Fatal("raw HTML leaked into summary")
			}
		})
	}
	if got := registryCheckFailure("registry.example", false, html404); !strings.Contains(got.Error, "也可能") {
		t.Fatal("404 was incorrectly treated as proof of private registry")
	}
}

type updateTransport func(*http.Request) (*http.Response, error)

func (f updateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUpdateCheckCredentialScopeAndPublicAccess(t *testing.T) {
	var checks atomic.Int32
	digest := "sha256:" + strings.Repeat("a", 64)
	cli, err := client.New(client.WithHost("tcp://docker.test:2375"), client.WithAPIVersion("1.44"), client.WithHTTPClient(&http.Client{Transport: updateTransport(func(r *http.Request) (*http.Response, error) {
		status := 200
		var value interface{}
		if strings.Contains(r.URL.Path, "/distribution/") {
			checks.Add(1)
			if r.Header.Get(registry.AuthHeader) == "" && !strings.Contains(r.URL.Path, "/public/") {
				status = 401
				value = map[string]string{"message": "unauthorized"}
			} else {
				value = map[string]interface{}{"Descriptor": map[string]string{"digest": digest}}
			}
		} else {
			value = map[string]interface{}{"RepoDigests": []string{"registry.example/team/app@" + digest}}
		}
		data, _ := json.Marshal(value)
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(data)), Request: r}, nil
	})}))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	service := NewService(cli)
	anonymous := WithCredentials(context.Background(), nil)
	missing, err := service.CheckUpdate(anonymous, "registry.example/team/app:latest", false)
	if err != nil || missing.ErrorCode != "REGISTRY_AUTH_MISSING" {
		t.Fatal("missing auth was not classified")
	}
	authorized := WithCredentials(context.Background(), []config.RegistryConfig{{Domain: "registry.example", Username: "fixture", Password: "fixture-secret"}})
	allowed, err := service.CheckUpdate(authorized, "registry.example/team/app:latest", false)
	if err != nil || allowed.Error != "" || allowed.HasUpdate {
		t.Fatal("credential changes reused cached failure")
	}
	again, err := service.CheckUpdate(anonymous, "registry.example/team/app:latest", false)
	if err != nil || again.ErrorCode != "REGISTRY_AUTH_MISSING" || checks.Load() != 2 {
		t.Fatal("credential scopes shared cache results")
	}
	public, err := service.CheckUpdate(anonymous, "registry.example/public/app:latest", true)
	if err != nil || public.Error != "" {
		t.Fatal("public registry was incorrectly marked as missing authentication")
	}
}

func TestBoundedErrorDetails(t *testing.T) {
	details := boundedErrorDetails(errors.New(strings.Repeat("错", 5000)))
	if len([]rune(details)) != 4097 {
		t.Fatal("error details were not bounded")
	}
}
