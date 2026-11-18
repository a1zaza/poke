package cluster

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"poke/pkg/images"
)

type AgentOptions struct {
	ControllerURL, NodeID, NodeToken, CAFile string
	AllowHTTP                                bool
}
type runtimeTask struct {
	task     Task
	request  *Request
	prepared *Preparation
	hash     string
	permit   chan Frame
	done     bool
}
type Agent struct {
	Executor     Executor
	Options      AgentOptions
	bootID       string
	session      string
	epoch        uint64
	leaseUntil   time.Time
	leaseRequest string
	leaseSent    time.Time
	tasks        map[string]*runtimeTask
	client       *http.Client
	dialer       *websocket.Dialer
	base         *url.URL
	mu           sync.Mutex
	stopping     bool
	peer         *Peer
	running      map[string]context.CancelFunc
	workers      chan struct{}
	writeMu      sync.Mutex
	wg           sync.WaitGroup
}

func NewAgent(executor Executor, opts AgentOptions) (*Agent, error) {
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
	transport.TLSClientConfig = tlsConfig.Clone()
	// net/http may add h2 to its TLS configuration. Gorilla's Upgrade handshake
	// requires HTTP/1.1, so its configuration must remain independent.
	websocketTLS := tlsConfig.Clone()
	websocketTLS.NextProtos = []string{"http/1.1"}
	a := &Agent{
		Executor: executor, Options: opts, base: u, bootID: randomID(), tasks: map[string]*runtimeTask{},
		running: map[string]context.CancelFunc{}, workers: make(chan struct{}, 8),
		client: &http.Client{
			Transport: transport, Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		dialer: &websocket.Dialer{
			Proxy: http.ProxyFromEnvironment, TLSClientConfig: websocketTLS,
			HandshakeTimeout: 10 * time.Second,
		},
	}
	opts.NodeToken = strings.TrimSpace(opts.NodeToken)
	if len(opts.NodeToken) != 64 {
		return nil, errors.New("需要有效的长期 POKE_NODE_TOKEN")
	}
	a.Options.NodeToken = opts.NodeToken
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
	h.Set("Authorization", "Bearer "+a.Options.NodeToken)
	h.Set("X-Poke-Node", a.Options.NodeID)
	h.Set("X-Poke-Boot", a.bootID)
	return h
}
func (a *Agent) send(f Frame) error {
	a.mu.Lock()
	p := a.peer
	f.Session = a.session
	f.Epoch = a.epoch
	f.BootID = a.bootID
	a.mu.Unlock()
	if p == nil {
		return errors.New("控制连接离线")
	}
	return p.Send(f)
}
func (a *Agent) Run(ctx context.Context) error {
	defer a.client.CloseIdleConnections()
	watchdogDone := make(chan struct{})
	defer close(watchdogDone)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-watchdogDone:
				return
			case <-ticker.C:
				a.mu.Lock()
				if !a.leaseUntil.IsZero() && !time.Now().Before(a.leaseUntil) {
					for _, cancel := range a.running {
						cancel()
					}
					if a.peer != nil {
						a.peer.Close()
					}
				}
				a.mu.Unlock()
			}
		}
	}()
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
		connectionStarted := time.Now()
		conn, resp, err := a.dialer.DialContext(ctx, a.endpoint("/agent/v1/connect", true), a.headers())
		if err == nil {
			p := NewPeer(conn)
			a.mu.Lock()
			a.peer = p
			a.mu.Unlock()
			connectedAt := time.Now()
			err = a.serve(ctx, p, connectionStarted)
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
			if resp.StatusCode == 409 {
				var diagnostic struct {
					Code  string `json:"code"`
					Error string `json:"error"`
				}
				if json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&diagnostic) == nil {
					err = fmt.Errorf("%s: %s", diagnostic.Code, diagnostic.Error)
				}
			}
			resp.Body.Close()
		}
		if ctx.Err() != nil {
			break
		}
		if errors.Is(err, ErrProtocolVersion) {
			return err
		}
		if resp != nil && (resp.StatusCode == 401 || resp.StatusCode == 403) {
			return errors.New("节点身份已撤销或过期，请检查 POKE_NODE_ID、POKE_NODE_TOKEN 或在 Controller 轮换 Token")
		}
		log.Printf("AGENT: 节点 %s 控制连接断开，将退避重试: %v", a.Options.NodeID, err)
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
func (a *Agent) serve(ctx context.Context, p *Peer, connectionStarted time.Time) error {
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			a.mu.Lock()
			a.stopping = true
			for _, cancel := range a.running {
				cancel()
			}
			a.mu.Unlock()
			a.wg.Wait()
			_ = a.send(Frame{Type: "release"})
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
	a.mu.Lock()
	known := []string{}
	for id, t := range a.tasks {
		if !t.task.Resolved {
			known = append(known, id)
		}
	}
	a.mu.Unlock()
	if err = p.Send(Frame{Pending: known, Type: "hello", ID: f.ID, Info: &AgentInfo{Hostname: hostname, Platform: runtime.GOOS + "/" + runtime.GOARCH}, BootID: a.bootID}); err != nil {
		return err
	}
	welcome, err := p.Read()
	if err != nil || welcome.Type != "welcome" || !safeID.MatchString(welcome.Session) || welcome.Epoch == 0 {
		return errors.New("Controller 会话确认失败")
	}
	a.mu.Lock()
	a.session = welcome.Session
	a.epoch = welcome.Epoch
	a.leaseUntil = connectionStarted.Add(45 * time.Second)
	pending := []Frame{}
	for _, t := range a.tasks {
		if t.done && !t.task.Resolved {
			copy := t.task
			pending = append(pending, Frame{Type: "result", ID: copy.ID, Task: &copy})
		} else if t.prepared != nil && t.task.Permit == "" {
			pending = append(pending, Frame{Type: "ready", ID: t.task.ID, Hash: t.hash, Domains: t.prepared.Domains})
		}
	}
	a.mu.Unlock()
	for _, f := range pending {
		if err := a.send(f); err != nil {
			return err
		}
	}
	heartbeatDone := make(chan struct{})
	defer close(heartbeatDone)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		var renewal time.Time
		for {
			select {
			case <-heartbeatDone:
				return
			case <-ticker.C:
				a.mu.Lock()
				expired := !time.Now().Before(a.leaseUntil)
				a.mu.Unlock()
				if expired {
					a.cancelAll()
					p.Close()
					return
				}
				if time.Since(renewal) >= 20*time.Second {
					renewal = time.Now()
					requestID := randomID()
					a.mu.Lock()
					a.leaseRequest = requestID
					a.leaseSent = renewal
					a.mu.Unlock()
					if a.send(Frame{Type: "lease", ID: requestID}) != nil {
						return
					}
				}
			}
		}
	}()
	for {
		f, err := p.Read()
		if err != nil {
			return err
		}
		a.mu.Lock()
		current := f.Session == a.session && f.Epoch == a.epoch && f.BootID == a.bootID && time.Now().Before(a.leaseUntil)
		a.mu.Unlock()
		if !current {
			return errors.New("控制消息会话无效或租约已过期")
		}
		switch f.Type {
		case "lease":
			a.mu.Lock()
			if f.ID == a.leaseRequest && !a.leaseSent.IsZero() {
				a.leaseUntil = a.leaseSent.Add(45 * time.Second)
				a.leaseRequest = ""
			}
			a.mu.Unlock()
		case "execute":
			if f.Request == nil {
				return errors.New("执行许可缺少上下文")
			}
			a.mu.Lock()
			t := a.tasks[f.ID]
			if t != nil && !t.done && t.task.Permit == "" && t.hash == f.Hash && safeID.MatchString(f.Permit) {
				t.task.Permit = f.Permit
				select {
				case t.permit <- f:
				default:
				}
			}
			a.mu.Unlock()
		case "open":
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !safeID.MatchString(f.Stream) || len(f.Ticket) != 64 || !ValidRequest(f.Request, f.Kind == "exec") || (f.Kind != "http" && f.Kind != "exec") {
				return errors.New("非法数据流请求")
			}
			select {
			case a.workers <- struct{}{}:
			default:
				if err = a.send(Frame{Type: "stream_error", ID: f.Stream}); err != nil {
					return err
				}
				continue
			}
			a.mu.Lock()
			if a.stopping {
				a.mu.Unlock()
				<-a.workers
				return ctx.Err()
			}
			a.wg.Add(1)
			a.mu.Unlock()
			go func(f Frame) { defer a.wg.Done(); defer func() { <-a.workers }(); a.stream(ctx, p, f) }(f)
		case "task":
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !safeID.MatchString(f.ID) || !ValidRequest(f.Request, false) || f.Request.Method == "GET" {
				return errors.New("非法任务请求")
			}
			if err = a.startTask(ctx, f); err != nil {
				return err
			}
		case "cancel":
			a.mu.Lock()
			if t := a.tasks[f.ID]; t != nil && t.task.Permit == "" && f.Permit != "" {
				t.task.Permit = f.Permit
			}
			cancel := a.running[f.ID]
			a.mu.Unlock()
			if cancel != nil {
				cancel()
			}
		case "ack":
			a.mu.Lock()
			if t := a.tasks[f.ID]; t != nil && t.done {
				t.task.Resolved = true
				t.task.Response = nil
				t.request = nil
				t.prepared = nil
			}
			if len(a.tasks) > 2048 {
				for id, t := range a.tasks {
					if t.done && t.task.Resolved {
						delete(a.tasks, id)
						if len(a.tasks) <= 1024 {
							break
						}
					}
				}
			}
			a.mu.Unlock()
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
	req = withProject(req, f.Request.Project)
	req.Header.Set("Content-Type", "application/json")
	writer := &streamWriter{conn: conn, header: make(http.Header)}
	a.Executor.ServeHTTP(writer, req)
	writer.finish()
}
func (a *Agent) cancelAll() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, cancel := range a.running {
		cancel()
	}
}
func (a *Agent) startTask(root context.Context, f Frame) error {
	a.mu.Lock()
	if a.stopping || root.Err() != nil {
		a.mu.Unlock()
		return root.Err()
	}
	if old := a.tasks[f.ID]; old != nil {
		copy := old.task
		done := old.done && !old.task.Resolved
		a.mu.Unlock()
		if done {
			return a.send(Frame{Type: "result", ID: f.ID, Task: &copy})
		}
		return nil
	}
	if len(a.tasks) >= 2048 {
		for id, t := range a.tasks {
			if t.done && t.task.Resolved {
				delete(a.tasks, id)
			}
		}
		if len(a.tasks) >= 2048 {
			a.mu.Unlock()
			return errors.New("待确认结果过多，等待重连对账")
		}
	}
	if len(a.running) >= 8 {
		failed := Task{ID: f.ID, BootID: a.bootID, NodeID: a.Options.NodeID, State: "failed", Response: &Response{Status: 429, ContentType: "application/json", Body: []byte(`{"error":"节点任务队列已满"}`)}}
		a.tasks[f.ID] = &runtimeTask{task: failed, done: true}
		a.mu.Unlock()
		return a.send(Frame{Type: "result", ID: f.ID, Task: &failed})
	}
	ctx, cancel := context.WithTimeout(root, 10*time.Minute)
	t := &runtimeTask{task: Task{ID: f.ID, NodeID: a.Options.NodeID, BootID: a.bootID, Method: f.Request.Method, Path: f.Request.Path, State: "accepted", CreatedAt: time.Now().UTC()}, request: f.Request, permit: make(chan Frame, 1)}
	a.tasks[f.ID] = t
	a.running[f.ID] = cancel
	a.wg.Add(1)
	a.mu.Unlock()
	go func() {
		defer a.wg.Done()
		defer cancel()
		defer func() { a.mu.Lock(); delete(a.running, f.ID); a.mu.Unlock() }()
		a.writeMu.Lock()
		defer a.writeMu.Unlock()
		result := Task{ID: f.ID, NodeID: a.Options.NodeID, BootID: a.bootID, State: "failed"}
		if ctx.Err() != nil {
			result.State = "cancelled"
			result.Error = "任务在执行前已取消"
		} else {
			prep, err := a.Executor.Prepare(ctx, f.Request)
			if prep.Cleanup != nil {
				defer prep.Cleanup()
			}
			if err != nil {
				result.Response = &Response{Status: 400, ContentType: "application/json", Body: mustJSON(map[string]string{"error": err.Error()})}
			} else {
				hash := requestHash(prep.Request)
				a.mu.Lock()
				t.prepared = &prep
				t.hash = hash
				a.mu.Unlock()
				_ = a.send(Frame{Type: "ready", ID: f.ID, Hash: hash, Domains: prep.Domains})
				select {
				case <-ctx.Done():
					result.State = "cancelled"
					result.Error = "未获得执行许可，任务已取消"
				case permit := <-t.permit:
					result.Permit = permit.Permit
					a.mu.Lock()
					valid := a.session == permit.Session && a.epoch == permit.Epoch && time.Now().Before(a.leaseUntil)
					a.mu.Unlock()
					if !valid || ctx.Err() != nil {
						result.State = "unknown"
						result.Error = "执行许可会话已失效，请确认状态"
					} else {
						req, _ := http.NewRequestWithContext(images.WithCredentials(ctx, permit.Request.Registries), prep.Request.Method, prep.Request.Path, bytes.NewReader(prep.Request.Body))
						req = withProject(req, prep.Request.Project)
						req.Header.Set("Content-Type", "application/json")
						writer := &boundedWriter{header: make(http.Header)}
						a.Executor.ServeHTTP(writer, req)
						response := writer.result()
						result.Response = &response
						result.State = "succeeded"
						if response.Status >= 400 {
							result.State = "failed"
						}
						if ctx.Err() != nil || writer.overflow {
							result.State = "unknown"
							result.Error = "执行中断或超时，结果需要确认"
						}
					}
				}
			}
		}
		a.mu.Lock()
		if result.Permit == "" && t.task.Permit != "" {
			result.Permit = t.task.Permit
			result.State = "unknown"
			result.Error = "执行许可到达时任务已中断，请确认结果"
		}
		t.task = result
		t.done = true
		t.prepared = nil
		copy := t.task
		a.mu.Unlock()
		_ = a.send(Frame{Type: "result", ID: f.ID, Task: &copy})
	}()
	return nil
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

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
