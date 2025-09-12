package middlewares

import (
	"net/http"
	"os"
)

func SecurityHeaders(next http.Handler) http.Handler {
	strict := os.Getenv("STRICT_SECURITY") == "1"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-DNS-Prefetch-Control", "off")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")

		// HSTS should only be effective over HTTPS (r.TLS != nil)
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		}

		// Minimal, safe-by-default CSP (adjust as you add assets/CDNs)
		w.Header().Set("Content-Security-Policy", "default-src 'self'")

		// Optional: COOP/COEP can break some embeds/workers unless all deps are compliant
		if strict {
			w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
			w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
			w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		}

		// Clean server banner
		w.Header().Set("Server", "")

		next.ServeHTTP(w, r)
	})
}
