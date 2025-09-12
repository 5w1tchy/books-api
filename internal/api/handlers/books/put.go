package books

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/5w1tchy/books-api/internal/api/apperr"
)

func handlePut(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	defer r.Body.Close()

	key := r.PathValue("key")
	if key == "" {
		apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "missing book key")
		return
	}
	if !isUUID(key) {
		apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "id must be a UUID")
		return
	}
	bookID := key

	var dto CreateBookDTO
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&dto); err != nil {
		apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "invalid JSON")
		return
	}
	title := strings.TrimSpace(dto.Title)
	authorName := strings.TrimSpace(dto.Author)
	if title == "" || authorName == "" {
		apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "title and author are required")
		return
	}

	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		apperr.WriteStatus(w, r, http.StatusInternalServerError, "TX begin failed", "")
		return
	}
	defer tx.Rollback()

	// 404 early
	var exists bool
	if err := tx.QueryRowContext(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM books WHERE id = $1)`, bookID).Scan(&exists); err != nil {
		apperr.WriteStatus(w, r, http.StatusInternalServerError, "lookup failed", "")
		return
	}
	if !exists {
		apperr.WriteStatus(w, r, http.StatusNotFound, "Not Found", "Book not found")
		return
	}

	authorID, _, err := getOrCreateAuthor(tx, authorName)
	if err != nil {
		if apperr.HandleDBError(w, r, err, "author upsert failed") {
			return
		}
	}
	base := slugify(title)
	slug, err := ensureUniqueSlug(tx, "books", "slug", base, 20)
	if err != nil {
		if apperr.HandleDBError(w, r, err, "slug generation failed") {
			return
		}
	}

	// Replace book basics
	if _, err := tx.ExecContext(r.Context(),
		`UPDATE books
		   SET title = $1, slug = $2, author = $3, author_id = $4
		 WHERE id = $5`,
		title, slug, authorName, authorID, bookID); err != nil {
		if apperr.HandleDBError(w, r, err, "update failed") {
			return
		}
	}

	// Replace categories
	if _, err := tx.ExecContext(r.Context(),
		`DELETE FROM book_categories WHERE book_id = $1`, bookID); err != nil {
		if apperr.HandleDBError(w, r, err, "failed to clear categories") {
			return
		}
	}
	slugs := dedupSlugs(dto.CategorySlugs)
	for _, s := range slugs {
		res, err := tx.ExecContext(r.Context(), `
			INSERT INTO book_categories (book_id, category_id)
			SELECT $1, c.id FROM categories c WHERE c.slug = $2
		`, bookID, s)
		if err != nil {
			if apperr.HandleDBError(w, r, err, "attach category failed") {
				return
			}
		}
		if n, _ := res.RowsAffected(); n == 0 {
			apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "unknown category slug: "+s)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		apperr.WriteStatus(w, r, http.StatusInternalServerError, "TX commit failed", "")
		return
	}
	handleGet(db, w, r, bookID)
}
