package books

import (
	"database/sql"
	"encoding/json"
	"net/http"

	storebooks "github.com/5w1tchy/books-api/internal/store/books"
	"github.com/redis/go-redis/v9"
)

func AdminGetCategories(db *sql.DB, _ *redis.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		categories, err := storebooks.GetAllCategories(r.Context(), db)
		if err != nil {
			http.Error(w, `{"status":"error","error":"failed to get categories"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"data":   categories,
		})
	})
}

func AdminGetAuthors(db *sql.DB, _ *redis.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		authors, err := storebooks.GetAllAuthors(r.Context(), db)
		if err != nil {
			http.Error(w, `{"status":"error","error":"failed to get authors"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"data":   authors,
		})
	})
}
