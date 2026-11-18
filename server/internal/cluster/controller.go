package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"poke/pkg/config"
	"poke/pkg/images"
)

type session struct {
	ready        bool
	id           string
	conflicts    int
	lastConflict time.Time
	info         *AgentInfo
	*Peer
	node    Node
	streams map[string]*dataStream
}
type dataStream struct {
	id, ticket, kind string
	sess             *session
	ready            chan *websocket.Conn
	done             chan struct{}
	once             sync.Once
	conn             *websocket.Conn
	reserved         bool
	failure          error
}
type Controller struct {
	wg         sync.WaitGroup
	Store      *Store
	mu         sync.Mutex
	sessions   map[string]*session
	streams    map[string]*dataStream
	closed     bool
	stop       chan struct{}
	Registries func() []config.RegistryConfig
}

func NewController(s *Store) *Controller {
	c := &Controller{Store: s, sessions: map[string]*session{}, streams: map[string]*dataStream{}, stop: make(chan struct{}), Registries: func() []config.RegistryConfig { return config.GetConfig().System.Registries }}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-c.stop:
				return
			case <-ticker.C:
				c.expireLeases()
			}
		}
	}()
	return c
}
func (c *Controller) Register(e *echo.Echo, g *echo.Group) {
	e.GET("/agent/v1/connect", c.connect)
	e.GET("/agent/v1/stream", c.acceptStream)
	g.GET("/nodes", c.listNodes)
	g.POST("/nodes", c.createNode)
	g.POST("/nodes/:node/projects/import", c.importProject)
	g.PUT("/nodes/:node/projects/:name/source", c.saveProjectSource)
	g.DELETE("/nodes/:node", c.revoke)
	g.PATCH("/nodes/:node", c.updateNode)
	g.POST("/nodes/:node/token", c.rotateToken)
	g.Any("/nodes/:node/api/*", c.dispatch)
	g.GET("/tasks", func(ctx echo.Context) error { return ctx.JSON(200, c.Store.Tasks()) })
	g.GET("/tasks/:task", func(ctx echo.Context) error {
		t, err := c.Store.Task(ctx.Param("task"))
		if err != nil {
			return echo.ErrNotFound
		}
		return ctx.JSON(200, t)
	})
	g.POST("/tasks/:task/cancel", c.cancelTask)
	g.POST("/tasks/:task/resolve", c.resolveTask)
}
func (c *Controller) NodeViews() []map[string]any {
	ns := c.Store.Nodes()
	out := make([]map[string]any, 0, len(ns))
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, n := range ns {
		state := "pending"
		if n.Registered {
			state = "offline"
		}
		if s, ok := c.sessions[n.ID]; ok && s.ready && c.Store.ValidLease(n.ID, s.node.BootID, s.node.Epoch) {
			state = "online"
		}
		if n.Revoked {
			state = "revoked"
		}
		view := map[string]any{"id": n.ID, "name": n.Name, "registries": n.Registries, "status": state, "created_at": n.CreatedAt, "boot_id": n.BootID, "lease_until": n.LeaseUntil, "blocked": c.Store.NodeBlocked(n.ID)}
		if s := c.sessions[n.ID]; s != nil {
			view["duplicate_attempts"] = s.conflicts
			view["last_duplicate_at"] = s.lastConflict
		}
		out = append(out, view)
	}
	return out
}
func (c *Controller) listNodes(ctx echo.Context) error { return ctx.JSON(200, c.NodeViews()) }
func (c *Controller) createNode(ctx echo.Context) error {
	var req struct {
		Name       string   `json:"name"`
		Registries []string `json:"registries"`
	}
	if err := json.NewDecoder(io.LimitReader(ctx.Request().Body, 16384)).Decode(&req); err != nil {
		return echo.ErrBadRequest
	}
	n, token, err := c.Store.CreateNode(req.Name, req.Registries)
	if err != nil {
		return echo.NewHTTPError(400, err.Error())
	}
	ctx.Response().Header().Set("Cache-Control", "no-store")
	return ctx.JSON(201, map[string]any{"node": n, "node_token": token})
}
func (c *Controller) updateNode(ctx echo.Context) error {
	var req struct {
		Name       string   `json:"name"`
		Registries []string `json:"registries"`
	}
	if json.NewDecoder(io.LimitReader(ctx.Request().Body, 16384)).Decode(&req) != nil {
		return echo.ErrBadRequest
	}
	if err := c.Store.UpdateNode(ctx.Param("node"), req.Name, req.Registries); err != nil {
		return echo.NewHTTPError(400, err.Error())
	}
	c.disconnect(ctx.Param("node"))
	return ctx.NoContent(204)
}

func (c *Controller) revoke(ctx echo.Context) error {
	if err := c.Store.Revoke(ctx.Param("node")); err != nil {
		return echo.ErrNotFound
	}
	c.disconnect(ctx.Param("node"))
	return ctx.NoContent(204)
}
func (c *Controller) rotateToken(ctx echo.Context) error {
	token, err := c.Store.RotateToken(ctx.Param("node"))
	if err != nil {
		return echo.ErrNotFound
	}
	c.disconnect(ctx.Param("node"))
	ctx.Response().Header().Set("Cache-Control", "no-store")
	return ctx.JSON(200, map[string]any{"node_id": ctx.Param("node"), "node_token": token})
}
func (c *Controller) authenticate(r *http.Request) (Node, error) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || parts[0] != "Bearer" {
		return Node{}, ErrUnauthorized
	}
	return c.Store.Authenticate(r.Header.Get("X-Poke-Node"), parts[1])
}

// Agent endpoints accept no browser Origin. Browser WebSockets use a separate authenticated route.
var machineUpgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == "" }, HandshakeTimeout: 10 * time.Second}

func (c *Controller) connect(ctx echo.Context) error {
	n, err := c.authenticate(ctx.Request())
	if err != nil {
		return echo.ErrUnauthorized
	}
	boot := ctx.Request().Header.Get("X-Poke-Boot")
	if !safeID.MatchString(boot) {
		return echo.NewHTTPError(400, "节点启动标识无效")
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return echo.ErrServiceUnavailable
	}
	old := c.sessions[n.ID]
	parts := strings.Fields(ctx.Request().Header.Get("Authorization"))
	n, err = c.Store.Claim(n.ID, parts[1], boot)
	if err != nil {
		code := "NODE_ALREADY_CONNECTED"
		if errors.Is(err, ErrNodeConnected) {
			if old == nil {
				code = "NODE_RECONNECT_PENDING"
			} else {
				old.conflicts++
				old.lastConflict = time.Now().UTC()
			}
			c.mu.Unlock()
			return ctx.JSON(409, map[string]string{"code": code, "error": "同一节点已有活动实例，请停止重复 Agent 或等待租约到期"})
		}
		c.mu.Unlock()
		if errors.Is(err, ErrUnauthorized) {
			return echo.ErrUnauthorized
		}
		return echo.ErrInternalServerError
	}
	// Reserve under the same lock before allowing another handshake.
	s := &session{node: n, id: randomID(), streams: map[string]*dataStream{}}
	c.sessions[n.ID] = s
	if old != nil {
		c.closeSessionLocked(old)
	}
	c.wg.Add(1)
	c.mu.Unlock()
	defer c.wg.Done()
	defer func() {
		c.mu.Lock()
		if c.sessions[n.ID] == s {
			delete(c.sessions, n.ID)
		}
		notReady := !s.ready
		c.closeSessionLocked(s)
		c.mu.Unlock()
		if notReady {
			_ = c.Store.ReleaseLease(n.ID, boot, n.Epoch)
		}
	}()
	conn, err := machineUpgrader.Upgrade(ctx.Response(), ctx.Request(), nil)
	if err != nil {
		_ = c.Store.ReleaseLease(n.ID, boot, n.Epoch)
		return err
	}
	p := NewPeer(conn)
	c.mu.Lock()
	if c.sessions[n.ID] != s || c.closed {
		c.mu.Unlock()
		p.Close()
		return nil
	}
	s.Peer = p
	c.mu.Unlock()
	if err := c.Store.RecoverNode(n.ID, boot); err != nil {
		return nil
	}
	challenge := randomID()
	if err := s.Send(Frame{Type: "hello", ID: challenge}); err != nil {
		return nil
	}
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	hello, err := s.Read()
	if err != nil || hello.Type != "hello" || hello.ID != challenge || hello.BootID != boot || hello.Info == nil || len(hello.Pending) > 2048 {
		return nil
	}
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.mu.Lock()
	if c.sessions[n.ID] != s {
		c.mu.Unlock()
		return nil
	}
	s.info = hello.Info
	if err := c.Store.ReconcilePending(n.ID, boot, hello.Pending); err != nil {
		c.mu.Unlock()
		return nil
	}
	if err := s.send(Frame{Type: "welcome", LeaseSeconds: 60}); err != nil {
		c.mu.Unlock()
		return nil
	}
	s.ready = true
	c.mu.Unlock()
	for {
		f, err := s.Read()
		if err != nil {
			return nil
		}
		c.mu.Lock()
		current := c.sessions[n.ID] == s && f.Session == s.id && f.Epoch == n.Epoch && f.BootID == boot && c.Store.ValidLease(n.ID, boot, n.Epoch)
		if !current {
			c.mu.Unlock()
			return nil
		}
		switch f.Type {
		case "lease":
			err = c.Store.RenewLease(n.ID, boot, n.Epoch, n.CredentialVersion)
			if err == nil {
				err = s.send(Frame{Type: "lease", ID: f.ID, LeaseSeconds: 60})
			}
		case "release":
			err = c.Store.ReleaseLease(n.ID, boot, n.Epoch)
			c.mu.Unlock()
			return err
		case "stream_error":
			if st := c.streams[f.ID]; st != nil && st.sess == s {
				st.failure = echo.NewHTTPError(429, "节点数据连接已达上限")
				c.releaseLocked(st)
			}
		case "ready":
			err = c.grant(s, f)
		case "result":
			err = c.result(s, f)
		default:
			err = errors.New("未知控制消息")
		}
		c.mu.Unlock()
		if err != nil {
			return nil
		}
	}
}
func (s *session) send(f Frame) error {
	f.Session = s.id
	f.Epoch = s.node.Epoch
	f.BootID = s.node.BootID
	return s.Send(f)
}

// Called under controller.mu: permission is durably recorded before it can be sent.
func (c *Controller) grant(s *session, f Frame) error {
	t, err := c.Store.Task(f.ID)
	if err != nil || t.NodeID != s.node.ID || t.BootID != s.node.BootID {
		return nil
	}
	if t.Terminal() || t.Permit != "" {
		return s.send(Frame{Type: "cancel", ID: t.ID, Permit: t.Permit})
	}
	if c.Store.NodeBlocked(t.NodeID) {
		return c.rejectTask(s, t, "存在结果待确认的任务，请先对账")
	}
	if len(f.Hash) != 64 || len(f.Domains) > 64 {
		return errors.New("任务准备信息无效")
	}
	expected := requiredDomains(t.Request)
	// For simple image calls the Controller can validate the exact registry itself.
	if expected != nil && !sameDomains(expected, f.Domains) {
		return errors.New("仓库范围与操作不一致")
	}
	t.Permit = randomID()
	t.PreparedHash = f.Hash
	t.State = "execution_granted"
	if err := c.Store.SaveTask(t); err != nil {
		return err
	}
	if !c.Store.ValidLease(s.node.ID, s.node.BootID, s.node.Epoch) {
		_ = c.Store.UpdateTask(t.ID, func(t *Task) {
			t.State = "unknown"
			t.Error = "执行许可持久化期间租约失效，请确认状态"
		})
		return errors.New("节点租约已失效")
	}
	return s.send(Frame{Type: "execute", ID: t.ID, Permit: t.Permit, Hash: t.PreparedHash, Request: &Request{Registries: c.operationCredentials(t.NodeID, f.Domains)}})
}
func (c *Controller) rejectTask(s *session, t Task, message string) error {
	t.State = "cancelled"
	t.Error = message
	if err := c.Store.SaveTask(t); err != nil {
		return err
	}
	return s.send(Frame{Type: "cancel", ID: t.ID})
}
func (c *Controller) result(s *session, f Frame) error {
	t, err := c.Store.Task(f.ID)
	if err != nil || t.NodeID != s.node.ID || t.BootID != s.node.BootID || f.Task == nil {
		return nil
	}
	if t.Terminal() && (t.State != "unknown" || t.Resolved) {
		return s.send(Frame{Type: "ack", ID: t.ID})
	}
	reported := f.Task
	if t.Permit != "" && reported.Permit == "" && (reported.State == "cancelled" || reported.State == "failed") {
		t.State = "unknown"
		t.Error = "Controller 已保存执行许可，但节点未确认收到；请核实实际状态"
		t.Response = nil
		if err := c.Store.SaveTask(t); err != nil {
			return err
		}
		return s.send(Frame{Type: "ack", ID: t.ID})
	}
	if !reported.Terminal() || reported.State == "unknown" && t.Permit == "" || reported.Permit != t.Permit {
		return errors.New("任务结果许可无效")
	}
	if t.Permit == "" && reported.State == "succeeded" {
		return errors.New("任务没有执行许可")
	}
	if reported.Response != nil && (len(reported.Response.Body) > MaxResult || reported.Response.Status < 100 || reported.Response.Status > 599) {
		return errors.New("任务结果超过限制")
	}
	t.State = reported.State
	t.Error = reported.Error
	t.Response = reported.Response
	if err = c.finishProject(t); err != nil {
		return err
	}
	if err = c.Store.SaveTask(t); err != nil {
		return err
	}
	return s.send(Frame{Type: "ack", ID: t.ID})
}
func requiredDomains(r *Request) []string {
	if r == nil {
		return []string{}
	}
	path := strings.SplitN(r.Path, "?", 2)[0]
	if path == "/api/images/pull" || path == "/api/images/check-update" {
		var body struct {
			Image string `json:"image"`
		}
		_ = json.Unmarshal(r.Body, &body)
		domain, _, _ := images.ParseRepositoryInfo(body.Image)
		return []string{domain}
	}
	if r.Project != nil || strings.HasSuffix(path, "/update") {
		return nil
	}
	return []string{}
}
func sameDomains(a, b []string) bool {
	set := map[string]bool{}
	for _, d := range a {
		set[images.NormalizeRegistry(d)] = true
	}
	for _, d := range b {
		if !set[images.NormalizeRegistry(d)] {
			return false
		}
		delete(set, images.NormalizeRegistry(d))
	}
	return len(set) == 0
}
func (c *Controller) operationCredentials(node string, domains []string) []config.RegistryConfig {
	out := []config.RegistryConfig{}
	for _, r := range c.credentials(node) {
		for _, d := range domains {
			if images.NormalizeRegistry(r.Domain) == images.NormalizeRegistry(d) {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

func (c *Controller) closeSessionLocked(s *session) {
	if s.Peer != nil {
		s.Close()
	}
	for _, st := range s.streams {
		c.releaseLocked(st)
	}
}
func (c *Controller) disconnect(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s := c.sessions[id]; s != nil {
		delete(c.sessions, id)
		c.closeSessionLocked(s)
	}
}
func (c *Controller) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	close(c.stop)
	for id, s := range c.sessions {
		c.closeSessionLocked(s)
		delete(c.sessions, id)
	}
	c.mu.Unlock()
	c.wg.Wait()
}
func (c *Controller) releaseLocked(st *dataStream) {
	st.once.Do(func() {
		if st.conn != nil {
			st.conn.Close()
		}
		close(st.done)
		delete(c.streams, st.id)
		delete(st.sess.streams, st.id)
	})
}
func (c *Controller) release(st *dataStream) { c.mu.Lock(); defer c.mu.Unlock(); c.releaseLocked(st) }
func (c *Controller) open(ctx context.Context, node, kind string, req *Request) (*dataStream, *websocket.Conn, error) {
	c.mu.Lock()
	s := c.sessions[node]
	if s == nil || !s.ready || !c.Store.ValidLease(node, s.node.BootID, s.node.Epoch) {
		c.mu.Unlock()
		return nil, nil, echo.NewHTTPError(503, "节点离线")
	}
	if len(s.streams) >= 16 {
		c.mu.Unlock()
		return nil, nil, echo.NewHTTPError(429, "节点连接数量已达上限")
	}
	st := &dataStream{id: randomID(), ticket: secret(), kind: kind, sess: s, ready: make(chan *websocket.Conn, 1), done: make(chan struct{})}
	c.streams[st.id] = st
	s.streams[st.id] = st
	c.mu.Unlock()
	if err := s.send(Frame{Type: "open", ID: st.id, Stream: st.id, Ticket: st.ticket, Kind: kind, Request: req}); err != nil {
		c.release(st)
		return nil, nil, echo.NewHTTPError(503, "节点连接已断开")
	}
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case conn := <-st.ready:
		return st, conn, nil
	case <-ctx.Done():
		c.release(st)
		return nil, nil, ctx.Err()
	case <-st.done:
		if st.failure != nil {
			return nil, nil, st.failure
		}
		return nil, nil, echo.NewHTTPError(503, "节点连接已断开")
	case <-timer.C:
		c.release(st)
		return nil, nil, echo.NewHTTPError(504, "节点建立数据连接超时")
	}
}
func (c *Controller) acceptStream(ctx echo.Context) error {
	n, err := c.authenticate(ctx.Request())
	if err != nil {
		return echo.ErrUnauthorized
	}
	c.mu.Lock()
	st := c.streams[ctx.Request().Header.Get("X-Poke-Stream")]
	if st == nil || !c.Store.ValidLease(n.ID, st.sess.node.BootID, st.sess.node.Epoch) || st.sess.node.ID != n.ID || c.sessions[n.ID] != st.sess || st.conn != nil || st.reserved || !matches(ctx.Request().Header.Get("X-Poke-Ticket"), digest(st.ticket)) {
		c.mu.Unlock()
		return echo.ErrUnauthorized
	}
	st.reserved = true
	c.mu.Unlock()
	conn, err := machineUpgrader.Upgrade(ctx.Response(), ctx.Request(), nil)
	if err != nil {
		c.release(st)
		return err
	}
	c.mu.Lock()
	if c.streams[st.id] != st || c.sessions[n.ID] != st.sess {
		c.mu.Unlock()
		conn.Close()
		return nil
	}
	conn.SetReadLimit(MaxFrame)
	st.conn = conn
	st.ticket = ""
	st.ready <- conn
	c.mu.Unlock()
	<-st.done
	return nil
}
func (c *Controller) credentials(node string) []config.RegistryConfig {
	var allowed []string
	for _, n := range c.Store.Nodes() {
		if n.ID == node && !n.Revoked {
			allowed = n.Registries
			break
		}
	}
	result := []config.RegistryConfig{}
	for _, r := range c.Registries() {
		for _, domain := range allowed {
			if images.NormalizeRegistry(domain) == images.NormalizeRegistry(r.Domain) {
				result = append(result, r)
				break
			}
		}
	}
	return result
}
func (c *Controller) dispatch(ctx echo.Context) error {
	node := ctx.Param("node")
	prefix := "/api/nodes/" + node
	body, err := io.ReadAll(io.LimitReader(ctx.Request().Body, MaxBody+1))
	if err != nil || len(body) > MaxBody {
		return echo.NewHTTPError(413, "请求内容超过限制")
	}
	path := strings.TrimPrefix(ctx.Request().URL.Path, prefix)
	if ctx.Request().URL.RawQuery != "" {
		path += "?" + ctx.Request().URL.RawQuery
	}
	req := &Request{Method: ctx.Request().Method, Path: path, Body: body}
	if !ValidRequest(req, false) {
		return echo.ErrNotFound
	}
	if isReadOnlyRequest(req) {
		req.Registries = c.operationCredentials(node, requiredDomains(req))
	}
	if handled, err := c.projectRequest(ctx, node, req); handled {
		return err
	}
	if !isReadOnlyRequest(req) {
		c.mu.Lock()
		s := c.sessions[node]
		ready := s != nil && s.ready && c.Store.ValidLease(node, s.node.BootID, s.node.Epoch)
		blocked := c.Store.NodeBlocked(node)
		c.mu.Unlock()
		if blocked {
			return echo.NewHTTPError(409, "存在结果待确认的任务，请在节点与任务中对账后解除")
		}
		if !ready {
			message := "节点离线"
			if req.Project != nil {
				message = "节点离线，Controller 已保留源配置，尚未执行本次部署"
			}
			return echo.NewHTTPError(503, message)
		}
		task := Task{ID: randomID(), NodeID: node, Method: req.Method, Path: req.Path, State: "queued", CreatedAt: time.Now().UTC(), Request: req, BootID: s.node.BootID}
		if err = c.Store.SaveTask(task); err != nil {
			return echo.ErrInternalServerError
		}
		if err = s.send(Frame{Type: "task", ID: task.ID, Request: req}); err != nil {
			if c.Store.UpdateTask(task.ID, func(t *Task) {
				if !t.Terminal() {
					if t.Permit == "" {
						t.State = "cancelled"
						t.Error = "任务发送中断，尚未授权执行"
					} else {
						t.State = "unknown"
						t.Error = "发送任务时连接断开，请确认节点执行状态"
					}
				}
			}) != nil {
				return echo.ErrInternalServerError
			}
		}
		return ctx.JSON(202, map[string]string{"task_id": task.ID})
	}
	requestCtx, cancel := context.WithTimeout(ctx.Request().Context(), 30*time.Second)
	defer cancel()
	st, conn, err := c.open(requestCtx, node, "http", req)
	if err != nil {
		return err
	}
	defer c.release(st)
	go func() {
		select {
		case <-requestCtx.Done():
			conn.Close()
		case <-st.done:
		}
	}()
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	var header Response
	if err = conn.ReadJSON(&header); err != nil {
		return echo.NewHTTPError(502, "节点响应失败")
	}
	if header.Status < 100 || header.Status > 599 {
		return echo.NewHTTPError(502, "节点响应无效")
	}
	ctx.Response().Header().Set("Content-Type", "application/json; charset=utf-8")
	ctx.Response().Header().Set("X-Content-Type-Options", "nosniff")
	ctx.Response().Header().Set("Cache-Control", "no-store")
	ctx.Response().WriteHeader(header.Status)
	for {
		kind, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				return nil
			}
			return err
		}
		if kind != websocket.BinaryMessage {
			return errors.New("数据流协议错误")
		}
		if _, err = ctx.Response().Write(data); err != nil {
			return err
		}
		ctx.Response().Flush()
	}
}
func (c *Controller) cancelTask(ctx echo.Context) error {
	t, err := c.Store.Task(ctx.Param("task"))
	if err != nil {
		return echo.ErrNotFound
	}
	if t.Terminal() {
		return ctx.JSON(200, t)
	}
	c.mu.Lock()
	s := c.sessions[t.NodeID]
	ready := s != nil && s.ready
	c.mu.Unlock()
	if !ready {
		return echo.NewHTTPError(503, "节点离线，无法确认取消")
	}
	if err = s.send(Frame{Type: "cancel", ID: t.ID}); err != nil {
		return echo.NewHTTPError(503, "节点连接已断开")
	}
	return ctx.JSON(202, map[string]string{"message": "已请求取消，请等待节点确认"})
}
func (c *Controller) Terminal(ctx echo.Context) error {
	node := ctx.Param("node")
	path := strings.TrimPrefix(ctx.Request().URL.Path, "/ws/nodes/"+node)
	req := &Request{Method: "GET", Path: "/ws" + path}
	if !ValidRequest(req, true) {
		return echo.ErrNotFound
	}
	st, agent, err := c.open(ctx.Request().Context(), node, "exec", req)
	if err != nil {
		return err
	}
	defer c.release(st)
	browser, err := (&websocket.Upgrader{HandshakeTimeout: 10 * time.Second}).Upgrade(ctx.Response(), ctx.Request(), nil)
	if err != nil {
		return err
	}
	defer browser.Close()
	heartbeat := NewPeer(browser)
	defer heartbeat.Close()
	browser.SetReadLimit(64 * 1024)
	pipe := func(dst, src *websocket.Conn, ch chan<- error) {
		for {
			k, data, err := src.ReadMessage()
			if err != nil {
				ch <- err
				return
			}
			dst.SetWriteDeadline(time.Now().Add(15 * time.Second))
			if err = dst.WriteMessage(k, data); err != nil {
				ch <- err
				return
			}
		}
	}
	done := make(chan error, 2)
	go pipe(agent, browser, done)
	go pipe(browser, agent, done)
	select {
	case <-done:
	case <-st.done:
	case <-ctx.Request().Context().Done():
	}
	return nil
}

func (c *Controller) resolveTask(ctx echo.Context) error {
	var req struct {
		Confirmed bool `json:"confirmed"`
	}
	if json.NewDecoder(io.LimitReader(ctx.Request().Body, 1024)).Decode(&req) != nil || !req.Confirmed {
		return echo.NewHTTPError(400, "请确认旧实例已停止并已核实资源实际状态")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	t, err := c.Store.Task(ctx.Param("task"))
	if err != nil {
		return echo.ErrNotFound
	}
	if t.State != "unknown" {
		return echo.NewHTTPError(409, "仅可确认结果待确认的任务")
	}
	if s := c.sessions[t.NodeID]; s != nil && s.node.BootID == t.BootID && c.Store.ValidLease(t.NodeID, s.node.BootID, s.node.Epoch) {
		return echo.NewHTTPError(409, "原执行实例仍持有租约，请先停止该实例并等待租约到期")
	}
	if err := c.Store.UpdateTask(t.ID, func(t *Task) { t.Resolved = true }); err != nil {
		return echo.ErrInternalServerError
	}
	return ctx.NoContent(204)
}

// Detect vanished processes even when no replacement Agent ever connects.
func (c *Controller) expireLeases() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	nodes := map[string]Node{}
	for _, n := range c.Store.Nodes() {
		nodes[n.ID] = n
	}
	for id, s := range c.sessions {
		if !c.Store.ValidLease(id, s.node.BootID, s.node.Epoch) {
			delete(c.sessions, id)
			c.closeSessionLocked(s)
		}
	}
	for _, meta := range c.Store.allTasks() {
		if meta.Terminal() {
			continue
		}
		n, ok := nodes[meta.NodeID]
		if ok && !n.Revoked && n.BootID == meta.BootID && time.Now().Before(n.LeaseUntil) {
			continue
		}
		if err := c.Store.UpdateTask(meta.ID, func(t *Task) {
			if t.Terminal() {
				return
			}
			if t.Permit == "" {
				t.State = "cancelled"
				t.Error = "节点租约已失效，未授权任务已取消"
			} else {
				t.State = "unknown"
				t.Error = "节点租约已失效，执行结果待确认；不会自动重放"
			}
		}); err != nil {
			// Stop dispatching this node if the uncertainty cannot be persisted.
			if s := c.sessions[meta.NodeID]; s != nil {
				delete(c.sessions, meta.NodeID)
				c.closeSessionLocked(s)
			}
		}
	}
}
