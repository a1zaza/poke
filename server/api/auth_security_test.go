package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestSessionCookieSecurityAttributes(t *testing.T) {
	t.Setenv("COOKIE_SECURE", "")
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.Header.Set(echo.HeaderXForwardedProto, "https")
	c := e.NewContext(req, httptest.NewRecorder())

	cookie := newSessionCookie(c, "token", 3600)
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected cookie security attributes: %+v", cookie)
	}
	if cookie.Path != "/" || cookie.Expires.IsZero() {
		t.Fatalf("unexpected cookie scope or expiry: %+v", cookie)
	}
}

func TestLoginRateLimiter(t *testing.T) {
	e := echo.New()
	e.POST("/login", func(c echo.Context) error {
		return c.NoContent(http.StatusUnauthorized)
	}, LoginRateLimiter())

	for i := 1; i <= 6; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		want := http.StatusUnauthorized
		if i == 6 {
			want = http.StatusTooManyRequests
		}
		if rec.Code != want {
			t.Fatalf("request %d status = %d, want %d", i, rec.Code, want)
		}
	}
}
