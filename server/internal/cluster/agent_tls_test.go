package cluster

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestAgentWebSocketsAfterHTTP2Enrollment(t *testing.T) {
	var enrollmentProtocol atomic.Int32
	upgrader := websocket.Upgrader{}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/agent/v1/enroll" {
			enrollmentProtocol.Store(int32(r.ProtoMajor))
			w.WriteHeader(http.StatusOK)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte("ready"))
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()

	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	invitation := filepath.Join(dir, "enrollment")
	if err := os.WriteFile(invitation, []byte(secret()), 0600); err != nil {
		t.Fatal(err)
	}
	agent, err := NewAgent(openTestStore(t), &testExecutor{}, AgentOptions{
		ControllerURL: server.URL, NodeID: randomID(), EnrollmentFile: invitation, CAFile: ca,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer agent.client.CloseIdleConnections()
	if enrollmentProtocol.Load() != 2 {
		t.Fatal("registration did not negotiate HTTP/2; regression condition was not exercised")
	}

	for _, path := range []string{"/agent/v1/connect", "/agent/v1/stream"} {
		t.Run(path, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			conn, response, err := agent.dialer.DialContext(ctx, agent.endpoint(path, true), agent.headers())
			if response != nil && response.Body != nil {
				defer response.Body.Close()
			}
			if err != nil {
				t.Fatalf("WebSocket after HTTP/2 registration: %v", err)
			}
			defer conn.Close()
			if response.ProtoMajor != 1 || response.StatusCode != http.StatusSwitchingProtocols {
				t.Fatalf("unexpected upgrade response: %s %d", response.Proto, response.StatusCode)
			}
			_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			_, data, err := conn.ReadMessage()
			if err != nil || string(data) != "ready" {
				t.Fatalf("WebSocket data: %q, %v", data, err)
			}
		})
	}
}
