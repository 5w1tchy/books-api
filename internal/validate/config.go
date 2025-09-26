package validate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Env validates required env configuration for auth & security.
// Fail-fast on bad config.
func Env() error {
	// JWT secret must be present & reasonably long
	secret := os.Getenv("AUTH_JWT_SECRET")
	if len(secret) < 32 {
		return errors.New("AUTH_JWT_SECRET must be at least 32 characters")
	}

	// Access/Refresh TTLs must parse and be > 0 (defaults are fine if unset)
	if _, err := envDuration("AUTH_ACCESS_TTL", "15m"); err != nil {
		return fmt.Errorf("AUTH_ACCESS_TTL: %w", err)
	}
	if _, err := envDuration("AUTH_REFRESH_TTL", "720h"); err != nil {
		return fmt.Errorf("AUTH_REFRESH_TTL: %w", err)
	}

	// Argon2 lower bounds (only enforce if explicitly set)
	if err := envMinUint("ARGON2_MEMORY", 65536); err != nil { // >= 64MiB
		return fmt.Errorf("ARGON2_MEMORY: %w", err)
	}
	if err := envMinUint("ARGON2_ITER", 2); err != nil { // >= 2
		return fmt.Errorf("ARGON2_ITER: %w", err)
	}
	if err := envMinUint("ARGON2_PAR", 1); err != nil { // >= 1
		return fmt.Errorf("ARGON2_PAR: %w", err)
	}
	return nil
}

// HardeningWarnings returns non-fatal warnings you may want to log on startup.
func HardeningWarnings(appEnv string) []string {
	var warns []string

	// Access TTL unusually long?
	if d, _ := envDuration("AUTH_ACCESS_TTL", "15m"); d > time.Hour {
		warns = append(warns, fmt.Sprintf("AUTH_ACCESS_TTL=%s is > 1h; consider shorter access tokens", d))
	}

	// Refresh TTL unusually short?
	if d, _ := envDuration("AUTH_REFRESH_TTL", "720h"); d < 24*time.Hour {
		warns = append(warns, fmt.Sprintf("AUTH_REFRESH_TTL=%s is < 24h; users may be logged out too often", d))
	}

	// Production-specific nudges
	if strings.EqualFold(appEnv, "production") {
		if os.Getenv("ARGON2_MEMORY") == "" || os.Getenv("ARGON2_ITER") == "" {
			warns = append(warns, "ARGON2_* not explicitly set; using code defaults. Set strong values in production")
		}
		// Redis transport/auth checks from envs
		if u := os.Getenv("UPSTASH_REDIS_URL"); u != "" && strings.HasPrefix(u, "redis://") {
			warns = append(warns, "UPSTASH_REDIS_URL uses redis:// (no TLS). Prefer rediss:// for TLS")
		}
		if os.Getenv("UPSTASH_REDIS_URL") == "" {
			// Using REDIS_ADDR path
			if os.Getenv("REDIS_PASSWORD") == "" || os.Getenv("REDIS_USER") == "" {
				warns = append(warns, "REDIS_ADDR provided without REDIS_USER/REDIS_PASSWORD; require auth in production")
			}
		}
	}

	return warns
}

// PingRedis checks connectivity with a short timeout.
func PingRedis(rdb *redis.Client, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, err := rdb.Ping(ctx).Result()
	return err
}

// --- helpers ---

func envDuration(key, def string) (time.Duration, error) {
	s := os.Getenv(key)
	if s == "" {
		s = def
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	return d, nil
}

func envMinUint(key string, min uint64) error {
	v := os.Getenv(key)
	if v == "" {
		return nil // unset -> code defaults apply elsewhere
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return fmt.Errorf("not a number: %v", err)
	}
	if n < min {
		return fmt.Errorf("must be >= %d", min)
	}
	return nil
}
