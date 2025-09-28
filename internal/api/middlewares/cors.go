package middlewares

import (
	"log"
	"net/http"
	"os"
	"strings"
)

func Cors(next http.Handler) http.Handler {
	// Allowlist from env (comma-separated). Add dev defaults if empty.
	allow := parseAllowlist(os.Getenv("CORS_ALLOW_ORIGINS"))
	if len(allow) == 0 {
		allow = []string{
			"http://localhost:5173",
			"http://127.0.0.1:5173",
			"http://localhost:3000",
			"http://127.0.0.1:3000",
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := normalizeOrigin(r.Header.Get("Origin"))

		// If the request comes from a browser with an Origin and it's not allowed, block it.
		if origin != "" && !isOriginAllowed(origin, allow) {
			log.Printf("[CORS] Blocked origin: %s on %s %s", origin, r.Method, r.URL.Path)
			http.Error(w, "Origin not allowed", http.StatusForbidden)
			return
		}

		// Allowed origin → set headers
		if isOriginAllowed(origin, allow) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		// Always advertise what we accept
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Requested-With, X-Request-ID")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PATCH, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Max-Age", "3600")
		w.Header().Set("Access-Control-Expose-Headers",
			"Authorization, X-Request-ID, X-RateLimit-Policy, X-RateLimit-Limit, X-RateLimit-Remaining, Retry-After, X-Response-Time")

		// Preflight
		if r.Method == http.MethodOptions {
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func parseAllowlist(env string) []string {
	if env == "" {
		return nil
	}
	parts := strings.Split(env, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := normalizeOrigin(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func normalizeOrigin(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "/")
	return s
}

func isOriginAllowed(origin string, allow []string) bool {
	if origin == "" {
		return false
	}
	for _, o := range allow {
		if normalizeOrigin(o) == origin {
			return true
		}
	}
	return false
}
