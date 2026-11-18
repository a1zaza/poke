package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/moby/moby/client"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"poke/internal/cluster"
	"poke/pkg/compose"
	"poke/pkg/images"
)

// Do not expose the Agent's POKE_NODE_TOKEN or unrelated process secrets to
// Compose interpolation or subprocess environments.
func remoteEnvironment() []string {
	out := []string{}
	for _, key := range []string{"PATH", "HOME", "TMPDIR", "DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH", "SSL_CERT_FILE", "SSL_CERT_DIR"} {
		if value, ok := os.LookupEnv(key); ok {
			out = append(out, key+"="+value)
		}
	}
	return out
}

type limitedOutput struct{ bytes.Buffer }

func (w *limitedOutput) Write(b []byte) (int, error) {
	if w.Len()+len(b) > cluster.MaxBody {
		return 0, errors.New("Compose 输出超过限制")
	}
	return w.Buffer.Write(b)
}
func (e *Executor) Prepare(ctx context.Context, source *cluster.Request) (cluster.Preparation, error) {
	r := *source
	r.Registries = nil
	prep := cluster.Preparation{Request: &r, Domains: []string{}}
	path := strings.SplitN(r.Path, "?", 2)[0]
	if path == "/api/images/pull" || path == "/api/images/check-update" {
		var payload struct {
			Image string `json:"image"`
		}
		if json.Unmarshal(r.Body, &payload) != nil || payload.Image == "" {
			return prep, errors.New("请指定镜像")
		}
		domain, _, _ := images.ParseRepositoryInfo(payload.Image)
		prep.Domains = []string{domain}
		return prep, nil
	}
	if r.Project == nil {
		if strings.HasSuffix(path, "/update") {
			return prep, errors.New("更新操作缺少 Controller 项目源配置")
		}
		return prep, nil
	}
	p := *r.Project
	r.Project = &p
	if !compose.Name.MatchString(p.Name) {
		return prep, errors.New("项目名称无效")
	}
	if err := compose.Validate(p.Content); err != nil {
		return prep, err
	}
	dir, err := prepareRemoteDirectory(e.remoteRoot, p.Name)
	if err != nil {
		return prep, err
	}
	service := ""
	if strings.HasPrefix(path, "/api/containers/") && strings.HasSuffix(path, "/update") {
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/api/containers/"), "/update")
		inspection, err := e.Docker.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
		container := inspection.Container
		if err != nil {
			return prep, err
		}
		if container.Config == nil || container.Config.Labels["com.docker.compose.project"] != p.Name {
			return prep, errors.New("容器所属项目已改变")
		}
		service = container.Config.Labels["com.docker.compose.service"]
		if service == "" {
			return prep, errors.New("容器缺少 Compose 服务标识")
		}
		p.Service = service
	}
	cmd := exec.CommandContext(ctx, "docker", "compose", "--env-file", "/dev/null", "--project-directory", dir, "-p", p.Name, "-f", "-", "config", "--format", "json")
	cmd.Stdin = strings.NewReader(p.Content)
	cmd.Env = remoteEnvironment()
	cmd.Dir = dir
	output := &limitedOutput{}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		return prep, errors.New("Compose 配置解析失败，请检查语法及外部依赖")
	}
	var model struct {
		Services map[string]struct {
			Image string `json:"image"`
		} `json:"services"`
	}
	if err := json.Unmarshal(output.Bytes(), &model); err != nil {
		return prep, errors.New("Compose 配置输出无效")
	}
	p.Content = strings.ReplaceAll(output.String(), "$", "$$")
	needsAuth := !strings.HasSuffix(path, "/action")
	if strings.HasSuffix(path, "/action") {
		var body ProjectActionRequest
		_ = json.Unmarshal(r.Body, &body)
		needsAuth = body.Action == "up" || body.Action == "pull"
	}
	domains := map[string]bool{}
	if needsAuth {
		for name, s := range model.Services {
			if service != "" && name != service {
				continue
			}
			if s.Image != "" {
				domain, _, _ := images.ParseRepositoryInfo(s.Image)
				domains[domain] = true
			}
		}
	}
	for domain := range domains {
		prep.Domains = append(prep.Domains, domain)
	}
	if service != "" {
		if _, ok := model.Services[service]; !ok {
			return prep, errors.New("Controller 配置中不存在该容器服务")
		}
	}
	return prep, nil
}
func prepareRemoteDirectory(root, name string) (string, error) {
	err := validateProjectName(name)
	dir := filepath.Join(root, name)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("项目根目录无效")
	}
	if err = os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return "", err
	}
	return validateManagedPath(root, dir)
}
func (e *Executor) remoteProject(w http.ResponseWriter, r *http.Request, p *cluster.ProjectSource) {
	fail := func(status int, message string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
	}
	dir, err := prepareRemoteDirectory(e.remoteRoot, p.Name)
	if err != nil {
		fail(400, "项目工作目录无效")
		return
	}
	// Rendered material is a projection of the task snapshot, never a source of truth.
	if err = writeProjectCompose(e.remoteRoot, dir, []byte(p.Content)); err != nil {
		fail(400, "项目运行文件写入失败")
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/containers/") {
		e.Router.ServeHTTP(w, r)
		return
	}
	args := []string{"compose", "-p", p.Name}
	action := "up"
	if strings.HasSuffix(r.URL.Path, "/action") {
		var body ProjectActionRequest
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			fail(400, "项目操作无效")
			return
		}
		action = body.Action
	}
	switch action {
	case "up":
		args = append(args, "up", "-d", "--remove-orphans")
	case "start", "stop", "restart", "down", "pull":
		args = append(args, action)
	case "remove":
		args = append(args, "down")
	default:
		fail(400, "不支持的项目操作")
		return
	}
	authDir, cleanup, err := e.ImageService.PrepareAuthConfigDir(r.Context(), dir)
	if err != nil {
		fail(500, "仓库凭据准备失败")
		return
	}
	defer cleanup()
	cmd := exec.CommandContext(r.Context(), "docker", args...)
	cmd.Dir = dir
	cmd.Env = append(remoteEnvironment(), "DOCKER_CONFIG="+authDir)
	output := &limitedOutput{}
	cmd.Stdout = output
	cmd.Stderr = output
	if err = cmd.Run(); err != nil {
		fail(500, fmt.Sprintf("项目操作失败: %s", output.String()))
		return
	}
	// Removing an application never removes host bind-mount business directories.
	if action == "remove" {
		root, err := openProjectRoot(e.remoteRoot, dir)
		if err != nil {
			fail(500, "项目运行文件清理失败")
			return
		}
		defer root.Close()
		if err = root.Remove(composeFilename); err != nil && !os.IsNotExist(err) {
			fail(500, "项目运行文件清理失败")
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "项目操作成功"})
}
