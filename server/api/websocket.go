package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/moby/moby/client"
	"poke/pkg/auth"
	"poke/pkg/logger"
)

var upgrader = websocket.Upgrader{
	HandshakeTimeout: 10 * time.Second,
}

type WSMessage struct {
	Event string `json:"event"`
	Data  string `json:"data,omitempty"`
	Rows  int    `json:"rows,omitempty"`
	Cols  int    `json:"cols,omitempty"`
}

func (h *Handler) ContainerExecWS(c echo.Context) error {
	// 0. Auth Check
	cookie, err := c.Cookie(sessionCookieName)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
	}
	claims, err := auth.VerifySession(cookie.Value)
	if err != nil || claims == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid session"})
	}

	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()
	return h.Exec(c.Request().Context(), ws, c.Param("id"))
}

// Exec is shared by the local API and the outbound Agent data channel.
func (h *Handler) Exec(ctx context.Context, ws *websocket.Conn, id string) error {
	ws.SetReadLimit(64 * 1024)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			ws.Close()
		case <-done:
		}
	}()
	logger.Info("CONTAINER_EXEC", "Starting exec for container: %s", id)

	// 1. Create Exec Instance
	execConfig := client.ExecCreateOptions{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		TTY:          true,
		Cmd:          []string{"/bin/sh"},
	}

	execIDResp, err := h.Docker.ExecCreate(ctx, id, execConfig)
	if err != nil {
		logger.Error("CONTAINER_EXEC", "Error creating exec: %v", err)
		ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error creating exec: %v", err)))
		return nil
	}

	// 2. Attach to Exec
	resp, err := h.Docker.ExecAttach(ctx, execIDResp.ID, client.ExecAttachOptions{
		TTY: true,
	})
	if err != nil {
		logger.Error("CONTAINER_EXEC", "Error attaching: %v", err)
		ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error attaching: %v", err)))
		return nil
	}
	defer resp.Close()

	// 3. Handle I/O

	// Stream Docker Output -> WS
	go func() {
		defer ws.Close()
		buf := make([]byte, 8192)
		for {
			n, err := resp.Reader.Read(buf)
			if err != nil {
				if err != io.EOF {
					logger.Warn("CONTAINER_EXEC", "Read error: %v", err)
				}
				break
			}
			if n > 0 {
				if err := ws.WriteMessage(websocket.TextMessage, buf[:n]); err != nil {
					break
				}
			}
		}
	}()

	// Stream WS Input -> Docker
	for {
		_, msgData, err := ws.ReadMessage()
		if err != nil {
			break
		}

		var msg WSMessage
		if err := json.Unmarshal(msgData, &msg); err != nil {
			continue
		}

		if msg.Event == "data" {
			resp.Conn.Write([]byte(msg.Data))
		} else if msg.Event == "resize" {
			h.Docker.ExecResize(ctx, execIDResp.ID, client.ExecResizeOptions{
				Height: uint(msg.Rows),
				Width:  uint(msg.Cols),
			})
		}
	}

	return nil
}
