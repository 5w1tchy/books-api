package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strconv"
	"time"
)

// issueRefresh creates and stores a refresh token in Redis
func (h *Handler) issueRefresh(ctx context.Context, userID string, tokenVersion int) (string, error) {
	token, err := randToken()
	if err != nil {
		return "", err
	}
	if h.RDB == nil {
		return "", errors.New("redis not configured")
	}
	key := "rt:" + token
	val := userID + "|" + itoa(tokenVersion)
	if err := h.RDB.Set(ctx, key, val, refreshTTL()).Err(); err != nil {
		return "", err
	}
	return token, nil
}

// refreshTTL returns the refresh token TTL from environment or default 30 days
func refreshTTL() time.Duration {
	if s := os.Getenv("AUTH_REFRESH_TTL"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			return d
		}
	}
	return 30 * 24 * time.Hour
}

// randToken generates a random hex token
func randToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// itoa converts int to string
func itoa(i int) string {
	return strconv.FormatInt(int64(i), 10)
}
