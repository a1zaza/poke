package api

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"poke/pkg/auth"
	"poke/pkg/logger"
)

const sessionCookieName = "poke_session"

// BrowserRequestProtection also protects login/logout. Agent requests do not
// carry browser Origin/Fetch Metadata headers and still require node credentials.
func BrowserRequestProtection() echo.MiddlewareFunc {
	protection := http.NewCrossOriginProtection()
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if strings.HasPrefix(c.Request().URL.Path, "/api/") {
				if err := protection.Check(c.Request()); err != nil {
					return c.JSON(http.StatusForbidden, map[string]string{"error": "不允许跨源修改请求"})
				}
			}
			return next(c)
		}
	}
}

func LoginRateLimiter() echo.MiddlewareFunc {
	store := middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{
		Rate:      5.0 / 60.0,
		Burst:     5,
		ExpiresIn: 15 * time.Minute,
	})
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: store,
		DenyHandler: func(c echo.Context, _ string, _ error) error {
			logger.Warn("AUTH", "Rate limited login attempt from %s", c.RealIP())
			return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "登录尝试过于频繁，请稍后再试"})
		},
	})
}

func newSessionCookie(c echo.Context, value string, maxAge int) *http.Cookie {
	expires := time.Now().Add(auth.SessionDuration)
	if maxAge < 0 {
		expires = time.Unix(1, 0)
	}

	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   requestUsesHTTPS(c),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
		Expires:  expires,
	}
}

func requestUsesHTTPS(c echo.Context) bool {
	if strings.EqualFold(os.Getenv("COOKIE_SECURE"), "true") || c.Request().TLS != nil {
		return true
	}
	forwardedProto := strings.TrimSpace(strings.Split(c.Request().Header.Get(echo.HeaderXForwardedProto), ",")[0])
	return strings.EqualFold(forwardedProto, "https")
}
