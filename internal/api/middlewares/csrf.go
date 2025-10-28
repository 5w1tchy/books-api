package middlewares

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
)

type CSRFOptions struct {
	TokenHeader    string        // Default: "X-CSRF-Token"
	CookieName     string        // Default: "csrf_token"
	CookiePath     string        // Default: "/"
	CookieSecure   bool          // Set to true in production with HTTPS
	CookieSameSite http.SameSite // Default: SameSiteStrictMode
}

func DefaultCSRFOptions() CSRFOptions {
	return CSRFOptions{
		TokenHeader:    "X-CSRF-Token",
		CookieName:     "csrf_token",
		CookiePath:     "/",
		CookieSecure:   false, // Set to true in production
		CookieSameSite: http.SameSiteStrictMode,
	}
}

func CSRF(opts CSRFOptions) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip CSRF for safe methods
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			// Get or generate CSRF token
			cookie, err := r.Cookie(opts.CookieName)
			var expectedToken string

			if err != nil || cookie.Value == "" {
				// Generate new token
				expectedToken = generateCSRFToken()
				http.SetCookie(w, &http.Cookie{
					Name:     opts.CookieName,
					Value:    expectedToken,
					Path:     opts.CookiePath,
					Secure:   opts.CookieSecure,
					HttpOnly: true,
					SameSite: opts.CookieSameSite,
				})
			} else {
				expectedToken = cookie.Value
			}

			// For state-changing methods, validate CSRF token
			providedToken := r.Header.Get(opts.TokenHeader)
			if providedToken == "" {
				// Also check form data for traditional forms
				providedToken = r.FormValue("csrf_token")
			}

			if !isValidCSRFToken(expectedToken, providedToken) {
				http.Error(w, "CSRF token validation failed", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func generateCSRFToken() string {
	var b [32]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func isValidCSRFToken(expected, provided string) bool {
	if expected == "" || provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) == 1
}

// CSRFTokenFromRequest extracts CSRF token from request context or generates one
func CSRFTokenFromRequest(r *http.Request, cookieName string) string {
	if cookie, err := r.Cookie(cookieName); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	return generateCSRFToken()
}
