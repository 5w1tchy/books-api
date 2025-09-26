package jwtutil

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var cfg = LoadConfig()

// SignAccess returns (tokenString, jti).
func SignAccess(userID string, tokenVersion int, ttl time.Duration) (string, string, error) {
	jti, err := randJTI()
	if err != nil {
		return "", "", err
	}
	claims := NewAccessClaims(userID, jti, tokenVersion, ttl)
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := t.SignedString(cfg.Secret)
	return s, jti, err
}

// ParseAccess verifies HS256 signature and leeway, returning claims.
func ParseAccess(tokenStr string) (*AccessClaims, error) {
	parser := jwt.NewParser(jwt.WithLeeway(cfg.ClockSkew), jwt.WithValidMethods([]string{"HS256"}))
	token, err := parser.ParseWithClaims(tokenStr, &AccessClaims{}, func(t *jwt.Token) (interface{}, error) {
		return cfg.Secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*AccessClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func randJTI() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// Helper for env-based TTLs (e.g., "15m")
func DefaultAccessTTL() time.Duration {
	if v := parseDuration("AUTH_ACCESS_TTL", "15m"); v > 0 {
		return v
	}
	return 15 * time.Minute
}

func parseDuration(key, def string) time.Duration {
	s := def
	if v := os.Getenv(key); v != "" {
		s = v
	}
	d, _ := time.ParseDuration(s)
	return d
}
