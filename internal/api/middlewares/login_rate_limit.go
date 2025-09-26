package middlewares

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

func LoginRateLimit(rdb *redis.Client, next http.Handler) http.Handler {
	// Defaults: 10 attempts per 5 minutes
	max := envInt("LOGIN_MAX_ATTEMPTS", 10)
	win := envDur("LOGIN_WINDOW", "5m")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)           // reuse from rate_limiter.go
		if ip == "" || rdb == nil { // fail-open if no IP/Redis
			next.ServeHTTP(w, r)
			return
		}

		ctx := context.Background()
		key := "rl:login:" + ip

		// INCR and set TTL if new
		n, err := rdb.Incr(ctx, key).Result()
		if err == nil && n == 1 {
			_ = rdb.Expire(ctx, key, win).Err()
		}
		if err == nil && n > int64(max) {
			w.Header().Set("Retry-After", strconv.Itoa(int(win.Seconds())))
			http.Error(w, "too many login attempts", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envDur(k, def string) time.Duration {
	s := def
	if v := os.Getenv(k); v != "" {
		s = v
	}
	d, _ := time.ParseDuration(s)
	return d
}
