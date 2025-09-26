package middlewares

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	jwtutil "github.com/5w1tchy/books-api/internal/security/jwt"
)

// RequireAuth verifies Bearer JWT, checks token_version against DB, then injects userID into context.
func RequireAuth(db *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.Header.Get("Authorization")
		if raw == "" {
			http.Error(w, "missing Authorization header", http.StatusUnauthorized)
			return
		}
		tokenStr, err := bearer(raw)
		if err != nil {
			http.Error(w, "invalid Authorization header", http.StatusUnauthorized)
			return
		}
		claims, err := jwtutil.ParseAccess(tokenStr)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		// Check current token_version from DB
		var dbVer int
		err = db.QueryRow(`SELECT COALESCE(token_version,1) FROM public.users WHERE id = $1`, claims.Subject).Scan(&dbVer)
		if err != nil {
			http.Error(w, "user not found", http.StatusUnauthorized)
			return
		}
		if claims.TokenVersion != dbVer {
			http.Error(w, "token revoked", http.StatusUnauthorized)
			return
		}

		ctx := WithUserID(r.Context(), claims.Subject)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OptionalAuth attaches userID if a valid Bearer is present; otherwise continues unauthenticated.
func OptionalAuth(db *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.Header.Get("Authorization")
		if raw == "" {
			next.ServeHTTP(w, r)
			return
		}
		tokenStr, err := bearer(raw)
		if err != nil {
			next.ServeHTTP(w, r) // ignore bad header; act as guest
			return
		}
		claims, err := jwtutil.ParseAccess(tokenStr)
		if err != nil {
			next.ServeHTTP(w, r) // ignore invalid token; act as guest
			return
		}
		var dbVer int
		err = db.QueryRow(`SELECT COALESCE(token_version,1) FROM public.users WHERE id = $1`, claims.Subject).Scan(&dbVer)
		if err != nil || claims.TokenVersion != dbVer {
			next.ServeHTTP(w, r) // treat as guest if mismatch/user missing
			return
		}
		ctx := WithUserID(r.Context(), claims.Subject)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearer(h string) (string, error) {
	if !strings.HasPrefix(h, "Bearer ") && !strings.HasPrefix(h, "bearer ") {
		return "", errors.New("no bearer")
	}
	return strings.TrimSpace(h[len("Bearer "):]), nil
}
