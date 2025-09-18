package foryou

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Request-scoped cache guard: no PINGs, warn once per request, no retry storms.
type cache struct {
	rdb     *redis.Client
	enabled bool
	warned  bool
	prefix  string
	ttl     time.Duration
	shortTO time.Duration // short timeout per cache op
}

const (
	versionKey = "fy:ver" // global version counter in Redis
)

// newCache builds a request-scoped cache wrapper.
// Prefix resolution rules (in order):
//  1. FOR_YOU_CACHE_PREFIX (if set) — e.g., "fy:v42:" for forced manual control
//  2. "fy:v{INCR key}:" where {INCR key} comes from fy:ver (default 1 on miss)
//     If Redis read fails, we fail-open to "fy:v1:".
func newCache(rdb *redis.Client) *cache {
	if rdb == nil || os.Getenv("FOR_YOU_DISABLE_CACHE") == "1" {
		return &cache{enabled: false}
	}

	// TTL (default 2h)
	ttl := 2 * time.Hour
	if v := os.Getenv("FOR_YOU_CACHE_TTL"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			ttl = time.Duration(secs) * time.Second
		}
	}

	// Per-op timeout (default 150ms)
	shortTO := 150 * time.Millisecond
	if v := os.Getenv("FOR_YOU_CACHE_TIMEOUT_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			shortTO = time.Duration(ms) * time.Millisecond
		}
	}

	// Prefix: env override wins
	if p := os.Getenv("FOR_YOU_CACHE_PREFIX"); p != "" {
		return &cache{
			rdb:     rdb,
			enabled: true,
			prefix:  p,
			ttl:     ttl,
			shortTO: shortTO,
		}
	}

	// Otherwise, resolve from Redis version key fy:ver (default 1)
	prefix := "fy:v1:"
	{
		ctx, cancel := context.WithTimeout(context.Background(), shortTO)
		defer cancel()
		ver, err := rdb.Get(ctx, versionKey).Int64()
		if err == redis.Nil {
			ver = 1
		} else if err != nil {
			// fail-open default v1 + warn once at first operation
			// (we don't log here to avoid startup noise; warnOnce will catch on use)
			ver = 1
		}
		prefix = fmt.Sprintf("fy:v%d:", ver)
	}

	return &cache{
		rdb:     rdb,
		enabled: true,
		prefix:  prefix,
		ttl:     ttl,
		shortTO: shortTO,
	}
}

func (c *cache) key(block string) string {
	return c.prefix + block
}

// mget returns a parallel slice of []byte (nil when missing).
func (c *cache) mget(ctx context.Context, blocks ...string) ([][]byte, bool) {
	if !c.enabled {
		return nil, false
	}
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, c.shortTO)
	defer cancel()

	keys := make([]string, len(blocks))
	for i, b := range blocks {
		keys[i] = c.key(b)
	}
	res, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		c.warnOnce("cache mget failed: %v; bypassing cache for this request", err)
		return nil, false
	}
	out := make([][]byte, len(res))
	for i, v := range res {
		if v == nil {
			out[i] = nil
			continue
		}
		switch t := v.(type) {
		case string:
			out[i] = []byte(t)
		case []byte:
			out[i] = t
		default:
			b, _ := json.Marshal(t)
			out[i] = b
		}
	}
	return out, true
}

// setPipeline uses the cache's default TTL.
func (c *cache) setPipeline(ctx context.Context, kv map[string][]byte) {
	if !c.enabled || len(kv) == 0 {
		return
	}
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, c.shortTO)
	defer cancel()

	pipe := c.rdb.Pipeline()
	for k, v := range kv {
		pipe.SetEx(ctx, k, v, c.ttl)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		c.warnOnce("cache pipeline set failed: %v (muted next)", err)
	}
}

func (c *cache) warnOnce(format string, args ...any) {
	if c.warned {
		return
	}
	c.warned = true
	log.Printf("[for-you][cache] "+format, args...)
}

// BumpVersion increments the global version key (fy:ver).
// Call this AFTER a successful commit of write operations that affect the /for-you feed.
// Safe no-op if rdb is nil; short timeout; returns error only for logging if you want.
func BumpVersion(ctx context.Context, rdb *redis.Client) error {
	if rdb == nil {
		return nil
	}
	shortTO := 150 * time.Millisecond
	if v := os.Getenv("FOR_YOU_CACHE_TIMEOUT_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			shortTO = time.Duration(ms) * time.Millisecond
		}
	}
	cctx, cancel := context.WithTimeout(ctx, shortTO)
	defer cancel()
	if _, err := rdb.Incr(cctx, versionKey).Result(); err != nil {
		return fmt.Errorf("bump version failed: %w", err)
	}
	return nil
}
