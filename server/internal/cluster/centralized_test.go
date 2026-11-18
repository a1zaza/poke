package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"poke/pkg/config"
	"poke/pkg/images"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func runTestAgent(t *testing.T, c *Controller, url string, node Node, token string, executor Executor) *Agent {
	t.Helper()
	a, err := NewAgent(executor, AgentOptions{ControllerURL: url, NodeID: node.ID, NodeToken: token, AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(4 * time.Second):
			t.Error("Agent shutdown stalled")
		}
	})
	eventually(t, func() bool { c.mu.Lock(); defer c.mu.Unlock(); s := c.sessions[node.ID]; return s != nil && s.ready })
	return a
}
func TestDuplicateConnectionDoesNotKickCurrent(t *testing.T) {
	c := NewController(openTestStore(t))
	defer c.Close()
	server := httptest.NewServer(testRouter(c))
	defer server.Close()
	node, token, _ := c.Store.CreateNode("same", nil)
	first := runTestAgent(t, c, server.URL, node, token, &testExecutor{})
	second, _ := NewAgent(&testExecutor{}, AgentOptions{ControllerURL: server.URL, NodeID: node.ID, NodeToken: token, AllowHTTP: true})
	_, response, err := second.dialer.Dial(second.endpoint("/agent/v1/connect", true), second.headers())
	if err == nil || response == nil || response.StatusCode != 409 {
		t.Fatal("duplicate connected", err, response)
	}
	response.Body.Close()
	c.mu.Lock()
	s := c.sessions[node.ID]
	if s.node.BootID != first.bootID || s.conflicts != 1 {
		t.Fatal("original instance replaced")
	}
	c.mu.Unlock()
	// A reconnect from the same boot is admitted and old connection cleanup cannot erase it.
	conn, response, err := first.dialer.Dial(first.endpoint("/agent/v1/connect", true), first.headers())
	if err != nil {
		t.Fatal(err, response)
	}
	defer conn.Close()
	p := NewPeer(conn)
	defer p.Close()
	hello, err := p.Read()
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Send(Frame{Type: "hello", ID: hello.ID, BootID: first.bootID, Info: &AgentInfo{Hostname: "same"}}); err != nil {
		t.Fatal(err)
	}
	welcome, err := p.Read()
	if err != nil || welcome.Type != "welcome" {
		t.Fatal("reconnect failed", err)
	}
	c.mu.Lock()
	current := c.sessions[node.ID]
	if current == nil || current.id != welcome.Session {
		t.Fatal("old connection cleanup erased new session")
	}
	c.mu.Unlock()
}
func TestControllerProjectSourceOfflineAndVersions(t *testing.T) {
	c := NewController(openTestStore(t))
	defer c.Close()
	server := httptest.NewServer(testRouter(c))
	defer server.Close()
	node, _, _ := c.Store.CreateNode("offline", nil)
	content := "services:\n  web:\n    image: registry.example/web:1\n"
	status, data := browserRequest(t, server.Client(), "POST", server.URL+"/api/nodes/"+node.ID+"/projects/import", string(mustJSON(map[string]string{"name": "web", "content": content})))
	if status != 201 {
		t.Fatalf("import %d %s", status, data)
	}
	status, data = browserRequest(t, server.Client(), "GET", server.URL+"/api/nodes/"+node.ID+"/api/projects/web", "")
	if status != 200 || !strings.Contains(string(data), `"actualStateAvailable":false`) || !strings.Contains(string(data), `"revision":1`) {
		t.Fatalf("offline source: %d %s", status, data)
	}
	p, err := c.Store.SaveProject(node.ID, "web", strings.ReplaceAll(content, ":1", ":2"), false)
	if err != nil || p.Revision != 2 || p.DeployedRevision != 0 {
		t.Fatal("draft version", p, err)
	}
	old := p
	old.Revision = 1
	task := Task{State: "succeeded", Request: &Request{Method: "PUT", Path: "/api/projects/web", Project: &old}}
	if err := c.finishProject(task); err != nil {
		t.Fatal(err)
	}
	p, _ = c.Store.Project(node.ID, "web")
	if p.Revision != 2 || p.DeployedRevision != 1 {
		t.Fatal("deployment overwrote newer draft", p)
	}
	before := len(c.Store.Tasks())
	status, data = browserRequest(t, server.Client(), "PUT", server.URL+"/api/nodes/"+node.ID+"/projects/web/source", string(mustJSON(map[string]string{"content": strings.ReplaceAll(content, ":1", ":3")})))
	if status != 200 || len(c.Store.Tasks()) != before {
		t.Fatalf("offline source edit created Docker task: %d %s", status, data)
	}
	current, _ := c.Store.Project(node.ID, "web")
	if current.Revision != 3 || current.DeployedRevision != 1 {
		t.Fatal("source edit overwrote deployed state", current)
	}
	if err := c.finishProject(Task{State: "succeeded", Request: &Request{Method: "POST", Path: "/api/projects/web/action", Body: []byte(`{"action":"stop"}`), Project: &current}}); err != nil {
		t.Fatal(err)
	}
	current, _ = c.Store.Project(node.ID, "web")
	if current.DeployedRevision != 1 {
		t.Fatal("stop marked entire source deployed")
	}
	status, data = browserRequest(t, server.Client(), "GET", server.URL+"/api/nodes/"+node.ID+"/api/projects", "")
	var list []map[string]any
	if status != 200 || json.Unmarshal(data, &list) != nil || len(list) != 1 {
		t.Fatal("offline listing", status, string(data))
	}
	if _, err := c.Store.SaveProject(node.ID, "../escape", content, true); err == nil {
		t.Fatal("project escaped root")
	}
}

type credentialExecutor struct {
	testExecutor
	prepared chan struct{}
	proceed  chan struct{}
	password atomic.Value
	once     sync.Once
}

func (e *credentialExecutor) Prepare(ctx context.Context, r *Request) (Preparation, error) {
	e.once.Do(func() { close(e.prepared) })
	select {
	case <-e.proceed:
	case <-ctx.Done():
		return Preparation{}, ctx.Err()
	}
	return Preparation{Request: r, Domains: requiredDomains(r)}, nil
}
func (e *credentialExecutor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	auth := images.NewService(nil).GetRegistryAuth(r.Context(), "registry.example")
	if auth != nil {
		e.password.Store(auth.Password)
	}
	if images.NewService(nil).GetRegistryAuth(r.Context(), "unused.example") != nil {
		e.password.Store("unrelated-secret-leaked")
	}
	e.testExecutor.ServeHTTP(w, r)
}
func TestCredentialsAtExecutionAndRepeatedPermit(t *testing.T) {
	c := NewController(openTestStore(t))
	defer c.Close()
	server := httptest.NewServer(testRouter(c))
	defer server.Close()
	var password atomic.Value
	password.Store("old-password")
	c.Registries = func() []config.RegistryConfig {
		return []config.RegistryConfig{{Domain: "registry.example", Username: "fixture", Password: password.Load().(string)}, {Domain: "unused.example", Password: "unrelated-secret"}}
	}
	node, token, _ := c.Store.CreateNode("auth", []string{"registry.example", "unused.example"})
	executor := &credentialExecutor{prepared: make(chan struct{}), proceed: make(chan struct{})}
	agent := runTestAgent(t, c, server.URL, node, token, executor)
	status, data := browserRequest(t, server.Client(), "POST", server.URL+"/api/nodes/"+node.ID+"/api/images/pull", `{"image":"registry.example/team/app:1"}`)
	if status != 202 {
		t.Fatalf("dispatch %d %s", status, data)
	}
	var submitted struct {
		ID string `json:"task_id"`
	}
	_ = json.Unmarshal(data, &submitted)
	select {
	case <-executor.prepared:
	case <-time.After(time.Second):
		t.Fatal("no preparation")
	}
	if executor.calls.Load() != 0 {
		t.Fatal("mutation executed before permit")
	}
	password.Store("new-password")
	close(executor.proceed)
	eventually(t, func() bool { task, _ := c.Store.Task(submitted.ID); return task.State == "succeeded" })
	if executor.password.Load() != "new-password" {
		t.Fatal("stale or unrelated credentials supplied", executor.password.Load())
	}
	task, _ := c.Store.Task(submitted.ID)
	c.mu.Lock()
	session := c.sessions[node.ID]
	c.mu.Unlock()
	for i := 0; i < 3; i++ {
		if err := session.send(Frame{Type: "execute", ID: task.ID, Permit: task.Permit, Hash: task.PreparedHash, Request: &Request{}}); err != nil {
			t.Fatal(err)
		}
	}
	eventually(t, func() bool { agent.mu.Lock(); defer agent.mu.Unlock(); return agent.tasks[task.ID].task.Resolved })
	if executor.calls.Load() != 1 {
		t.Fatal("repeated permit executed twice")
	}
	raw, err := os.ReadFile(filepath.Join(c.Store.dir, "tasks", task.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"old-password", "new-password", "unrelated-secret"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatal("registry credentials persisted in task")
		}
	}
}
func TestAgentCannotExecuteWithoutPermission(t *testing.T) {
	executor := &testExecutor{}
	agent, err := NewAgent(executor, AgentOptions{ControllerURL: "https://controller.example", NodeID: randomID(), NodeToken: secret()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	frame := Frame{ID: randomID(), Request: &Request{Method: "POST", Path: "/api/containers/demo/start"}}
	if err := agent.startTask(ctx, frame); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { agent.mu.Lock(); defer agent.mu.Unlock(); return agent.tasks[frame.ID].prepared != nil })
	if err := agent.startTask(ctx, frame); err != nil {
		t.Fatal(err)
	}
	if executor.calls.Load() != 0 {
		t.Fatal("executed without Controller permission")
	}
	cancel()
	agent.wg.Wait()
	if executor.calls.Load() != 0 || agent.tasks[frame.ID].task.State != "cancelled" {
		t.Fatal("cancelled ungranted operation executed")
	}
}

type containerExecutor struct{ testExecutor }

func (e *containerExecutor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/containers" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{{"Id": "one", "Labels": map[string]string{"com.docker.compose.project": "managed"}, "isControlled": false}, {"Id": "two", "Labels": map[string]string{"com.docker.compose.project": "external"}, "isControlled": true}})
		return
	}
	e.testExecutor.ServeHTTP(w, r)
}
func TestContainerOwnershipUsesControllerSources(t *testing.T) {
	c := NewController(openTestStore(t))
	defer c.Close()
	server := httptest.NewServer(testRouter(c))
	defer server.Close()
	n, token, _ := c.Store.CreateNode("node", nil)
	if _, err := c.Store.SaveProject(n.ID, "managed", "services:\n  web:\n    image: nginx\n", true); err != nil {
		t.Fatal(err)
	}
	runTestAgent(t, c, server.URL, n, token, &containerExecutor{})
	status, data := browserRequest(t, server.Client(), "GET", server.URL+"/api/nodes/"+n.ID+"/api/containers", "")
	var rows []map[string]any
	if status != 200 || json.Unmarshal(data, &rows) != nil || len(rows) != 2 || rows[0]["isControlled"] != true || rows[1]["isControlled"] != false {
		t.Fatalf("ownership %d %s", status, data)
	}
}

func TestExpiredLeaseSettlesTasksWithoutReconnect(t *testing.T) {
	c := NewController(openTestStore(t))
	defer c.Close()
	n, token, _ := c.Store.CreateNode("lost", nil)
	boot := randomID()
	lease, err := c.Store.Claim(n.ID, token, boot)
	if err != nil {
		t.Fatal(err)
	}
	queued := Task{ID: randomID(), NodeID: n.ID, BootID: boot, State: "queued"}
	granted := queued
	granted.ID = randomID()
	granted.Permit = randomID()
	granted.State = "execution_granted"
	for _, task := range []Task{queued, granted} {
		if err := c.Store.SaveTask(task); err != nil {
			t.Fatal(err)
		}
	}
	c.Store.mu.Lock()
	m := c.Store.copyNodes()
	stored := m[n.ID]
	stored.LeaseUntil = time.Now().Add(-time.Second)
	m[n.ID] = stored
	if err := c.Store.saveNodes(m); err != nil {
		t.Fatal(err)
	}
	c.Store.mu.Unlock()
	c.expireLeases()
	q, _ := c.Store.Task(queued.ID)
	g, _ := c.Store.Task(granted.ID)
	if q.State != "cancelled" || g.State != "unknown" || !c.Store.NodeBlocked(n.ID) {
		t.Fatal("expired lease left tasks hanging", q, g, lease)
	}
}
