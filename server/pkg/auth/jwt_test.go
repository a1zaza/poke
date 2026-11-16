package auth

import (
	"bytes"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

func TestInitializeJWTSecretGeneratesNewSecret(t *testing.T) {
	configureTestJWTSecret(t)
	first := currentTestJWTSecret(t)
	configureTestJWTSecret(t)
	second := currentTestJWTSecret(t)

	if len(first) != jwtSecretSize || len(second) != jwtSecretSize {
		t.Fatalf("unexpected secret sizes: %d and %d", len(first), len(second))
	}
	if bytes.Equal(first, second) {
		t.Fatal("InitializeJWTSecret() reused the previous secret")
	}
}

func TestCreateSessionRequiresConfiguredSecret(t *testing.T) {
	jwtSecretMu.Lock()
	previous := append([]byte(nil), jwtSecret...)
	jwtSecret = nil
	jwtSecretMu.Unlock()
	t.Cleanup(func() {
		jwtSecretMu.Lock()
		jwtSecret = previous
		jwtSecretMu.Unlock()
	})

	if _, err := CreateSession("admin"); err == nil {
		t.Fatal("CreateSession() succeeded without a configured secret")
	}
}

func TestCreateAndVerifySession(t *testing.T) {
	configureTestJWTSecret(t)
	token, err := CreateSession("admin")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	claims, err := VerifySession(token)
	if err != nil {
		t.Fatalf("VerifySession() error = %v", err)
	}
	if claims.UserID != "admin" || claims.Issuer != sessionIssuer || !claims.VerifyAudience(sessionAudience, true) {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestRestartInvalidatesExistingSession(t *testing.T) {
	configureTestJWTSecret(t)
	token, err := CreateSession("admin")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	configureTestJWTSecret(t)
	if _, err := VerifySession(token); err == nil {
		t.Fatal("VerifySession() accepted a token signed before secret rotation")
	}
}

func TestVerifySessionRejectsWrongAlgorithm(t *testing.T) {
	configureTestJWTSecret(t)
	claims := validTestClaims()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS384, claims).SignedString(currentTestJWTSecret(t))
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}
	if _, err := VerifySession(token); err == nil {
		t.Fatal("VerifySession() accepted an unexpected signing algorithm")
	}
}

func TestVerifySessionRejectsInvalidScopeAndExpiry(t *testing.T) {
	configureTestJWTSecret(t)
	secret := currentTestJWTSecret(t)
	tests := []struct {
		name   string
		mutate func(*Claims)
	}{
		{name: "issuer", mutate: func(c *Claims) { c.Issuer = "other" }},
		{name: "audience", mutate: func(c *Claims) { c.Audience = jwt.ClaimStrings{"other"} }},
		{name: "expired", mutate: func(c *Claims) { c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute)) }},
		{name: "missing expiry", mutate: func(c *Claims) { c.ExpiresAt = nil }},
		{name: "missing issued at", mutate: func(c *Claims) { c.IssuedAt = nil }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := validTestClaims()
			tt.mutate(claims)
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
			if err != nil {
				t.Fatalf("SignedString() error = %v", err)
			}
			if _, err := VerifySession(token); err == nil {
				t.Fatal("VerifySession() accepted invalid claims")
			}
		})
	}
}

func configureTestJWTSecret(t *testing.T) {
	t.Helper()
	if err := InitializeJWTSecret(); err != nil {
		t.Fatalf("InitializeJWTSecret() error = %v", err)
	}
}

func currentTestJWTSecret(t *testing.T) []byte {
	t.Helper()
	secret, err := currentJWTSecret()
	if err != nil {
		t.Fatalf("currentJWTSecret() error = %v", err)
	}
	return secret
}

func validTestClaims() *Claims {
	now := time.Now()
	return &Claims{
		UserID: "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    sessionIssuer,
			Subject:   "admin",
			Audience:  jwt.ClaimStrings{sessionAudience},
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Second)),
			IssuedAt:  jwt.NewNumericDate(now.Add(-time.Second)),
		},
	}
}
