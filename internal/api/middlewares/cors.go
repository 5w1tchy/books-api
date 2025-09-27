package middlewares

import (
	"log"
	"net/http"
	"os"
	"strings"
)

func Cors(next http.Handler) http.Handler {
	// Build allowlist once from env (comma-separated), with sensible dev defaults.
	allow := parseAllowlist(os.Getenv("CORS_ALLOW_ORIGINS"))
	if len(allow) == 0 {
		allow = []string{
			"http://localhost:5173",
			"http://127.0.0.1:5173",
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && !isOriginAllowed(origin, allow) {
			log.Printf("[CORS] Blocked origin: %s on %s %s", origin, r.Method, r.URL.Path)
			http.Error(w, "Origin not allowed", http.StatusForbidden)
			return
		}

		if isOriginAllowed(origin, allow) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}

		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PATCH, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "3600")

		w.Header().Set("Access-Control-Expose-Headers",
			"Authorization, X-Request-ID, X-RateLimit-Policy, X-RateLimit-Limit, X-RateLimit-Remaining, Retry-After, X-Response-Time, X-Password-Warning, X-Password-Score")

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
	var out []string
	for _, s := range strings.Split(env, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func isOriginAllowed(origin string, allow []string) bool {
	if origin == "" {
		return false
	}
	for _, o := range allow {
		if o == origin {
			return true
		}
	}
	return false
}
