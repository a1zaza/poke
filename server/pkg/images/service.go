package images

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"

	"poke/pkg/config"
)

type UpdateResult struct {
	HasUpdate    bool
	RemoteDigest string
	LastChecked  time.Time
	Error        string
}

type Service struct {
	AuthRoot string
	docker   *client.Client
	cache    sync.Map // map[string]*UpdateResult
}

func NewService(docker *client.Client) *Service {
	return &Service{
		docker: docker,
	}
}

type credentialsKey struct{}

// WithCredentials isolates one node operation from local configuration and other concurrent tasks.
func WithCredentials(ctx context.Context, registries []config.RegistryConfig) context.Context {
	return context.WithValue(ctx, credentialsKey{}, append([]config.RegistryConfig{}, registries...))
}
func credentials(ctx context.Context) []config.RegistryConfig {
	if regs, ok := ctx.Value(credentialsKey{}).([]config.RegistryConfig); ok {
		return regs
	}
	return config.GetConfig().System.Registries
}
func (s *Service) ResolveAuthConfig(ctx context.Context, domain string) string {
	auth := s.GetRegistryAuth(ctx, domain)
	if auth == nil {
		return ""
	}
	encoded, _ := encodeAuth(*auth)
	return encoded
}
func (s *Service) GetRegistryAuth(ctx context.Context, domain string) *registry.AuthConfig {
	domain = NormalizeRegistry(domain)
	for _, r := range credentials(ctx) {
		if NormalizeRegistry(r.Domain) == domain {
			return &registry.AuthConfig{ServerAddress: r.Domain, Username: r.Username, Password: r.Password}
		}
	}
	return nil
}
func (s *Service) PrepareAuthConfigDir(ctx context.Context, projectDir string) (string, func(), error) {
	tmpDir, err := os.MkdirTemp(s.AuthRoot, "poke-docker-config-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { os.RemoveAll(tmpDir) }
	auths := make(map[string]map[string]string)
	command := exec.CommandContext(ctx, "docker", "compose", "config", "--images")
	command.Dir = projectDir
	output, err := command.Output()
	if err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("解析项目镜像失败: %w", err)
	}
	needed := make(map[string]bool)
	for _, ref := range strings.Fields(string(output)) {
		domain, _, _ := ParseRepositoryInfo(ref)
		needed[domain] = true
	}
	for _, r := range credentials(ctx) {
		if !needed[NormalizeRegistry(r.Domain)] {
			continue
		}
		auths[NormalizeRegistry(r.Domain)] = map[string]string{"auth": base64.StdEncoding.EncodeToString([]byte(r.Username + ":" + r.Password))}
	}
	data, err := json.Marshal(map[string]any{"auths": auths})
	if err == nil {
		err = os.WriteFile(filepath.Join(tmpDir, "config.json"), data, 0600)
	}
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	return tmpDir, cleanup, nil
}

func NormalizeRegistry(r string) string {
	r = strings.ToLower(strings.TrimSpace(r))
	r = strings.TrimPrefix(r, "https://")
	r = strings.TrimPrefix(r, "http://")
	r = strings.TrimSuffix(r, "/")
	if r == "index.docker.io/v1" {
		return "docker.io"
	}
	return r
}

func encodeAuth(auth registry.AuthConfig) (string, error) {
	jsonBytes, err := json.Marshal(auth)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(jsonBytes), nil
}

// ParseRepositoryInfo splits image name into domain and path
// e.g. "nginx:latest" -> "docker.io", "library/nginx", "latest"
// "my.reg/foo/bar:tag" -> "my.reg", "foo/bar", "tag"
func ParseRepositoryInfo(ref string) (string, string, string) {
	domain := "docker.io"
	path := ""
	tag := "latest"

	// split tag
	if name, _, ok := strings.Cut(ref, "@"); ok {
		ref = name
		// Digest references do not include a tag.
	} else if strings.LastIndex(ref, ":") > strings.LastIndex(ref, "/") {
		// has tag
		idx := strings.LastIndex(ref, ":")
		tag = ref[idx+1:]
		ref = ref[:idx]
	}

	parts := strings.Split(ref, "/")
	first := parts[0]
	if first == "docker.io" {
		path = strings.Join(parts[1:], "/")
	} else if strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost" {
		domain = first
		path = strings.Join(parts[1:], "/")
	} else {
		// default docker hub
		// if only "nginx", path is "library/nginx"
		// if "foo/bar", path is "foo/bar"
		if len(parts) == 1 {
			path = "library/" + parts[0]
		} else {
			path = strings.Join(parts, "/")
		}
	}

	return domain, path, tag
}

func (s *Service) CheckUpdate(ctx context.Context, imageName string, force bool) (*UpdateResult, error) {
	if !force {
		if val, ok := s.cache.Load(imageName); ok {
			res := val.(*UpdateResult)
			if time.Since(res.LastChecked) < 1*time.Hour {
				return res, nil
			}
		}
	}

	ref := imageName
	if !strings.Contains(ref, ":") && !strings.Contains(ref, "@") {
		ref = ref + ":latest"
	}

	domain, _, _ := ParseRepositoryInfo(ref)
	authStr := s.ResolveAuthConfig(ctx, domain)

	// DistributionInspect
	dist, err := s.docker.DistributionInspect(ctx, ref, authStr)
	if err != nil {
		res := &UpdateResult{
			LastChecked: time.Now(),
			Error:       err.Error(),
		}
		if strings.Contains(err.Error(), "unauthorized") || strings.Contains(err.Error(), "denied") {
			res.Error = fmt.Sprintf("Auth failed for %s", domain)
		}
		s.cache.Store(imageName, res)
		return res, nil
	}

	remoteDigest := dist.Descriptor.Digest.String()

	// Inspect local
	localImg, _, err := s.docker.ImageInspectWithRaw(ctx, ref)
	if err != nil {
		res := &UpdateResult{
			LastChecked: time.Now(),
			Error:       "Local image not found",
		}
		s.cache.Store(imageName, res)
		return res, nil
	}

	hasUpdate := true
	// Logic: if any of the local RepoDigests matches remote digest -> no update.
	for _, rd := range localImg.RepoDigests {
		if strings.Contains(rd, remoteDigest) {
			hasUpdate = false
			break
		}
	}
	// Also if the current image ID matches exactly what the digest resolves to?
	// Usually digest is enough.

	res := &UpdateResult{
		HasUpdate:    hasUpdate,
		RemoteDigest: remoteDigest,
		LastChecked:  time.Now(),
	}
	s.cache.Store(imageName, res)

	return res, nil
}

func (s *Service) RefreshAll(ctx context.Context) {
	containers, err := s.docker.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		fmt.Printf("Error listing containers: %v\n", err)
		return
	}
	for _, c := range containers {
		_, _ = s.CheckUpdate(ctx, c.Image, false)
	}
}
