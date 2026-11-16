package cluster

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"poke/pkg/images"
)

type AgentOptions struct {
	ControllerURL, NodeID, EnrollmentFile, CAFile string
	AllowHTTP                                     bool
}
type identity struct {
	NodeID         string `json:"node_id"`
	Controller     string `json:"controller"`
	Credential     string `json:"credential"`
	Attempt        string `json:"attempt"`
	InvitationHash string `json:"invitation_hash"`
	Enrolled       bool   `json:"enrolled"`
}
type Agent struct {
	Store    *Store
	Executor Executor
	Options  AgentOptions
	identity identity
	client   *http.Client
	dialer   *websocket.Dialer
	base     *url.URL
	mu       sync.Mutex
	peer     *Peer
	running  map[string]context.CancelFunc
	workers  chan struct{}
	writeMu  sync.Mutex
	wg       sync.WaitGroup
}

func NewAgent(store *Store, executor Executor, opts AgentOptions) (*Agent, error) {
	u, err := url.Parse(opts.ControllerURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(opts.AllowHTTP && u.Scheme == "http")) {
		return nil, errors.New("Controller 地址必须是 HTTPS；仅测试环境可显式允许 HTTP")
	}
	if !safeID.MatchString(opts.NodeID) {
		return nil, errors.New("节点标识无效")
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if opts.CAFile != "" {
		data, err := os.ReadFile(opts.CAFile)
		if err != nil {
			return nil, err
		}
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(data) {
			return nil, errors.New("CA 文件没有有效证书")
		}
		tlsConfig.RootCAs = pool
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	a := &Agent{Store: store, Executor: executor, Options: opts, base: u, running: map[string]context.CancelFunc{}, workers: make(chan struct{}, 8), client: &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, dialer: &websocket.Dialer{Proxy: http.ProxyFromEnvironment, TLSClientConfig: tlsConfig, HandshakeTimeout: 10 * time.Second}}
	name := filepath.Join(store.dir, "identity.json")
	if err = readJSON(name, &a.identity); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if a.identity.NodeID != "" && (a.identity.NodeID != opts.NodeID || a.identity.Controller != opts.ControllerURL) {
		return nil, errors.New("数据目录身份与配置的节点或 Controller 不一致")
	}
	var invitation string
	if opts.EnrollmentFile != "" {
		data, err := os.ReadFile(opts.EnrollmentFile)
		if err != nil {
			return nil, err
		}
		invitation = strings.TrimSpace(string(data))
		if len(invitation) != 64 {
			return nil, errors.New("注册凭据格式无效")
		}
	}
	if a.identity.NodeID == "" || (invitation != "" && digest(invitation) != a.identity.InvitationHash) {
		if invitation == "" {
			return nil, errors.New("首次启动需要 POKE_ENROLLMENT_TOKEN_FILE")
		}
		a.identity = identity{NodeID: opts.NodeID, Controller: opts.ControllerURL, Credential: secret(), Attempt: randomID(), InvitationHash: digest(invitation)}
		if err = WriteJSON(name, a.identity); err != nil {
			return nil, err
		}
	}
	if !a.identity.Enrolled {
		if invitation == "" {
			return nil, errors.New("未完成注册，需要保留注册凭据文件以重试")
		}
		payload, _ := json.Marshal(map[string]string{"node_id": opts.NodeID, "token": invitation, "attempt": a.identity.Attempt, "credential": a.identity.Credential})
		req, err := http.NewRequest("POST", a.endpoint("/agent/v1/enroll", false), bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := a.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("注册连接失败: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("注册被拒绝（HTTP %d），请确认凭据是否过期或已被使用", resp.StatusCode)
		}
		a.identity.Enrolled = true
		if err = WriteJSON(name, a.identity); err != nil {
			return nil, err
		}
	}
	if len(a.identity.Credential) != 64 || a.identity.Attempt == "" {
		return nil, errors.New("节点身份文件无效")
	}
	return a, nil
}
func (a *Agent) endpoint(path string, ws bool) string {
	u := *a.base
	u.Path = strings.TrimSuffix(u.Path, "/") + path
	u.RawPath = ""
	if ws {
		if u.Scheme == "https" {
			u.Scheme = "wss"
		} else {
			u.Scheme = "ws"
		}
	}
	return u.String()
}
func (a *Agent) headers() http.Header {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+a.identity.Credential)
	h.Set("X-Poke-Node", a.identity.NodeID)
	return h
}
func (a *Agent) send(f Frame) error {
	a.mu.Lock()
	p := a.peer
	a.mu.Unlock()
	if p == nil {
		return errors.New("控制连接离线")
	}
	return p.Send(f)
}
func (a *Agent) Run(ctx context.Context) error {
	defer a.client.CloseIdleConnections()
	defer func() {
		a.mu.Lock()
		if a.peer != nil {
			a.peer.Close()
		}
		for _, cancel := range a.running {
			cancel()
		}
		a.mu.Unlock()
		a.wg.Wait()
	}()
	retry := time.Second
	for ctx.Err() == nil {
		conn, resp, err := a.dialer.DialContext(ctx, a.endpoint("/agent/v1/connect", true), a.headers())
		if err == nil {
			p := NewPeer(conn)
			a.mu.Lock()
			a.peer = p
			a.mu.Unlock()
			connectedAt := time.Now()
			err = a.serve(ctx, p)
			p.Close()
			a.mu.Lock()
			if a.peer == p {
				a.peer = nil
			}
			a.mu.Unlock()
			if time.Since(connectedAt) > time.Minute {
				retry = time.Second
			}
		}
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		if ctx.Err() != nil {
			break
		}
		if errors.Is(err, ErrProtocolVersion) {
			return err
		}
		if resp != nil && (resp.StatusCode == 401 || resp.StatusCode == 403) {
			return errors.New("节点身份已撤销或过期，请在 Controller 重新生成注册凭据")
		}
		log.Printf("AGENT: 控制连接断开，将重试: %v", err)
		delay := retry/2 + time.Duration(rand.Int63n(int64(retry/2)+1))
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
		if retry < 30*time.Second {
			retry *= 2
			if retry > 30*time.Second {
				retry = 30 * time.Second
			}
		}
	}
	return ctx.Err()
}
func (a *Agent) serve(ctx context.Context, p *Peer) error {
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			p.Close()
		case <-done:
		}
	}()
	f, err := p.Read()
	if err != nil {
		return err
	}
	if f.Type != "hello" {
		return errors.New("Controller 握手失败")
	}
	hostname, _ := os.Hostname()
	if err = p.Send(Frame{Type: "hello", ID: f.ID, Info: &AgentInfo{Hostname: hostname, Platform: runtime.GOOS + "/" + runtime.GOARCH}}); err != nil {
		return err
	}
	for _, task := range a.Store.allTasks() {
		if task.Terminal() && !task.Reported {
			t, err := a.Store.Task(task.ID)
			if err == nil {
				if err = p.Send(Frame{Type: "result", ID: t.ID, Task: &t}); err != nil {
					return err
				}
			}
		}
	}
	for {
		f, err := p.Read()
		if err != nil {
			return err
		}
		switch f.Type {
		case "open":
			if !safeID.MatchString(f.Stream) || len(f.Ticket) != 64 || !ValidRequest(f.Request, f.Kind == "exec") || (f.Kind != "http" && f.Kind != "exec") {
				return errors.New("非法数据流请求")
			}
			select {
			case a.workers <- struct{}{}:
			default:
				if err = p.Send(Frame{Type: "stream_error", ID: f.Stream}); err != nil {
					return err
				}
				continue
			}
			a.wg.Add(1)
			go func(f Frame) { defer a.wg.Done(); defer func() { <-a.workers }(); a.stream(ctx, p, f) }(f)
		case "task":
			if !safeID.MatchString(f.ID) || !ValidRequest(f.Request, false) || f.Request.Method == "GET" {
				return errors.New("非法任务请求")
			}
			if err = a.startTask(ctx, f); err != nil {
				return err
			}
		case "cancel":
			a.mu.Lock()
			cancel := a.running[f.ID]
			a.mu.Unlock()
			if cancel != nil {
				cancel()
			}
		case "ack":
			task, err := a.Store.Task(f.ID)
			if err == nil && task.Terminal() {
				task.Reported = true
				if err = a.Store.SaveTask(task); err != nil {
					return err
				}
			}
		default:
			return errors.New("未知控制消息")
		}
	}
}
func (a *Agent) stream(root context.Context, p *Peer, f Frame) {
	h := a.headers()
	h.Set("X-Poke-Stream", f.Stream)
	h.Set("X-Poke-Ticket", f.Ticket)
	ctx, cancel := context.WithCancel(root)
	defer cancel()
	conn, resp, err := a.dialer.DialContext(ctx, a.endpoint("/agent/v1/stream", true), h)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(64 * 1024)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-p.Done():
			conn.Close()
		case <-done:
		}
	}()
	if f.Kind == "exec" {
		heartbeat := NewPeer(conn)
		defer heartbeat.Close()
		u, _ := url.ParseRequestURI(f.Request.Path)
		parts := strings.Split(u.Path, "/")
		_ = a.Executor.Exec(ctx, conn, parts[3])
		return
	}
	// A reader is necessary to observe Controller/browser cancellation even while Docker is working.
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	}()
	queryCtx, cancelQuery := context.WithTimeout(images.WithCredentials(ctx, f.Request.Registries), 30*time.Second)
	defer cancelQuery()
	req, err := http.NewRequestWithContext(queryCtx, f.Request.Method, f.Request.Path, bytes.NewReader(f.Request.Body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	writer := &streamWriter{conn: conn, header: make(http.Header)}
	a.Executor.ServeHTTP(writer, req)
	writer.finish()
}
func (a *Agent) startTask(root context.Context, f Frame) error {
	a.mu.Lock()
	if _, exists := a.running[f.ID]; exists {
		a.mu.Unlock()
		return a.send(Frame{Type: "accepted", ID: f.ID})
	}
	a.mu.Unlock()
	if old, err := a.Store.Task(f.ID); err == nil {
		if old.Terminal() {
			return a.send(Frame{Type: "result", ID: old.ID, Task: &old})
		}
		return a.send(Frame{Type: "accepted", ID: old.ID})
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	a.mu.Lock()
	if len(a.running) >= 8 {
		a.mu.Unlock()
		failed := Task{ID: f.ID, NodeID: a.identity.NodeID, Method: f.Request.Method, Path: f.Request.Path, State: "failed", CreatedAt: time.Now().UTC(), Response: &Response{Status: 429, ContentType: "application/json", Body: []byte(`{"error":"节点任务队列已满，请稍后重试"}`)}}
		if err := a.Store.SaveTask(failed); err != nil {
			return err
		}
		return a.send(Frame{Type: "result", ID: f.ID, Task: &failed})
	}
	ctx, cancel := context.WithTimeout(root, 10*time.Minute)
	a.running[f.ID] = cancel
	a.mu.Unlock()
	task := Task{ID: f.ID, NodeID: a.identity.NodeID, Method: f.Request.Method, Path: f.Request.Path, State: "accepted", CreatedAt: time.Now().UTC()}
	if err := a.Store.SaveTask(task); err != nil {
		cancel()
		a.mu.Lock()
		delete(a.running, f.ID)
		a.mu.Unlock()
		return err
	}
	_ = a.send(Frame{Type: "accepted", ID: f.ID})
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		defer cancel()
		defer func() { a.mu.Lock(); delete(a.running, f.ID); a.mu.Unlock() }()
		// Mutations are serialized on each Agent, including Compose changes to the same project.
		a.writeMu.Lock()
		defer a.writeMu.Unlock()
		if ctx.Err() != nil {
			task.State = "cancelled"
			task.Error = "任务在执行前已取消"
		} else {
			task.State = "running"
			if a.Store.SaveTask(task) != nil {
				return
			}
			req, _ := http.NewRequestWithContext(images.WithCredentials(ctx, f.Request.Registries), f.Request.Method, f.Request.Path, bytes.NewReader(f.Request.Body))
			req.Header.Set("Content-Type", "application/json")
			writer := &boundedWriter{header: make(http.Header)}
			a.Executor.ServeHTTP(writer, req)
			response := writer.result()
			task.Response = &response
			task.State = "succeeded"
			if response.Status >= 400 {
				task.State = "failed"
			}
			if ctx.Err() != nil || writer.overflow {
				task.State = "unknown"
				task.Error = "执行期间已取消或超时，操作结果需要确认"
			}
		}
		if err := a.Store.SaveTask(task); err != nil {
			log.Printf("AGENT: 任务结果保存失败: %v", err)
			return
		}
		_ = a.send(Frame{Type: "result", ID: task.ID, Task: &task})
	}()
	return nil
}

type boundedWriter struct {
	header   http.Header
	status   int
	body     bytes.Buffer
	overflow bool
}

func (w *boundedWriter) Header() http.Header { return w.header }
func (w *boundedWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *boundedWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	if w.body.Len()+len(data) > MaxResult {
		w.overflow = true
		return 0, errors.New("任务响应超过限制")
	}
	return w.body.Write(data)
}
func (w *boundedWriter) Flush() {}
func (w *boundedWriter) result() Response {
	if w.overflow {
		return Response{Status: 502, ContentType: "application/json", Body: []byte(`{"error":"任务响应超过限制，请检查节点实际状态"}`)}
	}
	if w.status == 0 {
		w.status = 200
	}
	return Response{Status: w.status, ContentType: w.header.Get("Content-Type"), Body: append([]byte(nil), w.body.Bytes()...)}
}

type streamWriter struct {
	conn    *websocket.Conn
	header  http.Header
	started bool
	err     error
}

func (w *streamWriter) Header() http.Header { return w.header }
func (w *streamWriter) WriteHeader(status int) {
	if !w.started {
		w.started = true
		w.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		w.err = w.conn.WriteJSON(Response{Status: status, ContentType: w.header.Get("Content-Type")})
	}
}
func (w *streamWriter) Write(data []byte) (int, error) {
	if !w.started {
		w.WriteHeader(200)
	}
	if w.err != nil {
		return 0, w.err
	}
	total := 0
	for len(data) > 0 {
		n := len(data)
		if n > 32*1024 {
			n = 32 * 1024
		}
		w.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if w.err = w.conn.WriteMessage(websocket.BinaryMessage, data[:n]); w.err != nil {
			return total, w.err
		}
		total += n
		data = data[n:]
	}
	return total, nil
}
func (w *streamWriter) Flush() {}
func (w *streamWriter) finish() {
	if !w.started {
		w.WriteHeader(200)
	}
	if w.err == nil {
		w.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	}
}
