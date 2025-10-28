package middlewares

import (
	"net/http"
	"os"
	"strconv"
)

func BodySizeLimit(next http.Handler) http.Handler {
	// Default 10MB limit, configurable via env
	limit := int64(10 * 1024 * 1024) // 10MB

	if envLimit := os.Getenv("MAX_BODY_SIZE"); envLimit != "" {
		if parsed, err := strconv.ParseInt(envLimit, 10, 64); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only apply to requests with bodies
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}
