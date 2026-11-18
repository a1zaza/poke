package cluster

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAgentRestartWithoutEnrollmentToken(t *testing.T) {
	controller := NewController(openTestStore(t))
	defer controller.Close()
	var enrollments atomic.Int32
	router := testRouter(controller)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/agent/v1/enroll" {
			enrollments.Add(1)
		}
		router.ServeHTTP(w, r)
	}))
	defer server.Close()
	node, token, err := controller.Store.CreateNode("环境变量注册", nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { store.Close() }()
	opts := AgentOptions{ControllerURL: server.URL, NodeID: node.ID, AllowHTTP: true}
	if _, err := NewAgent(store, &testExecutor{}, opts); err == nil || !strings.Contains(err.Error(), "POKE_ENROLLMENT_TOKEN") {
		t.Fatalf("missing initial token: %v", err)
	}
	opts.EnrollmentToken = "invalid"
	if _, err := NewAgent(store, &testExecutor{}, opts); err == nil {
		t.Fatal("invalid token accepted")
	}
	opts.EnrollmentToken = " \n" + token + "\n"
	agent, err := NewAgent(store, &testExecutor{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer agent.client.CloseIdleConnections()
	credential := agent.identity.Credential
	if agent.Options.EnrollmentToken != "" {
		t.Fatal("temporary token retained in agent options")
	}
	data, err := os.ReadFile(filepath.Join(dir, "identity.json"))
	if err != nil || bytes.Contains(data, []byte(token)) {
		t.Fatal("temporary token leaked into identity file", err)
	}
	store.Close()
	store, err = OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	opts.EnrollmentToken = ""
	restarted, err := NewAgent(store, &testExecutor{}, opts)
	if err != nil {
		t.Fatalf("restart without temporary token: %v", err)
	}
	defer restarted.client.CloseIdleConnections()
	if restarted.identity.Credential != credential || !restarted.identity.Enrolled || enrollments.Load() != 1 {
		t.Fatal("restart did not reuse enrolled identity")
	}
	if _, err := controller.Store.Authenticate(node.ID, restarted.identity.Credential); err != nil {
		t.Fatalf("persisted identity rejected: %v", err)
	}
}
