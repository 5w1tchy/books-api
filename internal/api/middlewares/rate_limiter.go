package middlewares

import (
	"context"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// --------- Key helpers ---------

type KeyFunc func(r *http.Request) string

// Per-IP (good default). You can swap to per-user by reading auth and returning "user:<id>"
func PerIPKey(prefix string) KeyFunc {
	return func(r *http.Request) string {
		ip := clientIP(r)
		if ip == "" {
			ip = "unknown"
		}
		return prefix + ":" + ip
	}
}

func clientIP(r *http.Request) string {
	// X-Forwarded-For may have a list: client, proxy1, proxy2...
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return xrip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// --------- Token Bucket (Redis + Lua) ---------

type RedisTokenBucket struct {
	rdb      *redis.Client
	keyFn    KeyFunc
	ratePerS float64 // tokens per second
	burst    int     // bucket capacity
	script   *redis.Script
}

func NewRedisTokenBucket(rdb *redis.Client, ratePerSecond float64, burst int, keyFn KeyFunc) *RedisTokenBucket {
	lua := `
-- KEYS[1] = bucket key (hash with fields: tokens, ts)
-- ARGV[1] = ratePerS (float)
-- ARGV[2] = capacity (int)
-- Returns: {allowed (1/0), remaining_tokens (float), retry_after_ms (int)}
local key   = KEYS[1]
local rate  = tonumber(ARGV[1])
local cap   = tonumber(ARGV[2])

local t = redis.call('TIME')
local now_ms = (tonumber(t[1]) * 1000) + math.floor(tonumber(t[2]) / 1000)

local data = redis.call('HMGET', key, 'tokens', 'ts')
local tokens = tonumber(data[1])
local ts     = tonumber(data[2])

if tokens == nil then
  tokens = cap
  ts = now_ms
end

local delta_ms = now_ms - ts
if delta_ms > 0 then
  local refill = (delta_ms / 1000.0) * rate
  tokens = math.min(cap, tokens + refill)
end

local allowed = 0
local retry_after_ms = 0

if tokens >= 1.0 then
  tokens = tokens - 1.0
  allowed = 1
else
  allowed = 0
  retry_after_ms = math.ceil((1.0 - tokens) * 1000.0 / rate)
end

redis.call('HMSET', key, 'tokens', tokens, 'ts', now_ms)

local ttl_ms = math.ceil((cap / rate) * 1000.0)
redis.call('PEXPIRE', key, ttl_ms)

return {allowed, tokens, retry_after_ms}
`
	return &RedisTokenBucket{
		rdb:      rdb,
		keyFn:    keyFn,
		ratePerS: ratePerSecond,
		burst:    burst,
		script:   redis.NewScript(lua),
	}
}

func (tb *RedisTokenBucket) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := tb.keyFn(r)
		ctx := r.Context()

		res, err := tb.script.Run(ctx, tb.rdb, []string{key},
			strconv.FormatFloat(tb.ratePerS, 'f', -1, 64),
			strconv.Itoa(tb.burst),
		).Slice()

		if err != nil {
			log.Printf("[TokenBucket] Redis error: %v (allowing request)\n", err)
			next.ServeHTTP(w, r)
			return
		}

		allowed := res[0].(int64) == 1
		remainingStr := toString(res[1])
		retryAfterMs := toInt64(res[2])

		// Always expose headers
		w.Header().Set("X-RateLimit-Policy", "token-bucket")
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(tb.burst))
		w.Header().Set("X-RateLimit-Remaining", remainingStr)

		if !allowed {
			sec := (retryAfterMs + 999) / 1000
			if sec < 1 {
				sec = 1
			}
			w.Header().Set("Retry-After", strconv.FormatInt(sec, 10))

			// Log block
			log.Printf("[TokenBucket] Blocked request from %s (key=%s). Retry after %ds\n",
				r.RemoteAddr, key, sec)

			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// --------- Sliding Window (Redis ZSET) ---------

type RedisSlidingWindow struct {
	rdb    *redis.Client
	keyFn  KeyFunc
	limit  int
	window time.Duration
}

func NewRedisSlidingWindow(rdb *redis.Client, limit int, window time.Duration, keyFn KeyFunc) *RedisSlidingWindow {
	return &RedisSlidingWindow{rdb: rdb, keyFn: keyFn, limit: limit, window: window}
}

func (sw *RedisSlidingWindow) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		now := time.Now().UnixMilli()
		key := sw.keyFn(r)

		pipe := sw.rdb.TxPipeline()
		member := strconv.FormatInt(now, 10) + ":" + randomSuffix()
		pipe.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: member})
		pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(now-int64(sw.window/time.Millisecond), 10))
		countCmd := pipe.ZCard(ctx, key)
		pipe.PExpire(ctx, key, sw.window+time.Second)
		_, err := pipe.Exec(ctx)
		if err != nil {
			log.Printf("[SlidingWindow] Redis error: %v (allowing request)\n", err)
			next.ServeHTTP(w, r)
			return
		}
		count := countCmd.Val()

		w.Header().Set("X-RateLimit-Policy", "sliding-window")
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(sw.limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(maxInt(0, sw.limit-int(count))))

		if int(count) > sw.limit {
			oldestScore, err := sw.rdb.ZRangeWithScores(ctx, key, 0, 0).Result()
			var retrySec int64 = 1
			if err == nil && len(oldestScore) == 1 {
				oldest := int64(oldestScore[0].Score)
				ms := (oldest + int64(sw.window/time.Millisecond)) - now
				if ms < 1000 {
					ms = 1000
				}
				retrySec = (ms + 999) / 1000
			}
			w.Header().Set("Retry-After", strconv.FormatInt(retrySec, 10))

			// Log block
			log.Printf("[SlidingWindow] Blocked request from %s (key=%s). Retry after %ds\n",
				r.RemoteAddr, key, retrySec)

			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// --------- utils ---------

func toString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatInt(int64(t), 10)
	default:
		return "0"
	}
}

func toInt64(v interface{}) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case string:
		i, _ := strconv.ParseInt(t, 10, 64)
		return i
	case []byte:
		i, _ := strconv.ParseInt(string(t), 10, 64)
		return i
	case float64:
		return int64(t)
	default:
		return 0
	}
}

func randomSuffix() string {
	return strconv.FormatInt(time.Now().UnixNano()%1_000_000, 36)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
