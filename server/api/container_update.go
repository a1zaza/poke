package api

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"

	"github.com/labstack/echo/v4"
	"poke/internal/cluster"
	"poke/pkg/logger"
)

func (h *Handler) UpdateContainer(c echo.Context) error {
	id := c.Param("id")

	// 1. Inspect container to find compose project
	container, err := h.Docker.ContainerInspect(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Container not found"})
	}

	// 2. Check labels
	projectName := container.Config.Labels["com.docker.compose.project"]
	workingDir := container.Config.Labels["com.docker.compose.project.working_dir"]
	serviceName := container.Config.Labels["com.docker.compose.service"]

	if projectName == "" || workingDir == "" || serviceName == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "This container is not managed by Docker Compose (missing labels)",
			"code":  "NOT_COMPOSE_MANAGED",
		})
	}

	// 3. Verify that the Compose working directory is an existing directory
	// contained by the managed project root and is not a symbolic link.
	if p := cluster.OperationProject(c.Request().Context()); p != nil {
		if p.Name != projectName || p.Service != serviceName {
			return c.JSON(409, map[string]string{"error": "容器所属服务已改变"})
		}
		workingDir, err = projectDirForName(p.Name)
		if err != nil {
			return c.JSON(400, map[string]string{"error": "项目名称无效"})
		}
	}
	workingDir, err = validateManagedProjectDir(workingDir)
	if err != nil {
		logger.Warn("UPDATE", "Rejected unmanaged project path: %s", workingDir)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Project path is not managed by Poke",
			"code":  "NOT_COMPOSE_MANAGED",
		})
	}

	// 4. Execute Update
	// docker compose pull <service>
	// docker compose up -d <service>

	dockerConfigDir, cleanup, authErr := h.ImageService.PrepareAuthConfigDir(c.Request().Context(), workingDir)
	if authErr != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "仓库凭据准备失败"})
	}
	if dockerConfigDir != "" {
		defer cleanup()
	}

	// Execution helper
	runCmd := func(name string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(c.Request().Context(), name, args...)
		cmd.Dir = workingDir
		if dockerConfigDir != "" {
			env := os.Environ()
			if cluster.OperationProject(c.Request().Context()) != nil {
				env = remoteEnvironment()
			}
			cmd.Env = append(env, "DOCKER_CONFIG="+dockerConfigDir)
		}
		return cmd.CombinedOutput()
	}

	// Pull
	out, err := runCmd("docker", "compose", "pull", serviceName)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("拉取失败: %s", string(out))})
	}

	// Up
	out, err = runCmd("docker", "compose", "up", "-d", serviceName)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("更新失败: %s", string(out))})
	}

	// 更新后刷新镜像检查缓存。
	_, _ = h.ImageService.CheckUpdate(c.Request().Context(), container.Config.Image, true)

	return c.JSON(http.StatusOK, map[string]string{"message": "Container updated successfully"})
}
