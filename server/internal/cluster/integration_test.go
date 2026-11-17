package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"poke/pkg/config"
	"poke/pkg/images"
)

type testExecutor struct {
	calls     atomic.Int32
	cancelled chan struct{}
}

func (e *testExecutor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "GET" && r.URL.Path != "/api/images/check-update" {
		e.calls.Add(1)
	}
	if r.URL.Query().Get("wait") == "true" {
		<-r.Context().Done()
		if e.cancelled != nil {
			select {
			case e.cancelled <- struct{}{}:
			default:
			}
		}
		return
	}
	reg := images.NewService(nil).GetRegistryAuth(r.Context(), "registry.example")
	json.NewEncoder(w).Encode(map[string]any{"path": r.URL.Path, "query": r.URL.RawQuery, "credential_available": reg != nil, "cookie": r.Header.Get("Cookie")})
}
func (e *testExecutor) Exec(ctx context.Context, conn *websocket.Conn, id string) error {
	for {
		k, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if err = conn.WriteMessage(k, append([]byte(id+":"), data...)); err != nil {
			return err
		}
	}
}
func eventually(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not reached")
}
func browserRequest(t *testing.T, client *http.Client, method, url, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Test-Admin", "yes")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, data
}
func testRouter(c *Controller) *echo.Echo {
	e := echo.New()
	g := e.Group("/api", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			if ctx.Request().Header.Get("X-Test-Admin") != "yes" {
				return echo.ErrUnauthorized
			}
			return next(ctx)
		}
	})
	c.Register(e, g)
	e.GET("/ws/nodes/:node/containers/:id/exec", c.Terminal, func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			if ctx.Request().Header.Get("X-Test-Admin") != "yes" {
				return echo.ErrUnauthorized
			}
			return next(ctx)
		}
	})
	return e
}
func TestAgentOutboundLifecycle(t *testing.T) {
	controllerStore := openTestStore(t)
	agentStore := openTestStore(t)
	controller := NewController(controllerStore)
	defer controller.Close()
	controller.Registries = func() []config.RegistryConfig {
		return []config.RegistryConfig{{Domain: "registry.example", Username: "user", Password: "registry-secret"}, {Domain: "not-allowed.example", Password: "unrelated-secret"}}
	}
	var current atomic.Pointer[echo.Echo]
	current.Store(testRouter(controller))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { current.Load().ServeHTTP(w, r) }))
	defer server.Close()
	n, token, err := controllerStore.CreateNode("NAT 后节点", []string{"registry.example"})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "enrollment")
	if err = os.WriteFile(file, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	executor := &testExecutor{cancelled: make(chan struct{}, 1)}
	agent, err := NewAgent(agentStore, executor, AgentOptions{ControllerURL: server.URL, NodeID: n.ID, EnrollmentFile: file, AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	root, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { finished <- agent.Run(root) }()
	defer func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(3 * time.Second):
			t.Error("Agent did not stop")
		}
	}()
	eventually(t, func() bool {
		controller.mu.Lock()
		defer controller.mu.Unlock()
		return controller.sessions[n.ID] != nil && controller.sessions[n.ID].ready
	})
	status, data := browserRequest(t, server.Client(), "GET", server.URL+"/api/nodes/"+n.ID+"/api/containers?all=true", "")
	if status != 200 || !bytes.Contains(data, []byte(`"credential_available":false`)) || !bytes.Contains(data, []byte(`"query":"all=true"`)) {
		t.Fatalf("read: %d %s", status, data)
	}
	if !bytes.Contains(data, []byte(`"cookie":""`)) {
		t.Fatal("browser session sent to Agent")
	}
	// A read-only POST must retain scoped credentials without creating a task.
	beforeChecks := len(controllerStore.Tasks())
	status, data = browserRequest(t, server.Client(), "POST", server.URL+"/api/nodes/"+n.ID+"/api/images/check-update", `{"image":"registry.example/team/app"}`)
	if status != 200 || !bytes.Contains(data, []byte(`"credential_available":true`)) {
		t.Fatalf("update check: %d %s", status, data)
	}
	if len(controllerStore.Tasks()) != beforeChecks || len(agentStore.Tasks()) != 0 {
		t.Fatal("read-only update check created a mutation task")
	}
	status, data = browserRequest(t, server.Client(), "POST", server.URL+"/api/nodes/"+n.ID+"/api/images/pull", `{"image":"registry.example/team/app"}`)
	if status != 202 {
		t.Fatalf("dispatch: %d %s", status, data)
	}
	var submitted struct {
		ID string `json:"task_id"`
	}
	if err = json.Unmarshal(data, &submitted); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { task, _ := controllerStore.Task(submitted.ID); return task.State == "succeeded" })
	task, _ := controllerStore.Task(submitted.ID)
	if task.Response == nil || !bytes.Contains(task.Response.Body, []byte(`"credential_available":true`)) {
		t.Fatal("authorized pull did not receive scoped credentials")
	}
	// Re-delivery of the same task never executes the mutation again.
	controller.mu.Lock()
	session := controller.sessions[n.ID]
	controller.mu.Unlock()
	if err = session.Send(Frame{Type: "task", ID: submitted.ID, Request: &Request{Method: "POST", Path: "/api/containers/demo/start"}}); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { task, _ := agentStore.Task(submitted.ID); return task.Reported })
	if executor.calls.Load() != 1 {
		t.Fatal("task executed twice")
	}
	// No full-system settings, authentication or arbitrary URLs are accepted by the operation surface.
	for _, path := range []string{"/api/settings/system", "/api/auth/login", "/api/internal/registry-auth"} {
		status, _ = browserRequest(t, server.Client(), "GET", server.URL+"/api/nodes/"+n.ID+path, "")
		if status != 404 {
			t.Fatalf("forbidden path %s: %d", path, status)
		}
	}
	// Cancellation of an already-started mutation is reported as an uncertain result.
	status, data = browserRequest(t, server.Client(), "POST", server.URL+"/api/nodes/"+n.ID+"/api/containers/demo/start?wait=true", `{}`)
	if status != 202 {
		t.Fatalf("cancellable task: %d %s", status, data)
	}
	var cancelling struct {
		ID string `json:"task_id"`
	}
	if err = json.Unmarshal(data, &cancelling); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return executor.calls.Load() == 2 })
	status, _ = browserRequest(t, server.Client(), "POST", server.URL+"/api/tasks/"+cancelling.ID+"/cancel", "")
	if status != 202 {
		t.Fatalf("cancel: %d", status)
	}
	eventually(t, func() bool { task, _ := controllerStore.Task(cancelling.ID); return task.State == "unknown" })
	// A browser terminal is paired with another outbound Agent connection.
	headers := http.Header{"X-Test-Admin": []string{"yes"}}
	ws, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws/nodes/"+n.ID+"/containers/demo/exec", headers)
	if err != nil {
		t.Fatalf("terminal: %v %v", err, resp)
	}
	defer ws.Close()
	if err = ws.WriteMessage(websocket.TextMessage, []byte("input")); err != nil {
		t.Fatal(err)
	}
	ws.SetReadDeadline(time.Now().Add(time.Second))
	_, echoed, err := ws.ReadMessage()
	if err != nil || string(echoed) != "demo:input" {
		t.Fatalf("terminal %q %v", echoed, err)
	}
	// Controller process state is reconstructed; persistent node credentials remain valid.
	controller.Close()
	next := NewController(controllerStore)
	next.Registries = controller.Registries
	defer next.Close()
	current.Store(testRouter(next))
	eventually(t, func() bool {
		next.mu.Lock()
		defer next.mu.Unlock()
		return next.sessions[n.ID] != nil && next.sessions[n.ID].ready
	})
	status, _ = browserRequest(t, server.Client(), "GET", server.URL+"/api/nodes/"+n.ID+"/api/containers", "")
	if status != 200 {
		t.Fatalf("read after reconnect: %d", status)
	}
	status, _ = browserRequest(t, server.Client(), "DELETE", server.URL+"/api/nodes/"+n.ID, "")
	if status != 204 {
		t.Fatalf("revoke: %d", status)
	}
	status, _ = browserRequest(t, server.Client(), "GET", server.URL+"/api/nodes/"+n.ID+"/api/containers", "")
	if status != 503 {
		t.Fatalf("revoked node: %d", status)
	}
	for _, dir := range []string{controllerStore.dir, filepath.Join(controllerStore.dir, "tasks"), filepath.Join(agentStore.dir, "tasks")} {
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			raw, _ := os.ReadFile(filepath.Join(dir, entry.Name()))
			for _, s := range []string{token, agent.identity.Credential, "registry-secret", "unrelated-secret"} {
				if bytes.Contains(raw, []byte(s)) {
					t.Fatalf("secret leaked to %s", entry.Name())
				}
			}
		}
	}
}
func TestControllerAuthenticationAndOffline(t *testing.T) {
	c := NewController(openTestStore(t))
	defer c.Close()
	router := testRouter(c)
	for _, path := range []string{"/api/nodes", "/api/tasks", "/agent/v1/connect", "/agent/v1/stream"} {
		r := httptest.NewRecorder()
		router.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != 401 {
			t.Fatalf("%s: %d", path, r.Code)
		}
	}
	r := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/nodes/"+randomID()+"/api/containers", nil)
	req.Header.Set("X-Test-Admin", "yes")
	router.ServeHTTP(r, req)
	if r.Code != 503 {
		t.Fatal(r.Code)
	}
	if ValidRequest(&Request{Method: "POST", Path: "https://other.example/api/containers"}, false) || ValidRequest(&Request{Method: "GET", Path: "/api/projects/../settings"}, false) {
		t.Fatal("unsafe request accepted")
	}
}
func TestAgentIdentityAndTaskRecovery(t *testing.T) {
	s := openTestStore(t)
	id := randomID()
	credential := secret()
	original := identity{NodeID: id, Controller: "https://controller.example", Credential: credential, Attempt: randomID(), Enrolled: true}
	if err := WriteJSON(filepath.Join(s.dir, "identity.json"), original); err != nil {
		t.Fatal(err)
	}
	agent, err := NewAgent(s, &testExecutor{}, AgentOptions{ControllerURL: original.Controller, NodeID: id})
	if err != nil || agent.identity.Credential != credential {
		t.Fatalf("identity restart %v", err)
	}
	if _, err = NewAgent(s, &testExecutor{}, AgentOptions{ControllerURL: "http://controller.example", NodeID: id}); err == nil {
		t.Fatal("plaintext allowed by default")
	}
	if _, err = NewAgent(s, &testExecutor{}, AgentOptions{ControllerURL: "https://other.example", NodeID: id}); err == nil {
		t.Fatal("identity rebound to another Controller")
	}
	task := Task{ID: randomID(), State: "running", NodeID: id, CreatedAt: time.Now()}
	if err = s.SaveTask(task); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverTasks(); err != nil {
		t.Fatal(err)
	}
	task, _ = s.Task(task.ID)
	if task.State != "unknown" {
		t.Fatal("interrupted task replayed")
	}
}
func TestResultBound(t *testing.T) {
	w := &boundedWriter{header: make(http.Header)}
	_, _ = w.Write(bytes.Repeat([]byte("x"), MaxResult+1))
	r := w.result()
	if r.Status != 502 || len(r.Body) > MaxResult {
		t.Fatal(fmt.Sprint(r.Status))
	}
}

func TestTLSRegistrationAndOriginBoundary(t *testing.T) {
	controller := NewController(openTestStore(t))
	defer controller.Close()
	server := httptest.NewTLSServer(testRouter(controller))
	defer server.Close()
	node, token, err := controller.Store.CreateNode("TLS 节点", nil)
	if err != nil {
		t.Fatal(err)
	}
	invitation := filepath.Join(t.TempDir(), "token")
	if err = os.WriteFile(invitation, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	store := openTestStore(t)
	opts := AgentOptions{ControllerURL: server.URL, NodeID: node.ID, EnrollmentFile: invitation}
	if _, err = NewAgent(store, &testExecutor{}, opts); err == nil {
		t.Fatal("untrusted certificate accepted")
	}
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	opts.CAFile = ca
	agent, err := NewAgent(store, &testExecutor{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	headers := agent.headers()
	headers.Set("Origin", "https://untrusted.example")
	_, response, err := agent.dialer.Dial(agent.endpoint("/agent/v1/connect", true), headers)
	if err == nil || response == nil || response.StatusCode != 403 {
		t.Fatalf("Origin: %v %v", response, err)
	}
	if response != nil {
		response.Body.Close()
	}
}
