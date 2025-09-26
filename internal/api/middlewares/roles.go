package middlewares

import (
	"database/sql"
	"net/http"
)

// RequireRole wraps a handler and ensures the caller has the given role.
func RequireRole(db *sql.DB, role string, next http.Handler) http.Handler {
	// First ensure the user is authenticated
	base := RequireAuth(db, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := UserIDFrom(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var have string
		err := db.QueryRow(`SELECT role FROM public.users WHERE id=$1`, userID).Scan(&have)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if have != role {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
	return base
}
