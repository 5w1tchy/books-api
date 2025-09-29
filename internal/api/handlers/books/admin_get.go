package books

import (
	"database/sql"
	"encoding/json"
	"net/http"

	storebooks "github.com/5w1tchy/books-api/internal/store/books"
	"github.com/redis/go-redis/v9"
)

// AdminGet: GET /admin/books/{key} - Get single book for admin
func AdminGet(db *sql.DB, rdb *redis.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		key := r.PathValue("key")
		if key == "" {
			http.Error(w, `{"status":"error","error":"missing key"}`, http.StatusBadRequest)
			return
		}

		book, err := storebooks.GetAdminBookByKey(r.Context(), db, key)
		if err == sql.ErrNoRows {
			http.Error(w, `{"status":"error","error":"not found"}`, http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, `{"status":"error","error":"failed to get book"}`, http.StatusInternalServerError)
			return
		}

		resp := struct {
			Status string               `json:"status"`
			Data   storebooks.AdminBook `json:"data"`
		}{"success", book}
		_ = json.NewEncoder(w).Encode(resp)
	})
}
