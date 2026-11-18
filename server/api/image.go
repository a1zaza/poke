package api

import (
	"github.com/labstack/echo/v4"
	"net/http"
)

type CheckUpdateRequest struct {
	Image string `json:"image"`
	Force bool   `json:"force"`
}

func (h *Handler) CheckImageUpdate(c echo.Context) error {
	var req CheckUpdateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	if req.Image == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Image name required"})
	}

	// 用户手动检查时刷新缓存。
	force := true

	res, err := h.ImageService.CheckUpdate(c.Request().Context(), req.Image, force)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Adapt to frontend response format
	resp := map[string]interface{}{
		"hasUpdate":    res.HasUpdate,
		"remoteDigest": res.RemoteDigest,
	}
	if res.Error != "" {
		resp["error"] = res.Error
		resp["errorCode"] = res.ErrorCode
		resp["details"] = res.Details
		resp["registry"] = res.Registry
		resp["authConfigured"] = res.AuthConfigured
	}

	return c.JSON(http.StatusOK, resp)
}
