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
	ready bool
	info  *AgentInfo
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
	Registries func() []config.RegistryConfig
}

func NewController(s *Store) *Controller {
	return &Controller{Store: s, sessions: map[string]*session{}, streams: map[string]*dataStream{}, Registries: func() []config.RegistryConfig { return config.GetConfig().System.Registries }}
}
func (c *Controller) Register(e *echo.Echo, g *echo.Group) {
	e.POST("/agent/v1/enroll", c.enroll)
	e.GET("/agent/v1/connect", c.connect)
	e.GET("/agent/v1/stream", c.acceptStream)
	g.GET("/nodes", c.listNodes)
	g.POST("/nodes", c.createNode)
	g.DELETE("/nodes/:node", c.revoke)
	g.PATCH("/nodes/:node", c.updateNode)
	g.POST("/nodes/:node/enrollment", c.renewEnrollment)
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
		if s, ok := c.sessions[n.ID]; ok && s.ready {
			state = "online"
		}
		if n.Revoked {
			state = "revoked"
		}
		out = append(out, map[string]any{"id": n.ID, "name": n.Name, "registries": n.Registries, "status": state, "created_at": n.CreatedAt})
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
	return ctx.JSON(201, map[string]any{"node": n, "enrollment_token": token, "expires_at": n.EnrollmentExpires})
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

func (c *Controller) enroll(ctx echo.Context) error {
	var req struct {
		NodeID     string `json:"node_id"`
		Token      string `json:"token"`
		Attempt    string `json:"attempt"`
		Credential string `json:"credential"`
	}
	if err := json.NewDecoder(io.LimitReader(ctx.Request().Body, 8192)).Decode(&req); err != nil {
		return echo.ErrBadRequest
	}
	if err := c.Store.Enroll(req.NodeID, req.Token, req.Attempt, req.Credential); err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return echo.ErrUnauthorized
		}
		return echo.ErrInternalServerError
	}
	ctx.Response().Header().Set("Cache-Control", "no-store")
	return ctx.JSON(200, map[string]any{"version": Version, "node_id": req.NodeID})
}
func (c *Controller) revoke(ctx echo.Context) error {
	if err := c.Store.Revoke(ctx.Param("node")); err != nil {
		return echo.ErrNotFound
	}
	c.disconnect(ctx.Param("node"))
	return ctx.NoContent(204)
}
func (c *Controller) renewEnrollment(ctx echo.Context) error {
	token, err := c.Store.RenewEnrollment(ctx.Param("node"))
	if err != nil {
		return echo.ErrNotFound
	}
	c.disconnect(ctx.Param("node"))
	ctx.Response().Header().Set("Cache-Control", "no-store")
	return ctx.JSON(200, map[string]any{"node_id": ctx.Param("node"), "enrollment_token": token, "expires_at": time.Now().Add(10 * time.Minute)})
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
	conn, err := machineUpgrader.Upgrade(ctx.Response(), ctx.Request(), nil)
	if err != nil {
		return err
	}
	s := &session{Peer: NewPeer(conn), node: n, streams: map[string]*dataStream{}}
	defer s.Close()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	if _, err = c.authenticate(ctx.Request()); err != nil {
		c.mu.Unlock()
		return nil
	}
	c.wg.Add(1)
	defer c.wg.Done()
	old := c.sessions[n.ID]
	c.sessions[n.ID] = s
	if old != nil {
		c.closeSessionLocked(old)
	}
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		if c.sessions[n.ID] == s {
			delete(c.sessions, n.ID)
		}
		c.closeSessionLocked(s)
		c.mu.Unlock()
	}()
	challenge := randomID()
	if err = s.Send(Frame{Type: "hello", ID: challenge}); err != nil {
		return nil
	}
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	hello, err := s.Read()
	if err != nil || hello.Type != "hello" || hello.ID != challenge || hello.Info == nil {
		return nil
	}
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.mu.Lock()
	if c.sessions[n.ID] != s {
		c.mu.Unlock()
		return nil
	}
	s.ready = true
	s.info = hello.Info
	c.mu.Unlock()
	for {
		f, err := s.Read()
		if err != nil {
			return nil
		}
		c.mu.Lock()
		current := c.sessions[n.ID] == s
		c.mu.Unlock()
		if !current {
			return nil
		}
		switch f.Type {
		case "stream_error":
			c.mu.Lock()
			if st := c.streams[f.ID]; st != nil && st.sess == s {
				st.failure = echo.NewHTTPError(429, "节点数据连接已达上限")
				c.releaseLocked(st)
			}
			c.mu.Unlock()
		case "accepted", "result":
			t, err := c.Store.Task(f.ID)
			if err != nil || t.NodeID != n.ID {
				continue
			}
			if f.Type == "accepted" {
				if t.State == "queued" {
					t.State = "running"
					if err = c.Store.SaveTask(t); err != nil {
						s.Close()
						return nil
					}
				}
				continue
			}
			if f.Task == nil || !f.Task.Terminal() {
				s.Close()
				return nil
			}
			if f.Task.Response != nil && (len(f.Task.Response.Body) > MaxResult || f.Task.Response.Status < 100 || f.Task.Response.Status > 599) {
				s.Close()
				return nil
			}
			t.State = f.Task.State
			t.Response = f.Task.Response
			t.Error = f.Task.Error
			if err = c.Store.SaveTask(t); err != nil {
				s.Close()
				return nil
			}
			if err = s.Send(Frame{Type: "ack", ID: t.ID}); err != nil {
				return nil
			}
		default:
			s.Close()
			return nil
		}
	}
}
func (c *Controller) closeSessionLocked(s *session) {
	s.Close()
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
	c.closed = true
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
	if s == nil || !s.ready {
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
	if err := s.Send(Frame{Type: "open", ID: st.id, Stream: st.id, Ticket: st.ticket, Kind: kind, Request: req}); err != nil {
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
	if st == nil || st.sess.node.ID != n.ID || c.sessions[n.ID] != st.sess || st.conn != nil || st.reserved || !matches(ctx.Request().Header.Get("X-Poke-Ticket"), digest(st.ticket)) {
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
	if req.Method != "GET" {
		if strings.HasPrefix(path, "/api/projects") || strings.Contains(path, "/update") {
			req.Registries = c.credentials(node)
		}
		if strings.HasPrefix(path, "/api/images/pull") || strings.HasPrefix(path, "/api/images/check-update") {
			var payload struct {
				Image string `json:"image"`
			}
			if json.Unmarshal(body, &payload) == nil {
				domain, _, _ := images.ParseRepositoryInfo(payload.Image)
				for _, r := range c.credentials(node) {
					if images.NormalizeRegistry(r.Domain) == domain {
						req.Registries = append(req.Registries, r)
					}
				}
			}
		}
	}
	if req.Method != "GET" {
		c.mu.Lock()
		s := c.sessions[node]
		ready := s != nil && s.ready
		c.mu.Unlock()
		if !ready {
			return echo.NewHTTPError(503, "节点离线")
		}
		task := Task{ID: randomID(), NodeID: node, Method: req.Method, Path: req.Path, State: "queued", CreatedAt: time.Now().UTC()}
		if err = c.Store.SaveTask(task); err != nil {
			return echo.ErrInternalServerError
		}
		if err = s.Send(Frame{Type: "task", ID: task.ID, Request: req}); err != nil {
			if c.Store.UpdateTask(task.ID, func(t *Task) {
				if !t.Terminal() {
					t.State = "unknown"
					t.Error = "发送任务时连接断开，请确认节点执行状态"
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
	if err = s.Send(Frame{Type: "cancel", ID: t.ID}); err != nil {
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
