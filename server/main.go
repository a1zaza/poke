package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/docker/docker/client"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"poke/api"
	"poke/internal/cluster"
	"poke/pkg/auth"
	"poke/pkg/config"
	"poke/pkg/images"
	"poke/webui"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	mode := os.Getenv("POKE_MODE")
	if mode == "" {
		mode = "standalone"
	}
	if mode != "standalone" && mode != "controller" && mode != "agent" {
		return errors.New("POKE_MODE 必须是 standalone、controller 或 agent")
	}
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer cli.Close()
	localEnabled := mode != "controller" || os.Getenv("POKE_LOCAL_ENABLED") == "true"
	service := images.NewService(cli)
	handler := &api.Handler{Docker: cli, ImageService: service, Mode: mode, LocalEnabled: localEnabled}
	if mode == "agent" {
		// Runtime authentication only; no Agent identity or task store.
		authRoot, err := os.MkdirTemp("", "poke-agent-auth-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(authRoot)
		service.AuthRoot = authRoot
		agent, err := cluster.NewAgent(api.NewExecutor(handler), cluster.AgentOptions{ControllerURL: os.Getenv("POKE_CONTROLLER_URL"), NodeID: os.Getenv("POKE_NODE_ID"), NodeToken: os.Getenv("POKE_NODE_TOKEN"), CAFile: os.Getenv("POKE_CA_FILE"), AllowHTTP: os.Getenv("POKE_ALLOW_INSECURE_HTTP") == "true"})
		if err != nil {
			return err
		}
		return agent.Run(ctx)
	}
	if err = auth.InitializeJWTSecret(); err != nil {
		return err
	}
	if err = config.Initialize(); err != nil {
		return err
	}
	e := echo.New()
	e.Use(middleware.Recover())
	e.Use(api.BrowserRequestProtection())
	e.Use(middleware.BodyLimit("1M"))
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{Format: "${time_rfc3339} | ${status} | ${latency_human} | ${method} | ${path}\n"}))
	e.GET("/health", func(c echo.Context) error { return c.String(200, "OK") })
	e.POST("/api/auth/login", handler.Login, api.LoginRateLimiter())
	e.POST("/api/auth/logout", handler.Logout)
	g := e.Group("/api", api.AuthMiddleware())
	g.GET("/auth/session", func(c echo.Context) error { return c.JSON(200, map[string]any{"authenticated": true, "mode": mode}) })
	if localEnabled {
		api.RegisterResources(g, handler)
	} else {
		handler.Docker = nil
	}
	g.GET("/settings/system", handler.GetSettings)
	g.POST("/settings/system", handler.UpdateSettings)
	if localEnabled {
		e.GET("/ws/containers/:id/exec", handler.ContainerExecWS)
	}
	if mode == "controller" {
		store, err := cluster.OpenStore(filepath.Join(config.Directory(), "controller"))
		if err != nil {
			return err
		}
		defer store.Close()
		if err = store.RecoverTasks(); err != nil {
			return err
		}
		controller := cluster.NewController(store)
		defer controller.Close()
		controller.Register(e, g)
		e.GET("/ws/nodes/:node/containers/:id/exec", controller.Terminal, api.AuthMiddleware())
	} else {
		g.GET("/nodes", func(c echo.Context) error { return c.JSON(200, []any{}) })
	}
	if err = webui.Register(e, config.Directory()); err != nil {
		return err
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "80"
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if e.Shutdown(shutdown) != nil {
			e.Close()
		}
	}()
	if err = e.Start(":" + port); err != nil && !errors.Is(err, http.ErrServerClosed) {
		stop()
		<-done
		return err
	}
	<-done
	return nil
}
