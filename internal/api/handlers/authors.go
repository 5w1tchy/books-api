package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
)

type Author struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// /authors or /authors/{slug}
func AuthorsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		slug := strings.Trim(strings.TrimPrefix(r.URL.Path, "/authors/"), "/")

		// List all authors
		if slug == "" {
			rows, err := db.Query(`
				SELECT a.id, a.name, a.slug, COUNT(b.id) AS books_count
				FROM authors a
				LEFT JOIN books b ON b.author_id = a.id
				GROUP BY a.id
				ORDER BY a.name`)
			if err != nil {
				http.Error(w, "DB error", 500)
				return
			}
			defer rows.Close()

			var out []map[string]any
			for rows.Next() {
				var a Author
				var count int
				if err := rows.Scan(&a.ID, &a.Name, &a.Slug, &count); err != nil {
					http.Error(w, "DB scan error", 500)
					return
				}
				out = append(out, map[string]any{
					"id": a.ID, "name": a.Name, "slug": a.Slug, "books_count": count,
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "count": len(out), "data": out})
			return
		}

		// Author hub: list books by slug
		rows, err := db.Query(`
			SELECT b.id, b.short_id, b.title, b.slug
			FROM authors a
			JOIN books b ON b.author_id = a.id
			WHERE a.slug = $1
			ORDER BY b.created_at DESC`, slug)
		if err != nil {
			http.Error(w, "DB error", 500)
			return
		}
		defer rows.Close()

		var books []map[string]any
		for rows.Next() {
			var id string
			var shortID int64
			var title, bslug string
			if err := rows.Scan(&id, &shortID, &title, &bslug); err != nil {
				http.Error(w, "DB scan error", 500)
				return
			}
			books = append(books, map[string]any{
				"id": id, "short_id": shortID, "title": title, "slug": bslug,
				"url": "/books/" + bslug,
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "author_slug": slug, "count": len(books), "data": books})
	}
}
