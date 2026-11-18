package cluster

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrUnauthorized = errors.New("节点凭据无效或已撤销")
var ErrNotFound = errors.New("记录不存在")
var safeID = regexp.MustCompile(`^[a-f0-9]{32}$`)

func randomID() string { return secret()[:32] }
func secret() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("安全随机源不可用: %w", err))
	}
	return hex.EncodeToString(b[:])
}
func digest(s string) string { v := sha256.Sum256([]byte(s)); return hex.EncodeToString(v[:]) }
func matches(s, hash string) bool {
	return s != "" && subtle.ConstantTimeCompare([]byte(digest(s)), []byte(hash)) == 1
}

// WriteJSON commits a complete private file; failed writes never publish partial JSON.
func WriteJSON(name string, v any) (err error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(name), ".poke-*.tmp")
	if err != nil {
		return err
	}
	defer func() { f.Close(); os.Remove(f.Name()) }()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), name); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(name))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
func readJSON(name string, v any) error {
	info, err := os.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s 必须是普通文件", name)
	}
	if err = os.Chmod(name, 0600); err != nil {
		return err
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s 损坏，请从备份恢复: %w", name, err)
	}
	return nil
}

const LeaseDuration = 60 * time.Second

type Node struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Registries        []string  `json:"registries"`
	Registered        bool      `json:"registered"`
	Revoked           bool      `json:"revoked"`
	CreatedAt         time.Time `json:"created_at"`
	CredentialHash    string    `json:"credential_hash,omitempty"`
	CredentialVersion uint64    `json:"credential_version"`
	BootID            string    `json:"boot_id,omitempty"`
	Epoch             uint64    `json:"epoch"`
	LeaseUntil        time.Time `json:"lease_until,omitempty"`
}

func (n Node) Public() Node {
	n.Registered = n.CredentialHash != ""
	n.Registries = append([]string{}, n.Registries...)
	n.CredentialHash = ""
	return n
}

type Task struct {
	ID           string    `json:"id"`
	NodeID       string    `json:"node_id"`
	Method       string    `json:"method"`
	Path         string    `json:"path"`
	State        string    `json:"state"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Response     *Response `json:"response,omitempty"`
	BootID       string    `json:"boot_id,omitempty"`
	Permit       string    `json:"permit,omitempty"`
	Request      *Request  `json:"request,omitempty"`
	PreparedHash string    `json:"prepared_hash,omitempty"`
	Resolved     bool      `json:"resolved,omitempty"`
	Error        string    `json:"error,omitempty"`
}

func (t Task) Terminal() bool {
	return t.State == "succeeded" || t.State == "failed" || t.State == "unknown" || t.State == "cancelled"
}

type nodeFile struct {
	Version int             `json:"version"`
	Nodes   map[string]Node `json:"nodes"`
}
type Store struct {
	mu    sync.RWMutex
	dir   string
	lock  *os.File
	nodes map[string]Node
	tasks map[string]Task
}

func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "tasks"), 0700); err != nil {
		return nil, err
	}
	// A JSON state directory belongs to exactly one running process.
	lock, err := os.OpenFile(filepath.Join(dir, "state.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, fmt.Errorf("数据目录已被其他实例使用: %w", err)
	}
	s := &Store{dir: dir, lock: lock, nodes: map[string]Node{}, tasks: map[string]Task{}}
	fail := func(err error) (*Store, error) { s.Close(); return nil, err }
	var state nodeFile
	err = readJSON(filepath.Join(dir, "nodes.json"), &state)
	if err == nil {
		if state.Version != 2 || state.Nodes == nil {
			return fail(errors.New("节点文件版本或结构无效"))
		}
		s.nodes = state.Nodes
	} else if !os.IsNotExist(err) {
		return fail(err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "tasks"))
	if err != nil {
		return fail(err)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var task Task
		if err = readJSON(filepath.Join(dir, "tasks", entry.Name()), &task); err != nil {
			return fail(err)
		}
		if !safeID.MatchString(task.ID) || entry.Name() != task.ID+".json" {
			return fail(errors.New("任务文件标识无效"))
		}
		task.Response = nil
		s.tasks[task.ID] = task
	}
	if err := s.validateProjects(); err != nil {
		return fail(err)
	}
	return s, nil
}
func (s *Store) Close() error {
	if s.lock != nil {
		syscall.Flock(int(s.lock.Fd()), syscall.LOCK_UN)
		err := s.lock.Close()
		s.lock = nil
		return err
	}
	return nil
}
func (s *Store) saveNodes(nodes map[string]Node) error {
	if err := WriteJSON(filepath.Join(s.dir, "nodes.json"), nodeFile{2, nodes}); err != nil {
		return err
	}
	s.nodes = nodes
	return nil
}
func (s *Store) copyNodes() map[string]Node {
	m := make(map[string]Node, len(s.nodes))
	for k, v := range s.nodes {
		m[k] = v
	}
	return m
}
func (s *Store) CreateNode(name string, registries []string) (Node, string, error) {
	if strings.TrimSpace(name) == "" || len(name) > 100 {
		return Node{}, "", errors.New("请输入不超过 100 字符的节点名称")
	}
	for i, r := range registries {
		registries[i] = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(r), "https://"), "http://"), "/")
	}
	token := secret()
	n := Node{ID: randomID(), Name: name, Registries: append([]string{}, registries...), CreatedAt: time.Now().UTC(), CredentialHash: digest(token), CredentialVersion: 1}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.copyNodes()
	m[n.ID] = n
	if err := s.saveNodes(m); err != nil {
		return Node{}, "", err
	}
	return n.Public(), token, nil
}
func (s *Store) Authenticate(id, credential string) (Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.nodes[id]
	if !ok || n.Revoked || !matches(credential, n.CredentialHash) {
		return Node{}, ErrUnauthorized
	}
	n.Registries = append([]string{}, n.Registries...)
	return n.Public(), nil
}
func (s *Store) Nodes() []Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	nodes := make([]Node, 0, len(s.nodes))
	for _, n := range s.nodes {
		v := n.Public()
		v.Registries = append([]string{}, v.Registries...)
		nodes = append(nodes, v)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].CreatedAt.Before(nodes[j].CreatedAt) })
	return nodes
}
func (s *Store) Revoke(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.nodes[id]
	if !ok {
		return ErrNotFound
	}
	n.Revoked = true
	n.CredentialHash = ""
	n.CredentialVersion++
	m := s.copyNodes()
	m[id] = n
	return s.saveNodes(m)
}
func (s *Store) saveTaskLocked(task Task) error {
	task.UpdatedAt = time.Now().UTC()
	if err := WriteJSON(filepath.Join(s.dir, "tasks", task.ID+".json"), task); err != nil {
		return err
	}
	task.Response = nil // Keep only metadata resident; full results are loaded on demand.
	s.tasks[task.ID] = task
	return nil
}
func (s *Store) SaveTask(task Task) error {
	if !safeID.MatchString(task.ID) {
		return errors.New("任务标识无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveTaskLocked(task)
}
func (s *Store) Task(id string) (Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.tasks[id]; !ok {
		return Task{}, ErrNotFound
	}
	var task Task
	if err := readJSON(filepath.Join(s.dir, "tasks", id+".json"), &task); err != nil {
		return Task{}, err
	}
	return task, nil
}
func (s *Store) UpdateTask(id string, update func(*Task)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[id]; !ok {
		return ErrNotFound
	}
	var task Task
	if err := readJSON(filepath.Join(s.dir, "tasks", id+".json"), &task); err != nil {
		return err
	}
	update(&task)
	return s.saveTaskLocked(task)
}
func (s *Store) allTasks() []Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ts := make([]Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		t.Response = nil
		t.Request = nil
		ts = append(ts, t)
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i].CreatedAt.After(ts[j].CreatedAt) })
	return ts
}
func (s *Store) RecoverTasks() error {
	for _, meta := range s.allTasks() {
		if meta.Terminal() {
			continue
		}
		if err := s.UpdateTask(meta.ID, func(t *Task) {
			if t.Permit == "" {
				t.State = "cancelled"
				t.Error = "Controller 重启，未授权的任务已取消"
			} else {
				t.State = "unknown"
				t.Error = "Controller 重启，执行结果待确认；不会自动重放"
			}
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) RotateToken(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.nodes[id]
	if !ok {
		return "", ErrNotFound
	}
	token := secret()
	n.Revoked = false
	n.CredentialHash = digest(token)
	n.CredentialVersion++
	n.Epoch++ // Fence existing control/data messages immediately on rotation.
	// Preserve the old lease: credential rotation cannot stop an in-flight Docker operation.
	m := s.copyNodes()
	m[id] = n
	if err := s.saveNodes(m); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Store) Tasks() []Task {
	tasks := s.allTasks()
	if len(tasks) > 200 {
		tasks = tasks[:200]
	}
	return tasks
}

func (s *Store) UpdateNode(id, name string, registries []string) error {
	if strings.TrimSpace(name) == "" || len(name) > 100 {
		return errors.New("节点名称无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.nodes[id]
	if !ok || n.Revoked {
		return ErrNotFound
	}
	n.Name = name
	n.Registries = append([]string{}, registries...)
	m := s.copyNodes()
	m[id] = n
	return s.saveNodes(m)
}
