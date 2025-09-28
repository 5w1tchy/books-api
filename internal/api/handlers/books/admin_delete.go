package books

import (
	"database/sql"
	"log"
	"net/http"

	storebooks "github.com/5w1tchy/books-api/internal/store/books"
	storeforyou "github.com/5w1tchy/books-api/internal/store/foryou"
	"github.com/redis/go-redis/v9"
)

// AdminDelete: DELETE /admin/books/{key}
func AdminDelete(db *sql.DB, rdb *redis.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		key := r.PathValue("key")
		if key == "" {
			http.Error(w, `{"status":"error","error":"missing key"}`, http.StatusBadRequest)
			return
		}

		if err := storebooks.Delete(r.Context(), db, key); err == sql.ErrNoRows {
			http.Error(w, `{"status":"error","error":"not found"}`, http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, `{"status":"error","error":"failed to delete"}`, http.StatusInternalServerError)
			return
		}

		// Best-effort global cache bust for /for-you
		if err := storeforyou.BumpVersion(r.Context(), rdb); err != nil {
			log.Printf("[for-you] bump version failed: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
