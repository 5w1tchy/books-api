package jwtutil

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type AccessClaims struct {
	TokenVersion int `json:"tv"`
	jwt.RegisteredClaims
}

func NewAccessClaims(userID, jti string, tokenVersion int, ttl time.Duration) AccessClaims {
	now := time.Now()
	return AccessClaims{
		TokenVersion: tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
}
