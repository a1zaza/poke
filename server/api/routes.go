package api

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"net/http"
)

// RegisterResources exposes only the bounded Docker/Compose operation surface.
// The Controller authenticates browser calls; Agent calls arrive over its authenticated control connection.
func RegisterResources(g *echo.Group, h *Handler) {
	g.GET("/containers", h.ListContainers)
	g.GET("/containers/:id", h.GetContainer)
	g.POST("/containers/:id/start", h.StartContainer)
	g.POST("/containers/:id/stop", h.StopContainer)
	g.POST("/containers/:id/restart", h.RestartContainer)
	g.DELETE("/containers/:id", h.DeleteContainer)
	g.POST("/containers/:id/update", h.UpdateContainer)
	g.GET("/containers/:id/logs", h.GetContainerLogs)
	g.GET("/system/info", h.GetSystemInfo)
	g.GET("/system/events", h.GetSystemEvents)
	g.GET("/images", h.ListImages)
	g.POST("/images/pull", h.PullImage)
	g.POST("/images/check-update", h.CheckImageUpdate)
	g.POST("/images/prune", h.PruneImages)
	g.DELETE("/images/:id", h.DeleteImage)
	g.GET("/networks", h.ListNetworks)
	g.DELETE("/networks/:id", h.RemoveNetwork)
	g.GET("/volumes", h.ListVolumes)
	g.POST("/volumes/prune", h.PruneVolumes)
	g.DELETE("/volumes/:name", h.RemoveVolume)
	g.GET("/projects", h.ListProjects)
	g.POST("/projects", h.CreateProject)
	g.GET("/projects/:name", h.GetProject)
	g.PUT("/projects/:name", h.UpdateProject)
	g.POST("/projects/:name/action", h.ProjectAction)
}

type Executor struct {
	*Handler
	Router *echo.Echo
}

func NewExecutor(h *Handler) *Executor {
	e := echo.New()
	e.Use(middleware.Recover())
	RegisterResources(e.Group("/api"), h)
	return &Executor{Handler: h, Router: e}
}
func (e *Executor) ServeHTTP(w http.ResponseWriter, r *http.Request) { e.Router.ServeHTTP(w, r) }
