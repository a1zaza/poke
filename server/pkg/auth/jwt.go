package auth

import (
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

const (
	SessionDuration = 24 * time.Hour
	sessionIssuer   = "poke"
	sessionAudience = "poke-web"
	jwtSecretSize   = 32
)

var (
	jwtSecretMu sync.RWMutex
	jwtSecret   []byte
)

type Claims struct {
	UserID string `json:"userId"`
	jwt.RegisteredClaims
}

func InitializeJWTSecret() error {
	secret := make([]byte, jwtSecretSize)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("generate JWT signing secret: %w", err)
	}

	jwtSecretMu.Lock()
	jwtSecret = secret
	jwtSecretMu.Unlock()
	return nil
}

func currentJWTSecret() ([]byte, error) {
	jwtSecretMu.RLock()
	defer jwtSecretMu.RUnlock()
	if len(jwtSecret) == 0 {
		return nil, errors.New("JWT secret is not initialized")
	}
	return append([]byte(nil), jwtSecret...), nil
}

func CreateSession(userID string) (string, error) {
	secret, err := currentJWTSecret()
	if err != nil {
		return "", err
	}

	now := time.Now()
	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    sessionIssuer,
			Subject:   userID,
			Audience:  jwt.ClaimStrings{sessionAudience},
			ExpiresAt: jwt.NewNumericDate(now.Add(SessionDuration)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

func VerifySession(tokenString string) (*Claims, error) {
	secret, err := currentJWTSecret()
	if err != nil {
		return nil, err
	}

	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	token, err := parser.ParseWithClaims(tokenString, &Claims{}, func(_ *jwt.Token) (interface{}, error) {
		return secret, nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	now := time.Now()
	if !claims.VerifyIssuer(sessionIssuer, true) || !claims.VerifyAudience(sessionAudience, true) {
		return nil, errors.New("invalid token scope")
	}
	if !claims.VerifyExpiresAt(now, true) || !claims.VerifyIssuedAt(now, true) || claims.IssuedAt == nil {
		return nil, errors.New("invalid token time claims")
	}

	return claims, nil
}
