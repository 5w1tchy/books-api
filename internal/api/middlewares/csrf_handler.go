package middlewares

import (
	"encoding/json"
	"net/http"
)

// CSRFToken endpoint to provide CSRF tokens to frontend
func CSRFTokenHandler(opts CSRFOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		token := CSRFTokenFromRequest(r, opts.CookieName)

		// Set the cookie
		http.SetCookie(w, &http.Cookie{
			Name:     opts.CookieName,
			Value:    token,
			Path:     opts.CookiePath,
			Secure:   opts.CookieSecure,
			HttpOnly: true,
			SameSite: opts.CookieSameSite,
		})

		// Return token in response for JavaScript access
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"csrf_token": token,
		})
	}
}
