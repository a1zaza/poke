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

func TestBrowserRequestProtection(t *testing.T) {
	for _, tt := range []struct {
		name, method, origin, site string
		want                       int
	}{
		{"same origin", "POST", "https://controller.example.com", "same-origin", 204},
		{"origin fallback", "POST", "https://controller.example.com", "", 204},
		{"sibling domain", "POST", "https://other.example.com", "same-site", 403},
		{"cross site", "DELETE", "https://evil.invalid", "cross-site", 403},
		{"legacy browser", "PUT", "https://evil.invalid", "", 403},
		{"null origin", "POST", "null", "", 403},
		{"different port", "POST", "https://controller.example.com:8443", "", 403},
		{"read", "GET", "https://evil.invalid", "cross-site", 204},
		{"machine request", "POST", "", "", 204},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			e.Use(BrowserRequestProtection())
			called := false
			e.Add(tt.method, "/api/test", func(c echo.Context) error {
				called = true
				return c.NoContent(204)
			})
			req := httptest.NewRequest(tt.method, "https://controller.example.com/api/test", nil)
			req.Header.Set("Origin", tt.origin)
			req.Header.Set("Sec-Fetch-Site", tt.site)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != tt.want || called != (tt.want == 204) {
				t.Fatalf("status=%d handler called=%v, want %d", rec.Code, called, tt.want)
			}
		})
	}
}
