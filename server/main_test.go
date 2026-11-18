package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moby/moby/client"
)

func TestDockerClientNegotiatesSupportedDaemonAPI(t *testing.T) {
	for _, tc := range []struct{ daemon, want string }{
		{"1.43", "1.43"}, {"1.44", "1.44"}, {"1.53", "1.53"}, {"1.99", client.MaxAPIVersion},
	} {
		t.Run(tc.daemon, func(t *testing.T) {
			var ping atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/_ping" {
					ping.Add(1)
					w.Header().Set("API-Version", tc.daemon)
					w.Header().Set("Min-API-Version", tc.want)
					_, _ = fmt.Fprint(w, "OK")
					return
				}
				if r.URL.Path != "/v"+tc.want+"/info" {
					http.Error(w, `{"message":"client API is too old or too new"}`, 400)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"ID":"mock-daemon","Name":"test-host","ServerVersion":"test"}`)
			}))
			defer server.Close()
			t.Setenv("DOCKER_HOST", server.URL)
			t.Setenv("DOCKER_API_VERSION", "")
			t.Setenv("DOCKER_TLS_VERIFY", "")
			t.Setenv("DOCKER_CERT_PATH", "")
			cli, err := newDockerClient()
			if err != nil {
				t.Fatal(err)
			}
			defer cli.Close()
			for i := 0; i < 2; i++ {
				info, err := cli.Info(context.Background(), client.InfoOptions{})
				if err != nil || info.Info.Name != "test-host" {
					t.Fatalf("API %s: %+v, %v", tc.daemon, info, err)
				}
			}
			if cli.ClientVersion() != tc.want || ping.Load() != 1 {
				t.Fatalf("negotiated %s; pings=%d", cli.ClientVersion(), ping.Load())
			}
		})
	}
}
func TestDockerClientHonorsExplicitAPIVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.44/info" {
			t.Errorf("explicit version unexpectedly negotiated: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"Name":"manual-version"}`)
	}))
	defer server.Close()
	t.Setenv("DOCKER_HOST", server.URL)
	t.Setenv("DOCKER_API_VERSION", "1.44")
	t.Setenv("DOCKER_TLS_VERIFY", "")
	t.Setenv("DOCKER_CERT_PATH", "")
	cli, err := newDockerClient()
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	if _, err := cli.Info(context.Background(), client.InfoOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_API_VERSION", "invalid")
	if _, err := newDockerClient(); err == nil || !strings.Contains(err.Error(), "invalid API version") {
		t.Fatal("invalid override accepted", err)
	}
}
