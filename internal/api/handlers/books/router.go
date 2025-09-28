package books

import (
	"database/sql"
	"net/http"

	"github.com/redis/go-redis/v9"
)

// Public read-only Books handler.
// All writes moved under /admin/books/*.
func Handler(db *sql.DB, _ *redis.Client) http.Handler {
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
		case http.MethodOptions:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}
