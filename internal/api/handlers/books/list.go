package books

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	storebooks "github.com/5w1tchy/books-api/internal/store/books"
)

type PublicBook struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Authors    []string  `json:"authors"`
	Categories []string  `json:"categories"`
	CreatedAt  time.Time `json:"created_at"`
}

func list(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		// Get query parameters
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		author := strings.TrimSpace(r.URL.Query().Get("author"))
		category := strings.TrimSpace(r.URL.Query().Get("category"))
		limit := clamp(parseInt(r.URL.Query().Get("limit"), 20), 1, 100)
		offset := clamp(parseInt(r.URL.Query().Get("offset"), 0), 0, 100000)

		// Convert offset to page for the admin function
		page := (offset / limit) + 1

		// Use the same filter as admin but with public-appropriate fields
		filter := storebooks.ListBooksFilter{
			Query:      q,
			Category:   category,
			AuthorName: author,
			Page:       page,
			Size:       limit,
		}

		// Use the existing ListAdminBooks function (it works!)
		books, total, err := storebooks.ListAdminBooks(r.Context(), db, filter)
		if err != nil {
			http.Error(w, `{"status":"error","error":"failed to list"}`, http.StatusInternalServerError)
			return
		}

		// Convert AdminBook to PublicBook format (remove sensitive fields)
		publicBooks := make([]PublicBook, len(books))
		for i, book := range books {
			publicBooks[i] = PublicBook{
				ID:         book.ID,
				Title:      book.Title,
				Authors:    book.Authors,
				Categories: book.Categories,
				CreatedAt:  book.CreatedAt,
			}
		}

		resp := struct {
			Status string       `json:"status"`
			Data   []PublicBook `json:"data"`
			Total  int          `json:"total"`
			Limit  int          `json:"limit"`
			Offset int          `json:"offset"`
		}{
			Status: "success",
			Data:   publicBooks,
			Total:  total,
			Limit:  limit,
			Offset: offset,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}
