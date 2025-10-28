package middlewares

import (
	"log"
	"net/http"
	"runtime/debug"
)

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// Get request ID if available
				rid := GetRequestID(r)
				if rid == "" {
					rid = "unknown"
				}

				// Log the panic with stack trace
				log.Printf("[PANIC] RequestID=%s URL=%s %s: %v\n%s",
					rid, r.Method, r.URL.Path, err, debug.Stack())

				// Don't expose internal errors to client
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
