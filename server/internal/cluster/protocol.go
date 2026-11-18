package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"poke/pkg/config"
)

var ErrProtocolVersion = errors.New("节点协议版本不兼容")

const Version = 2
const MaxBody = 1024 * 1024
const MaxResult = 256 * 1024
const MaxFrame = 4 * 1024 * 1024

type Request struct {
	Method     string                  `json:"method"`
	Path       string                  `json:"path"`
	Body       []byte                  `json:"body,omitempty"`
	Registries []config.RegistryConfig `json:"registries,omitempty"`
	Project    *ProjectSource          `json:"project,omitempty"`
}
type ProjectSource struct {
	Service          string `json:"service,omitempty"`
	ID               string `json:"id"`
	NodeID           string `json:"node_id"`
	Name             string `json:"name"`
	Revision         int    `json:"revision"`
	DeployedRevision int    `json:"deployed_revision"`
	Content          string `json:"content"`
	Deleted          bool   `json:"deleted,omitempty"`
	PendingDelete    bool   `json:"pending_delete,omitempty"`
}

type Response struct {
	Status      int    `json:"status"`
	Body        []byte `json:"body,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}
type AgentInfo struct {
	Hostname string `json:"hostname"`
	Platform string `json:"platform"`
}
type Frame struct {
	Pending      []string   `json:"pending,omitempty"`
	Session      string     `json:"session,omitempty"`
	BootID       string     `json:"boot_id,omitempty"`
	Epoch        uint64     `json:"epoch,omitempty"`
	Permit       string     `json:"permit,omitempty"`
	Hash         string     `json:"hash,omitempty"`
	Domains      []string   `json:"domains,omitempty"`
	LeaseSeconds int        `json:"lease_seconds,omitempty"`
	Info         *AgentInfo `json:"info,omitempty"`
	Version      int        `json:"version"`
	Type         string     `json:"type"`
	ID           string     `json:"id,omitempty"`
	Stream       string     `json:"stream,omitempty"`
	Ticket       string     `json:"ticket,omitempty"`
	Kind         string     `json:"kind,omitempty"`
	Request      *Request   `json:"request,omitempty"`
	Response     *Response  `json:"response,omitempty"`
	Task         *Task      `json:"task,omitempty"`
}

var resource = regexp.MustCompile(`^/api/(?:containers(?:/[^/]+(?:/(?:start|stop|restart|update|logs))?)?|images(?:/(?:pull|check-update|prune|[^/]+))?|networks(?:/[^/]+)?|volumes(?:/[^/]+)?|projects(?:/[^/]+(?:/action)?)?|system/(?:info|events))$`)
var terminal = regexp.MustCompile(`^/ws/containers/[^/]+/exec$`)

// Update checks use POST for their body, but do not mutate Docker resources.
// Keep them out of the persistent mutation task queue.
func isReadOnlyRequest(r *Request) bool {
	if r == nil {
		return false
	}
	if r.Method == http.MethodGet {
		return true
	}
	u, err := url.ParseRequestURI(r.Path)
	return err == nil && r.Method == http.MethodPost && u.Path == "/api/images/check-update"
}

func ValidRequest(r *Request, exec bool) bool {
	if r == nil || len(r.Body) > MaxBody || len(r.Path) > 8192 {
		return false
	}
	u, err := url.ParseRequestURI(r.Path)
	if err != nil || u.IsAbs() || u.Host != "" || strings.Contains(u.Path, "\\") {
		return false
	}
	for _, ch := range u.Path {
		if ch < 32 || ch == 127 {
			return false
		}
	}
	for _, part := range strings.Split(u.Path, "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	if exec {
		return r.Method == http.MethodGet && terminal.MatchString(u.Path)
	}
	switch r.Method {
	case "GET", "POST", "PUT", "DELETE":
	default:
		return false
	}
	return resource.MatchString(u.Path)
}

// Peer serializes all data writes and sends bounded heartbeats independently of application dispatch.
type Peer struct {
	Conn *websocket.Conn
	mu   sync.Mutex
	done chan struct{}
	once sync.Once
}

func NewPeer(c *websocket.Conn) *Peer {
	p := &Peer{Conn: c, done: make(chan struct{})}
	c.SetReadLimit(MaxFrame)
	c.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.SetPongHandler(func(string) error { return c.SetReadDeadline(time.Now().Add(60 * time.Second)) })
	go func() {
		tick := time.NewTicker(20 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-p.done:
				return
			case <-tick.C:
				if err := c.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					p.Close()
					return
				}
			}
		}
	}()
	return p
}
func (p *Peer) Send(v Frame) error {
	v.Version = Version
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return p.Conn.WriteJSON(v)
}
func (p *Peer) Close()                { p.once.Do(func() { close(p.done); p.Conn.Close() }) }
func (p *Peer) Done() <-chan struct{} { return p.done }
func (p *Peer) Read() (Frame, error) {
	var f Frame
	err := p.Conn.ReadJSON(&f)
	if err == nil && f.Version != Version {
		err = ErrProtocolVersion
	}
	return f, err
}

// Preparation resolves a fixed operation without Docker mutations or registry secrets.
type Preparation struct {
	Request *Request
	Domains []string
	Cleanup func()
}
type Executor interface {
	Prepare(context.Context, *Request) (Preparation, error)

	ServeHTTP(http.ResponseWriter, *http.Request)
	Exec(context.Context, *websocket.Conn, string) error
}

func requestHash(r *Request) string {
	data, _ := json.Marshal(r)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
