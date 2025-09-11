package middlewares

import (
	"log"
	"net/http"
)

var allowedOrigins = []string{
	"http://localhost:5173",
	"http://127.0.0.1:5173",
	// add your future frontend origin here (e.g., "https://books-ui.onrender.com")
}

func Cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && !isOriginAllowed(origin) {
			log.Printf("[CORS] Blocked request from origin: %s on %s %s\n",
				origin, r.Method, r.URL.Path)
			http.Error(w, "Origin not allowed", http.StatusForbidden)
			return
		}

		if isOriginAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}

		// Allow common headers + our Request-ID
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "3600")

		// Expose useful response headers to the browser (incl. X-Request-ID)
		w.Header().Set("Access-Control-Expose-Headers",
			"Authorization, X-Request-ID, X-RateLimit-Policy, X-RateLimit-Limit, X-RateLimit-Remaining, Retry-After, X-Response-Time")

		// Fast-path preflight
		if r.Method == http.MethodOptions {
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isOriginAllowed(origin string) bool {
	for _, o := range allowedOrigins {
		if o == origin {
			return true
		}
	}
	return false
}
