package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"

	"github.com/moby/moby/api/types/volume"

	"github.com/labstack/echo/v4"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"

	"poke/pkg/auth"
	"poke/pkg/config"
	"poke/pkg/images"
	"poke/pkg/logger"
)

type Handler struct {
	LocalEnabled bool
	Mode         string
	Docker       *client.Client
	ImageService *images.Service
}

type ContainerListItem struct {
	container.Summary
	IsControlled bool `json:"isControlled"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) Login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	if config.VerifyCredentials(req.Username, req.Password) {
		token, err := auth.CreateSession("admin")
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create session"})
		}

		c.SetCookie(newSessionCookie(c, token, int(auth.SessionDuration.Seconds())))

		return c.JSON(http.StatusOK, map[string]bool{"success": true})
	}

	logger.Warn("AUTH", "Failed login attempt from %s", c.RealIP())
	return c.JSON(http.StatusUnauthorized, map[string]string{"error": "用户名或密码错误"})
}

func (h *Handler) Logout(c echo.Context) error {
	c.SetCookie(newSessionCookie(c, "", -1))
	return c.JSON(http.StatusOK, map[string]bool{"success": true})
}

// Middleware
func AuthMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Read from cookie
			cookie, err := c.Cookie(sessionCookieName)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
			}

			claims, err := auth.VerifySession(cookie.Value)
			if err != nil || claims == nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid session"})
			}

			// Store user in context if needed
			c.Set("user", claims.UserID)
			return next(c)
		}
	}
}

// Container Handlers

func (h *Handler) ListContainers(c echo.Context) error {
	containersResult, err := h.Docker.ContainerList(c.Request().Context(), client.ContainerListOptions{All: true})
	containers := containersResult.Items

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	items := make([]ContainerListItem, 0, len(containers))
	for _, item := range containers {
		projectName := item.Labels["com.docker.compose.project"]
		workingDir := item.Labels["com.docker.compose.project.working_dir"]
		items = append(items, ContainerListItem{
			Summary:      item,
			IsControlled: projectName != "" && isManagedProjectDir(workingDir),
		})
	}

	return c.JSON(http.StatusOK, items)
}

func (h *Handler) GetContainer(c echo.Context) error {
	id := c.Param("id")
	inspection, err := h.Docker.ContainerInspect(c.Request().Context(), id, client.ContainerInspectOptions{})
	info := inspection.Container
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Container not found"})
	}
	return c.JSON(http.StatusOK, info)
}

func (h *Handler) StartContainer(c echo.Context) error {
	id := c.Param("id")
	if _, err := h.Docker.ContainerStart(c.Request().Context(), id, client.ContainerStartOptions{}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]bool{"success": true})
}

func (h *Handler) StopContainer(c echo.Context) error {
	id := c.Param("id")
	if _, err := h.Docker.ContainerStop(c.Request().Context(), id, client.ContainerStopOptions{}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]bool{"success": true})
}

func (h *Handler) RestartContainer(c echo.Context) error {
	id := c.Param("id")
	if _, err := h.Docker.ContainerRestart(c.Request().Context(), id, client.ContainerRestartOptions{}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]bool{"success": true})
}

func (h *Handler) DeleteContainer(c echo.Context) error {
	id := c.Param("id")
	force := c.QueryParam("force") == "true"

	opts := client.ContainerRemoveOptions{
		Force: force,
	}

	if _, err := h.Docker.ContainerRemove(c.Request().Context(), id, opts); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]bool{"success": true})
}

func (h *Handler) GetContainerLogs(c echo.Context) error {
	id := c.Param("id")
	tail := c.QueryParam("tail")
	if tail == "" {
		tail = "100"
	}
	limit, err := strconv.Atoi(tail)
	if err != nil || limit < 1 || limit > 2000 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "日志行数必须在 1 到 2000 之间"})
	}
	timestamps := c.QueryParam("timestamps") == "true"

	options := client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
		Timestamps: timestamps,
	}

	reader, err := h.Docker.ContainerLogs(c.Request().Context(), id, options)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	defer reader.Close()

	// Read all logs
	logs, err := io.ReadAll(io.LimitReader(reader, 8*1024*1024+1))
	if len(logs) > 8*1024*1024 {
		return c.JSON(http.StatusRequestEntityTooLarge, map[string]string{"error": "日志内容超过限制，请减少查询行数"})
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"logs": string(logs)})
}

func (h *Handler) GetSystemEvents(c echo.Context) error {
	// Get events from the last 24 hours
	since := c.QueryParam("since")
	until := c.QueryParam("until")

	if since == "" {
		// Default to last 24 hours
		since = fmt.Sprintf("%d", time.Now().Unix()-24*60*60)
	}
	if until == "" {
		until = fmt.Sprintf("%d", time.Now().Unix())
	}

	eventStream := h.Docker.Events(c.Request().Context(), client.EventsListOptions{
		Since: since,
		Until: until,
	})

	messages, errs := eventStream.Messages, eventStream.Err
	var events []events.Message
	timeout := time.After(2 * time.Second)

	for {
		select {
		case event := <-messages:
			events = append(events, event)
		case err := <-errs:
			if err != nil && err != io.EOF {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}
			// Reverse to show latest first
			for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
				events[i], events[j] = events[j], events[i]
			}
			return c.JSON(http.StatusOK, events)
		case <-timeout:
			// Reverse to show latest first
			for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
				events[i], events[j] = events[j], events[i]
			}
			return c.JSON(http.StatusOK, events)
		}
	}
}

func (h *Handler) GetSystemInfo(c echo.Context) error {
	ctx := c.Request().Context()

	// Get high-level system info from Docker API
	infoResult, err := h.Docker.Info(ctx, client.InfoOptions{})
	dInfo := infoResult.Info
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get Docker info: " + err.Error()})
	}

	v, _ := mem.VirtualMemory()
	cInfos, _ := cpu.Info()
	// hInfo, _ := host.Info() // No longer using host.Info for hostname/platform if user wants Docker API

	// Docker Stats
	containersResult, _ := h.Docker.ContainerList(ctx, client.ContainerListOptions{All: true})
	containers := containersResult.Items
	imagesResult, _ := h.Docker.ImageList(ctx, client.ImageListOptions{})
	images := imagesResult.Items
	networksResult, _ := h.Docker.NetworkList(ctx, client.NetworkListOptions{})
	networks := networksResult.Items
	volumesResult, _ := h.Docker.VolumeList(ctx, client.VolumeListOptions{})
	volumes := volume.ListResponse{Volumes: volumesResult.Items, Warnings: volumesResult.Warnings}

	// Helper type for map key
	type portKey struct {
		Container string
		Public    uint16
		Private   uint16
		Proto     string // "tcp", "udp"
		State     string
	}

	type PortInfo struct {
		Container string `json:"container"`
		IP        string `json:"ip"`
		Public    uint16 `json:"public"`
		Private   uint16 `json:"private"`
		Type      string `json:"type"`
		State     string `json:"state"`
		// Internal flags for merging
		hasV4 bool
		hasV6 bool
	}

	// Use map to merge dual-stack ports
	portMap := make(map[portKey]*PortInfo)
	unmergedPorts := []PortInfo{} // For specific IPs typically

	running := 0
	stopped := 0
	for _, c := range containers {
		if c.State == "running" {
			running++
		} else {
			stopped++
		}

		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}

		if c.State == "running" {
			for _, p := range c.Ports {
				if p.PublicPort != 0 {
					ip := ""
					if p.IP.IsValid() {
						ip = p.IP.String()
					}
					isV6 := strings.Contains(ip, ":")
					isAny := ip == "0.0.0.0" || ip == "" || ip == "::"

					// Normalize IP
					if isAny {
						if isV6 {
							ip = "::"
						} else {
							ip = "0.0.0.0"
						}
					}

					// Clean proto
					proto := p.Type // e.g. "tcp"

					if isAny {
						key := portKey{
							Container: name,
							Public:    p.PublicPort,
							Private:   p.PrivatePort,
							Proto:     proto,
							State:     string(c.State),
						}

						if _, exists := portMap[key]; !exists {
							portMap[key] = &PortInfo{
								Container: name,
								Public:    p.PublicPort,
								Private:   p.PrivatePort,
								Type:      proto, // Base type
								State:     string(c.State),
								hasV4:     false,
								hasV6:     false,
							}
						}

						entry := portMap[key]
						if isV6 {
							entry.hasV6 = true
						} else {
							entry.hasV4 = true
						}
					} else {
						// Specific IP, add directly
						displayProto := proto
						if isV6 {
							displayProto += " (v6)"
						} else {
							displayProto += " (v4)"
						}
						unmergedPorts = append(unmergedPorts, PortInfo{
							Container: name,
							IP:        ip,
							Public:    p.PublicPort,
							Private:   p.PrivatePort,
							Type:      displayProto,
							State:     string(c.State),
						})
					}
				}
			}
		} else {
			// For stopped containers
			inspection, err := h.Docker.ContainerInspect(ctx, c.ID, client.ContainerInspectOptions{})
			cj := inspection.Container
			if err == nil && cj.HostConfig != nil && cj.HostConfig.PortBindings != nil {
				for pProto, bindings := range cj.HostConfig.PortBindings {
					parts := strings.Split(pProto.String(), "/")
					if len(parts) != 2 {
						continue
					}
					privatePort, _ := strconv.ParseUint(parts[0], 10, 16)
					protoType := parts[1]

					for _, b := range bindings {
						publicPort, _ := strconv.ParseUint(b.HostPort, 10, 16)
						if publicPort == 0 {
							continue
						}

						ip := ""
						if b.HostIP.IsValid() {
							ip = b.HostIP.String()
						}
						isV6 := strings.Contains(ip, ":")
						isAny := ip == "0.0.0.0" || ip == "" || ip == "::"

						if isAny {
							if isV6 {
								ip = "::"
							} else {
								ip = "0.0.0.0"
							}
						}

						if isAny {
							key := portKey{
								Container: name,
								Public:    uint16(publicPort),
								Private:   uint16(privatePort),
								Proto:     protoType,
								State:     string(c.State),
							}

							if _, exists := portMap[key]; !exists {
								portMap[key] = &PortInfo{
									Container: name,
									Public:    uint16(publicPort),
									Private:   uint16(privatePort),
									Type:      protoType,
									State:     string(c.State),
									hasV4:     false,
									hasV6:     false,
								}
							}
							entry := portMap[key]
							if isV6 {
								entry.hasV6 = true
							} else {
								entry.hasV4 = true
							}
						} else {
							// Specific IP
							displayProto := protoType
							if isV6 {
								displayProto += " (v6)"
							} else {
								displayProto += " (v4)"
							}

							unmergedPorts = append(unmergedPorts, PortInfo{
								Container: name,
								IP:        ip,
								Public:    uint16(publicPort),
								Private:   uint16(privatePort),
								Type:      displayProto,
								State:     string(c.State),
							})
						}
					}
				}
			}
		}
	}

	// Finalize merged ports
	for _, p := range portMap {
		if p.hasV4 && p.hasV6 {
			p.IP = "All (v4/v6)"
			// Type remains "tcp" etc.
		} else if p.hasV6 {
			p.IP = "::"
			p.Type = p.Type + " (v6)"
		} else {
			p.IP = "0.0.0.0"
			p.Type = p.Type + " (v4)"
		}

		// Append to result
		unmergedPorts = append(unmergedPorts, *p)
	}

	// Sort logic removed (frontend can sort, or we sort here?)
	// Let's sort simply by container name to be nice
	sort.Slice(unmergedPorts, func(i, j int) bool {
		return unmergedPorts[i].Container < unmergedPorts[j].Container
	})

	ports := unmergedPorts

	stats := map[string]interface{}{
		"hostname": dInfo.Name,
		"platform": dInfo.OperatingSystem,
		"release":  dInfo.KernelVersion,
		"arch":     dInfo.Architecture,
		"memory": map[string]interface{}{
			"total":        v.Total,
			"free":         v.Free,
			"used":         v.Used,
			"usagePercent": int(v.UsedPercent),
		},
		"cpus": map[string]interface{}{
			"model": func() string {
				if len(cInfos) > 0 {
					return cInfos[0].ModelName
				}
				return "Unknown"
			}(),
			"count": len(cInfos),
		},
		"docker": map[string]interface{}{
			"version": dInfo.ServerVersion,
			"containers": map[string]int{
				"total":   len(containers),
				"running": running,
				"stopped": stopped,
			},
			"images":   len(images),
			"volumes":  len(volumes.Volumes),
			"networks": len(networks),
		},
		"ports": ports,
	}

	return c.JSON(http.StatusOK, stats)
}

// Image Handlers

func (h *Handler) ListImages(c echo.Context) error {
	imagesResult, err := h.Docker.ImageList(c.Request().Context(), client.ImageListOptions{})
	images := imagesResult.Items
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, images)
}

func (h *Handler) DeleteImage(c echo.Context) error {
	id := c.Param("id")
	force := c.QueryParam("force") == "true"

	_, err := h.Docker.ImageRemove(c.Request().Context(), id, client.ImageRemoveOptions{
		Force:         force,
		PruneChildren: true,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]bool{"success": true})
}

type PullImageRequest struct {
	Image string `json:"image"`
}

func (h *Handler) PullImage(c echo.Context) error {
	var req PullImageRequest
	if err := c.Bind(&req); err != nil || req.Image == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid image name"})
	}

	domain, _, _ := images.ParseRepositoryInfo(req.Image)
	authStr := h.ImageService.ResolveAuthConfig(c.Request().Context(), domain)

	reader, err := h.Docker.ImagePull(c.Request().Context(), req.Image, client.ImagePullOptions{
		RegistryAuth: authStr,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	defer reader.Close()
	decoder := json.NewDecoder(reader)
	for {
		var event struct {
			Error string `json:"error"`
		}
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				break
			}
			return c.JSON(http.StatusBadGateway, map[string]string{"error": "镜像拉取响应中断"})
		}
		if event.Error != "" {
			return c.JSON(http.StatusBadGateway, map[string]string{"error": event.Error})
		}
	}

	return c.JSON(http.StatusOK, map[string]bool{"success": true})
}

func (h *Handler) PruneImages(c echo.Context) error {
	// prune all?
	all := c.QueryParam("all") == "true"

	// filters
	f := make(client.Filters)
	if !all {
		f.Add("dangling", "true")
	} else {
		f.Add("dangling", "false")
	}

	report, err := h.Docker.ImagePrune(c.Request().Context(), client.ImagePruneOptions{Filters: f})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, report.Report)
}

// Network Handlers

func (h *Handler) ListNetworks(c echo.Context) error {
	networksResult, err := h.Docker.NetworkList(c.Request().Context(), client.NetworkListOptions{})
	networks := networksResult.Items
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, networks)
}

func (h *Handler) RemoveNetwork(c echo.Context) error {
	id := c.Param("id")
	if _, err := h.Docker.NetworkRemove(c.Request().Context(), id, client.NetworkRemoveOptions{}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]bool{"success": true})
}

// Volume Handlers

func (h *Handler) ListVolumes(c echo.Context) error {
	volumes, err := h.Docker.VolumeList(c.Request().Context(), client.VolumeListOptions{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	// Keep the existing public Volumes/Warnings response, not the SDK result wrapper.
	return c.JSON(http.StatusOK, volume.ListResponse{Volumes: volumes.Items, Warnings: volumes.Warnings})
}

func (h *Handler) RemoveVolume(c echo.Context) error {
	name := c.Param("name")
	if _, err := h.Docker.VolumeRemove(c.Request().Context(), name, client.VolumeRemoveOptions{Force: true}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]bool{"success": true})
}

func (h *Handler) PruneVolumes(c echo.Context) error {
	// filters
	report, err := h.Docker.VolumePrune(c.Request().Context(), client.VolumePruneOptions{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, report.Report)
}

// Settings Handlers

func (h *Handler) GetSettings(c echo.Context) error {
	cfg := config.GetConfig()

	logoURL := "/logo.png?v=" + fmt.Sprintf("%d", time.Now().Unix()) // Cache buster, but path is always same

	hostname := cfg.System.Hostname
	if hostname == "" {
		if h.Docker != nil {
			infoResult, err := h.Docker.Info(c.Request().Context(), client.InfoOptions{})
			info := infoResult.Info
			if err == nil && info.Name != "" {
				hostname = info.Name
			}
		}

		if hostname == "" {
			hostname, _ = os.Hostname()
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"username":      cfg.Auth.Username,
		"local_enabled": h.LocalEnabled,
		"hostname":      hostname,
		"logo":          logoURL,
		"mode":          h.Mode,
		"registries": func() []interface{} {
			res := make([]interface{}, 0)
			for _, r := range cfg.System.Registries {
				res = append(res, map[string]string{
					"domain":   r.Domain,
					"username": r.Username,
					// Do not expose password
				})
			}
			return res
		}(),
	})
}

func (h *Handler) UpdateSettings(c echo.Context) error {
	var req map[string]interface{}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	updates := make(map[string]interface{})
	if raw, exists := req["username"]; exists {
		username, ok := raw.(string)
		if !ok {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "用户名必须为字符串"})
		}
		if err := config.ValidateUsername(username); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		updates["username"] = username
	}

	if hostname, ok := req["hostname"].(string); ok {
		updates["hostname"] = hostname
	}

	if logo, ok := req["logo"].(string); ok {
		updates["logo"] = logo
	}

	if password, ok := req["password"].(string); ok && password != "" {
		updates["password"] = password
	}

	if registries, ok := req["registries"]; ok {
		updates["registries"] = registries
	}

	if logo, ok := updates["logo"].(string); ok {
		if strings.HasPrefix(logo, "data:image/") {
			header, encoded, found := strings.Cut(logo, ",")
			if !found || !strings.HasSuffix(header, ";base64") {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "Logo 格式无效"})
			}
			data, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil || len(data) == 0 {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "Logo 数据无效"})
			}
			if err := config.SaveLogo(data); err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Logo 保存失败"})
			}
			updates["logo"] = "custom"
		} else if logo == "" {
			if err := config.ResetLogo(); err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Logo 重置失败"})
			}
		} else {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Logo 格式无效"})
		}
	}
	if err := config.UpdateConfig(updates); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]bool{"success": true})
}
