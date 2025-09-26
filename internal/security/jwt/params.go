package jwtutil

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Secret    []byte
	ClockSkew time.Duration
}

func LoadConfig() Config {
	secret := os.Getenv("AUTH_JWT_SECRET")
	if len(secret) < 32 {
		// keep going in dev, but you should set a strong secret in .env / prod
	}
	leeway := time.Duration(parseInt("AUTH_CLOCK_SKEW_SEC", 60)) * time.Second
	return Config{
		Secret:    []byte(secret),
		ClockSkew: leeway,
	}
}

func parseInt(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}
