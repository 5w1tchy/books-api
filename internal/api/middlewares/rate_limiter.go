package middlewares

import (
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

type KeyFunc func(r *http.Request) string

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

type RedisTokenBucket struct {
	rdb      *redis.Client
	keyFn    KeyFunc
	ratePerS float64
	burst    int
	script   *redis.Script
}

func NewRedisTokenBucket(rdb *redis.Client, ratePerSecond float64, burst int, keyFn KeyFunc) *RedisTokenBucket {
	lua := `
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
if tokens >= 1.0 then
  tokens = tokens - 1.0
  allowed = 1
else
  allowed = 0
end

redis.call('HMSET', key, 'tokens', tokens, 'ts', now_ms)
redis.call('PEXPIRE', key, math.ceil((cap / rate) * 1000.0))

return {allowed, tokens}
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
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

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
		remaining := toString(res[1])

		w.Header().Set("X-RateLimit-Policy", "token-bucket")
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(tb.burst))
		w.Header().Set("X-RateLimit-Remaining", remaining)

		if !allowed {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

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
