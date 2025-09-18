package books

import (
	"database/sql"
	"net/http"

	"github.com/redis/go-redis/v9"
)

// Handler returns a single handler that dispatches to the method-specific funcs.
// The top-level router mounts this same handler on multiple patterns.
func Handler(db *sql.DB, rdb *redis.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.PathValue("key") != "" {
				get(db)(w, r)
			} else {
				list(db)(w, r)
			}
		case http.MethodHead:
			head(db)(w, r)
		case http.MethodPost:
			create(db, rdb)(w, r)
		case http.MethodPatch:
			patch(db, rdb)(w, r)
		case http.MethodPut:
			put(db, rdb)(w, r)
		case http.MethodDelete:
			del(db, rdb)(w, r)
		case http.MethodOptions:
			// Preflight OK
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}
