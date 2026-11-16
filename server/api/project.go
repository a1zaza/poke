package api

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/labstack/echo/v4"
)

// Project Types
type ProjectContainer struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	State   string `json:"state"`
	Status  string `json:"status"`
	Service string `json:"service"`
	Image   string `json:"image"`
}

type Project struct {
	Name           string             `json:"name"`
	Status         string             `json:"status"`
	RunningCount   int                `json:"runningCount"`
	TotalCount     int                `json:"totalCount"`
	WorkingDir     string             `json:"workingDir"`
	IsControlled   bool               `json:"isControlled"`
	Containers     []ProjectContainer `json:"containers"`
	ComposeContent string             `json:"composeContent,omitempty"`
}

type ProjectActionRequest struct {
	Action string `json:"action"`
}

type CreateProjectRequest struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type UpdateProjectRequest struct {
	Content string `json:"content"`
}

// Helpers

func (h *Handler) ListProjects(c echo.Context) error {
	containers, err := h.Docker.ContainerList(c.Request().Context(), types.ContainerListOptions{All: true})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	projectsMap := make(map[string]*Project)

	// Phase 1: Scan containers
	for _, container := range containers {
		projectName := container.Labels["com.docker.compose.project"]
		if projectName == "" {
			continue
		}

		if _, exists := projectsMap[projectName]; !exists {
			workDir := container.Labels["com.docker.compose.project.working_dir"]
			projectsMap[projectName] = &Project{
				Name:         projectName,
				WorkingDir:   workDir,
				IsControlled: isManagedProjectDir(workDir),
				Containers:   []ProjectContainer{},
			}
		}

		serviceName := container.Labels["com.docker.compose.service"]
		name := ""
		if len(container.Names) > 0 {
			name = strings.TrimPrefix(container.Names[0], "/")
		}

		projectsMap[projectName].Containers = append(projectsMap[projectName].Containers, ProjectContainer{
			ID:      container.ID,
			Name:    name,
			State:   container.State,
			Status:  container.Status,
			Service: serviceName,
			Image:   container.Image,
		})
	}

	// Phase 2: Scan /poke/projects directory for missing projects
	entries, err := os.ReadDir(managedProjectsRoot)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() && validateProjectName(entry.Name()) == nil {
				projectName := entry.Name()
				if _, exists := projectsMap[projectName]; !exists {
					projectDir := filepath.Join(managedProjectsRoot, projectName)
					if _, err := validateManagedProjectDir(projectDir); err != nil {
						continue
					}
					// Check if docker-compose.yml exists
					composePath := filepath.Join(projectDir, "docker-compose.yml")
					if _, err := os.Stat(composePath); err == nil {
						projectsMap[projectName] = &Project{
							Name:         projectName,
							Status:       "stopped",
							WorkingDir:   projectDir,
							IsControlled: true,
							Containers:   []ProjectContainer{},
						}
					}
				}
			}
		}
	}

	projects := make([]*Project, 0, len(projectsMap))
	for _, p := range projectsMap {
		runningCount := 0
		for _, c := range p.Containers {
			if c.State == "running" {
				runningCount++
			}
		}
		p.RunningCount = runningCount
		p.TotalCount = len(p.Containers)

		// Only override status if we have containers.
		// If we don't have containers (from Phase 2), status is already "stopped".
		if p.TotalCount > 0 {
			if runningCount == p.TotalCount {
				p.Status = "running"
			} else if runningCount > 0 {
				p.Status = "partial"
			} else {
				p.Status = "stopped"
			}
		}

		projects = append(projects, p)
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].Name < projects[j].Name
	})

	return c.JSON(http.StatusOK, projects)
}

func (h *Handler) GetProject(c echo.Context) error {
	name := c.Param("name")
	if err := validateProjectName(name); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid project name"})
	}

	containers, err := h.Docker.ContainerList(c.Request().Context(), types.ContainerListOptions{All: true})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	var project *Project

	for _, container := range containers {
		pName := container.Labels["com.docker.compose.project"]
		if pName != name {
			continue
		}

		if project == nil {
			workDir := container.Labels["com.docker.compose.project.working_dir"]
			project = &Project{
				Name:         pName,
				WorkingDir:   workDir,
				IsControlled: isManagedProjectDir(workDir),
				Containers:   []ProjectContainer{},
			}
		}

		serviceName := container.Labels["com.docker.compose.service"]
		cName := ""
		if len(container.Names) > 0 {
			cName = strings.TrimPrefix(container.Names[0], "/")
		}

		project.Containers = append(project.Containers, ProjectContainer{
			ID:      container.ID,
			Name:    cName,
			State:   container.State,
			Status:  container.Status,
			Service: serviceName,
			Image:   container.Image,
		})
	}

	if project == nil {
		// Try to find in /poke/projects
		projectDir, _ := projectDirForName(name)
		if _, err := validateManagedProjectDir(projectDir); err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Project not found"})
		}
		if _, err := os.Stat(filepath.Join(projectDir, "docker-compose.yml")); err == nil {
			project = &Project{
				Name:         name,
				Status:       "stopped",
				WorkingDir:   projectDir,
				IsControlled: true,
				Containers:   []ProjectContainer{},
			}
		} else {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Project not found"})
		}
	}

	// Read compose content if controlled
	if project.IsControlled {
		composePath := filepath.Join(project.WorkingDir, "docker-compose.yml")
		content, err := os.ReadFile(composePath)
		if err == nil {
			project.ComposeContent = string(content)
		}
	}

	return c.JSON(http.StatusOK, project)
}

func (h *Handler) CreateProject(c echo.Context) error {
	var req CreateProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	if req.Name == "" || req.Content == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Name and content required"})
	}

	projectDir, err := projectDirForName(req.Name)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid project name"})
	}
	if err := os.MkdirAll(managedProjectsRoot, 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create project root: " + err.Error()})
	}
	rootInfo, err := os.Lstat(managedProjectsRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Managed project root is invalid"})
	}
	if _, err := os.Lstat(projectDir); err == nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "Project already exists"})
	} else if !os.IsNotExist(err) {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to inspect project directory: " + err.Error()})
	}
	if err := os.Mkdir(projectDir, 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create directory: " + err.Error()})
	}
	projectCreated := true
	defer func() {
		if projectCreated {
			_ = os.RemoveAll(projectDir)
		}
	}()

	composeFile := filepath.Join(projectDir, "docker-compose.yml")
	if err := os.WriteFile(composeFile, []byte(req.Content), 0644); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to write compose file: " + err.Error()})
	}

	// Run docker compose up -d
	dockerConfigDir, cleanup, authErr := h.ImageService.PrepareAuthConfigDir(c.Request().Context(), projectDir)
	if authErr != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "仓库凭据准备失败"})
	}
	defer cleanup()

	cmd := exec.CommandContext(c.Request().Context(), "docker", "compose", "up", "-d")
	cmd.Dir = projectDir
	if dockerConfigDir != "" {
		cmd.Env = append(os.Environ(), "DOCKER_CONFIG="+dockerConfigDir)
	}

	projectCreated = false // Retain recovery files once Docker may have changed state.
	output, err := cmd.CombinedOutput()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("部署失败: %s", string(output))})
	}

	projectCreated = false
	return c.JSON(http.StatusOK, map[string]string{"message": "Project deployed successfully"})
}

func (h *Handler) ProjectAction(c echo.Context) error {
	name := c.Param("name")
	if err := validateProjectName(name); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid project name"})
	}
	var req ProjectActionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	switch req.Action {
	case "up", "start", "stop", "restart", "down", "pull", "remove":
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "不支持的项目操作"})
	}
	projectDir, _ := projectDirForName(name)
	validatedDir, pathErr := validateManagedProjectDir(projectDir)
	if pathErr != nil && os.IsNotExist(pathErr) {
		// Fallback: lookup via containers
		containers, err := h.Docker.ContainerList(c.Request().Context(), types.ContainerListOptions{All: true})
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		found := false
		for _, c := range containers {
			if c.Labels["com.docker.compose.project"] == name {
				wd := c.Labels["com.docker.compose.project.working_dir"]
				if managedDir, err := validateManagedProjectDir(wd); err == nil {
					projectDir = managedDir
					found = true
					break
				}
			}
		}

		if !found {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Project not found or not managed"})
		}
	} else if pathErr != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Project path is not managed"})
	} else {
		projectDir = validatedDir
	}

	if req.Action == "remove" {
		if _, err := validateManagedProjectDir(projectDir); err != nil {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "Project path is not managed"})
		}
		// 1. Docker compose down
		cmd := exec.CommandContext(c.Request().Context(), "docker", "compose", "down")
		cmd.Dir = projectDir
		if err := cmd.Run(); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "项目停止失败，已保留项目文件"})
		}

		// 2. Remove directory
		if err := os.RemoveAll(projectDir); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to remove project directory: " + err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]string{"message": "Project removed successfully"})
	}

	// Execute standard action
	dockerConfigDir, cleanup, authErr := h.ImageService.PrepareAuthConfigDir(c.Request().Context(), projectDir)
	if authErr != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "仓库凭据准备失败"})
	}
	defer cleanup()

	args := []string{"compose", req.Action}
	if req.Action == "up" {
		args = append(args, "-d")
	}
	cmd := exec.CommandContext(c.Request().Context(), "docker", args...)
	cmd.Dir = projectDir
	if dockerConfigDir != "" {
		cmd.Env = append(os.Environ(), "DOCKER_CONFIG="+dockerConfigDir)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("项目操作失败: %s", string(output))})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Action success"})
}

func (h *Handler) UpdateProject(c echo.Context) error {
	name := c.Param("name")
	if err := validateProjectName(name); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid project name"})
	}
	var req UpdateProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	projectDir, _ := projectDirForName(name)
	projectDir, err := validateManagedProjectDir(projectDir)
	if err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Project is not managed"})
	}

	composeFile := filepath.Join(projectDir, "docker-compose.yml")
	if err := os.WriteFile(composeFile, []byte(req.Content), 0644); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to write compose file"})
	}

	// Redeploy
	// docker compose up -d --remove-orphans
	dockerConfigDir, cleanup, authErr := h.ImageService.PrepareAuthConfigDir(c.Request().Context(), projectDir)
	if authErr != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "仓库凭据准备失败"})
	}
	defer cleanup()

	cmd := exec.CommandContext(c.Request().Context(), "docker", "compose", "up", "-d", "--remove-orphans")
	cmd.Dir = projectDir
	if dockerConfigDir != "" {
		cmd.Env = append(os.Environ(), "DOCKER_CONFIG="+dockerConfigDir)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Redeployment failed: %s", string(output)),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Project updated and redeployed"})
}
