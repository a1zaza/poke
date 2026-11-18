package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"poke/pkg/compose"
)

type projectHistory struct {
	Versions []ProjectSource `json:"versions"`
}

func (s *Store) projectFile(node, name string) (string, error) {
	if !safeID.MatchString(node) || !compose.Name.MatchString(name) {
		return "", errors.New("项目标识无效")
	}
	return filepath.Join(s.dir, "projects", node+"-"+digest(name)+".json"), nil
}
func (s *Store) Project(node, name string) (ProjectSource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.projectLocked(node, name)
}
func (s *Store) projectLocked(node, name string) (ProjectSource, error) {
	file, err := s.projectFile(node, name)
	if err != nil {
		return ProjectSource{}, err
	}
	var history projectHistory
	if err = readJSON(file, &history); err != nil {
		if os.IsNotExist(err) {
			return ProjectSource{}, ErrNotFound
		}
		return ProjectSource{}, err
	}
	if len(history.Versions) == 0 {
		return ProjectSource{}, errors.New("项目版本文件为空")
	}
	source := history.Versions[len(history.Versions)-1]
	if source.NodeID != node || source.Name != name || source.Deleted {
		return ProjectSource{}, ErrNotFound
	}
	return source, nil
}
func (s *Store) SaveProject(node, name, content string, create bool) (ProjectSource, error) {
	if err := compose.Validate(content); err != nil {
		return ProjectSource{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.projectFile(node, name)
	if err != nil {
		return ProjectSource{}, err
	}
	if err = os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return ProjectSource{}, err
	}
	var history projectHistory
	if err = readJSON(file, &history); err != nil && !os.IsNotExist(err) {
		return ProjectSource{}, err
	}
	source := ProjectSource{ID: randomID(), NodeID: node, Name: name}
	if len(history.Versions) > 0 {
		source = history.Versions[len(history.Versions)-1]
		if create && !source.Deleted {
			return ProjectSource{}, errors.New("项目已存在")
		}
		if create && source.Deleted {
			source.ID = randomID()
			source.DeployedRevision = 0
		}
		if !create && source.Deleted {
			return ProjectSource{}, ErrNotFound
		}
	}
	if len(history.Versions) == 0 && !create {
		return ProjectSource{}, ErrNotFound
	}
	source.Content = content
	source.Revision++
	source.Deleted = false
	source.PendingDelete = false
	history.Versions = append(history.Versions, source)
	if err = WriteJSON(file, history); err != nil {
		return ProjectSource{}, err
	}
	return source, nil
}
func (s *Store) Projects(node string) ([]ProjectSource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(filepath.Join(s.dir, "projects"))
	if os.IsNotExist(err) {
		return []ProjectSource{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []ProjectSource{}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), node+"-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var h projectHistory
		if err := readJSON(filepath.Join(s.dir, "projects", entry.Name()), &h); err != nil {
			return nil, err
		}
		if len(h.Versions) == 0 {
			return nil, errors.New("项目历史损坏")
		}
		p := h.Versions[len(h.Versions)-1]
		if !p.Deleted {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func (c *Controller) finishProject(t Task) error {
	if t.Request == nil || t.Request.Project == nil || t.State != "succeeded" {
		return nil
	}
	p := t.Request.Project
	if !p.PendingDelete {
		path := strings.SplitN(t.Request.Path, "?", 2)[0]
		deployed := path == "/api/projects" && t.Request.Method == "POST" || strings.HasPrefix(path, "/api/projects/") && t.Request.Method == "PUT"
		if strings.HasSuffix(path, "/action") {
			var body struct {
				Action string `json:"action"`
			}
			_ = json.Unmarshal(t.Request.Body, &body)
			deployed = body.Action == "up"
		}
		if !deployed {
			return nil
		}
	}
	c.Store.mu.Lock()
	defer c.Store.mu.Unlock()
	file, err := c.Store.projectFile(p.NodeID, p.Name)
	if err != nil {
		return err
	}
	var h projectHistory
	if err = readJSON(file, &h); err != nil {
		return err
	}
	if len(h.Versions) == 0 {
		return errors.New("项目历史为空")
	}
	last := &h.Versions[len(h.Versions)-1]
	if last.ID != p.ID {
		return errors.New("项目身份发生变化")
	}
	if p.PendingDelete && last.Revision == p.Revision {
		last.Deleted = true
		last.PendingDelete = false
	} else {
		last.DeployedRevision = p.Revision
	}
	return WriteJSON(file, h)
}
func (c *Controller) query(ctx context.Context, node string, req *Request) (Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	st, conn, err := c.open(ctx, node, "http", req)
	if err != nil {
		return Response{}, err
	}
	defer c.release(st)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	var result Response
	if err := conn.ReadJSON(&result); err != nil {
		return result, err
	}
	for {
		kind, data, err := conn.ReadMessage()
		if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
			break
		}
		if err != nil {
			return result, err
		}
		if kind != websocket.BinaryMessage || len(result.Body)+len(data) > MaxBody {
			return result, errors.New("节点响应超过限制")
		}
		result.Body = append(result.Body, data...)
	}
	return result, nil
}

// Returns handled=true for local source reads or preparation errors. Mutations
// retain the public API but carry an immutable Controller-owned source snapshot.
func (c *Controller) projectRequest(ctx echo.Context, node string, req *Request) (bool, error) {
	path := strings.SplitN(req.Path, "?", 2)[0]
	if strings.HasSuffix(path, "/update") && strings.HasPrefix(path, "/api/containers/") {
		containerPath := strings.TrimSuffix(path, "/update")
		result, err := c.query(ctx.Request().Context(), node, &Request{Method: "GET", Path: containerPath})
		if err != nil {
			return true, err
		}
		var data struct {
			Config struct {
				Labels map[string]string `json:"Labels"`
			} `json:"Config"`
		}
		if result.Status != 200 || json.Unmarshal(result.Body, &data) != nil {
			return true, echo.NewHTTPError(400, "无法读取容器所属项目")
		}
		source, err := c.Store.Project(node, data.Config.Labels["com.docker.compose.project"])
		if err != nil {
			return true, echo.NewHTTPError(400, "请先将该 Compose 项目导入 Controller")
		}
		if source.DeployedRevision > 0 && source.DeployedRevision != source.Revision {
			source, err = c.Store.ProjectRevision(node, source.Name, source.DeployedRevision)
			if err != nil {
				return true, echo.NewHTTPError(500, "已部署的项目版本不可用")
			}
		}
		req.Project = &source
		return false, nil
	}
	if path == "/api/containers" && req.Method == "GET" {
		projects, err := c.Store.Projects(node)
		if err != nil {
			return true, echo.NewHTTPError(500, "项目配置读取失败")
		}
		response, err := c.query(ctx.Request().Context(), node, req)
		if err != nil {
			return true, err
		}
		if response.Status != 200 {
			return true, ctx.Blob(response.Status, "application/json", response.Body)
		}
		var rows []map[string]any
		if json.Unmarshal(response.Body, &rows) != nil {
			return true, echo.NewHTTPError(502, "容器响应无效")
		}
		managed := map[string]bool{}
		for _, p := range projects {
			managed[p.Name] = true
		}
		for _, row := range rows {
			labels, _ := row["Labels"].(map[string]any)
			name, _ := labels["com.docker.compose.project"].(string)
			row["isControlled"] = managed[name]
		}
		return true, ctx.JSON(200, rows)
	}
	if path != "/api/projects" && !strings.HasPrefix(path, "/api/projects/") {
		return false, nil
	}
	parts := strings.Split(strings.TrimPrefix(path, "/api/projects"), "/")
	name := ""
	if len(parts) > 1 {
		name, _ = url.PathUnescape(parts[1])
	}
	if req.Method == "GET" {
		sources, err := c.Store.Projects(node)
		if err != nil {
			return true, echo.NewHTTPError(500, "项目源配置读取失败")
		}
		result, queryErr := c.query(ctx.Request().Context(), node, req)
		merge := func(actual map[string]any, p ProjectSource) map[string]any {
			available := queryErr == nil && (result.Status == 200 || result.Status == 404)
			if actual == nil {
				actual = map[string]any{"name": p.Name, "status": "unknown", "runningCount": 0, "totalCount": 0, "containers": []any{}, "workingDir": "/poke/projects/" + p.Name}
				if available {
					actual["status"] = "stopped"
				}
			}
			actual["isControlled"] = true
			actual["composeContent"] = p.Content
			actual["projectId"] = p.ID
			actual["revision"] = p.Revision
			actual["deployedRevision"] = p.DeployedRevision
			actual["actualStateAvailable"] = available
			return actual
		}
		if name != "" {
			var actual map[string]any
			if queryErr == nil && result.Status == 200 {
				_ = json.Unmarshal(result.Body, &actual)
			}
			for _, p := range sources {
				if p.Name == name {
					return true, ctx.JSON(200, merge(actual, p))
				}
			}
			if actual != nil {
				actual["isControlled"] = false
				delete(actual, "composeContent")
				return true, ctx.JSON(200, actual)
			}
			return true, echo.ErrNotFound
		}
		actual := []map[string]any{}
		if queryErr == nil && result.Status == 200 {
			if err := json.Unmarshal(result.Body, &actual); err != nil {
				return true, echo.NewHTTPError(502, "节点项目列表无效")
			}
		}
		byName := map[string]int{}
		for i, p := range actual {
			p["isControlled"] = false
			delete(p, "composeContent")
			if n, ok := p["name"].(string); ok {
				byName[n] = i
			}
		}
		for _, p := range sources {
			if i, ok := byName[p.Name]; ok {
				actual[i] = merge(actual[i], p)
			} else {
				actual = append(actual, merge(nil, p))
			}
		}
		if queryErr != nil && len(sources) == 0 {
			return true, queryErr
		}
		return true, ctx.JSON(200, actual)
	}
	// The mutation dispatcher serializes project revisions and task submission.
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Store.NodeBlocked(node) {
		return true, echo.NewHTTPError(409, "请先对账结果待确认的任务")
	}
	var body struct {
		Name    string `json:"name"`
		Content string `json:"content"`
		Action  string `json:"action"`
	}
	if json.Unmarshal(req.Body, &body) != nil {
		return true, echo.ErrBadRequest
	}
	var source ProjectSource
	var err error
	if path == "/api/projects" && req.Method == "POST" {
		source, err = c.Store.SaveProject(node, body.Name, body.Content, true)
	} else if req.Method == "PUT" {
		source, err = c.Store.SaveProject(node, name, body.Content, false)
	} else {
		source, err = c.Store.Project(node, name)
		if body.Action == "remove" {
			source.PendingDelete = true
		}
	}
	if err != nil {
		return true, echo.NewHTTPError(400, err.Error())
	}
	req.Project = &source
	return false, nil
}

func (c *Controller) importProject(ctx echo.Context) error {
	var body struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if json.NewDecoder(http.MaxBytesReader(ctx.Response(), ctx.Request().Body, MaxBody)).Decode(&body) != nil {
		return echo.ErrBadRequest
	}
	node := ctx.Param("node")
	found := false
	for _, n := range c.Store.Nodes() {
		if n.ID == node && !n.Revoked {
			found = true
		}
	}
	if !found {
		return echo.ErrNotFound
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	source, err := c.Store.SaveProject(node, body.Name, body.Content, true)
	if err != nil {
		return echo.NewHTTPError(400, err.Error())
	}
	return ctx.JSON(201, source)
}

func (s *Store) validateProjects() error {
	entries, err := os.ReadDir(filepath.Join(s.dir, "projects"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var history projectHistory
		if err := readJSON(filepath.Join(s.dir, "projects", entry.Name()), &history); err != nil {
			return err
		}
		if len(history.Versions) == 0 {
			return errors.New("项目历史为空")
		}
		previous := 0
		for _, p := range history.Versions {
			file, err := s.projectFile(p.NodeID, p.Name)
			if err != nil || !safeID.MatchString(p.ID) || filepath.Base(file) != entry.Name() || p.Revision <= previous {
				return errors.New("项目历史结构无效")
			}
			previous = p.Revision
		}
	}
	return nil
}

func (s *Store) ProjectRevision(node, name string, revision int) (ProjectSource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	file, err := s.projectFile(node, name)
	if err != nil {
		return ProjectSource{}, err
	}
	var history projectHistory
	if err = readJSON(file, &history); err != nil {
		return ProjectSource{}, err
	}
	if len(history.Versions) == 0 {
		return ProjectSource{}, ErrNotFound
	}
	current := history.Versions[len(history.Versions)-1]
	for _, p := range history.Versions {
		if p.Revision == revision && p.ID == current.ID {
			p.DeployedRevision = revision
			return p, nil
		}
	}
	return ProjectSource{}, ErrNotFound
}

func (c *Controller) saveProjectSource(ctx echo.Context) error {
	var body struct {
		Content string `json:"content"`
	}
	if json.NewDecoder(http.MaxBytesReader(ctx.Response(), ctx.Request().Body, MaxBody)).Decode(&body) != nil {
		return echo.ErrBadRequest
	}
	node := ctx.Param("node")
	found := false
	for _, n := range c.Store.Nodes() {
		if n.ID == node && !n.Revoked {
			found = true
		}
	}
	if !found {
		return echo.ErrNotFound
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	source, err := c.Store.SaveProject(node, ctx.Param("name"), body.Content, false)
	if err != nil {
		return echo.NewHTTPError(400, err.Error())
	}
	return ctx.JSON(200, source)
}
