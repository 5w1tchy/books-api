package books

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	storebooks "github.com/5w1tchy/books-api/internal/store/books"
	"github.com/redis/go-redis/v9"
)

// AdminList: GET /admin/books - List all books for admin panel
func AdminList(db *sql.DB, rdb *redis.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		q := r.URL.Query()
		page, _ := strconv.Atoi(q.Get("page"))
		if page < 1 {
			page = 1
		}
		size, _ := strconv.Atoi(q.Get("size"))
		if size < 1 || size > 100 {
			size = 25
		}

		filter := storebooks.ListBooksFilter{
			Query:      q.Get("query"),    // search title/author
			Category:   q.Get("category"), // filter by category
			AuthorName: q.Get("author"),   // filter by author
			Page:       page,
			Size:       size,
		}

		books, total, err := storebooks.ListAdminBooks(r.Context(), db, filter)
		if err != nil {
			http.Error(w, `{"status":"error","error":"failed to list books"}`, http.StatusInternalServerError)
			return
		}

		resp := struct {
			Status string                 `json:"status"`
			Data   []storebooks.AdminBook `json:"data"`
			Total  int                    `json:"total"`
			Page   int                    `json:"page"`
			Size   int                    `json:"size"`
		}{"success", books, total, page, size}

		_ = json.NewEncoder(w).Encode(resp)
	})
}
