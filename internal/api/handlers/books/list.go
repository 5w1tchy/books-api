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
	Slug       string    `json:"slug"`
	Title      string    `json:"title"`
	Authors    []string  `json:"authors"`
	Author     string    `json:"author"` // For compatibility
	Categories []string  `json:"categories"`
	ImageUrl   string    `json:"imageUrl"`
	Short      string    `json:"short,omitempty"`
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
			// Get author as comma-separated string
			author := ""
			if len(book.Authors) > 0 {
				author = strings.Join(book.Authors, ", ")
			}

			// Build cover image URL
			imageUrl := ""
			if book.CoverURL != nil && *book.CoverURL != "" {
				imageUrl = "/books/" + book.Slug + "/cover"
			}

			publicBooks[i] = PublicBook{
				ID:         book.ID,
				Slug:       book.Slug,
				Title:      book.Title,
				Authors:    book.Authors,
				Author:     author,
				Categories: book.Categories,
				ImageUrl:   imageUrl,
				Short:      book.Short,
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
