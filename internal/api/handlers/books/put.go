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

	idPart := strings.Trim(strings.TrimPrefix(r.URL.Path, "/books/"), "/")
	if idPart == "" {
		apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "missing book key")
		return
	}
	if !isUUID(idPart) {
		apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "id must be a UUID")
		return
	}
	bookID := idPart

	var dto CreateBookDTO // full replacement
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&dto); err != nil {
		apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "Invalid JSON")
		return
	}
	if strings.TrimSpace(dto.Title) == "" ||
		strings.TrimSpace(dto.Author) == "" ||
		len(dto.CategorySlugs) == 0 {
		apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "title, author, category_slugs are required")
		return
	}
	slugs := dedupSlugs(dto.CategorySlugs)
	if len(slugs) == 0 {
		apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "category_slugs cannot be empty")
		return
	}

	tx, err := db.Begin()
	if err != nil {
		apperr.WriteStatus(w, r, http.StatusInternalServerError, "TX begin failed", "")
		return
	}
	defer tx.Rollback()

	// Early existence check (clean 404)
	var exists bool
	if err := tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM books WHERE id = $1)`, bookID).Scan(&exists); err != nil {
		apperr.WriteStatus(w, r, http.StatusInternalServerError, "lookup failed", "")
		return
	}
	if !exists {
		apperr.WriteStatus(w, r, http.StatusNotFound, "Not Found", "Book not found")
		return
	}

	authorID, _, err := getOrCreateAuthor(tx, dto.Author)
	if err != nil {
		if apperr.HandleDBError(w, r, err, "author upsert failed") {
			return
		}
	}

	if _, err := tx.Exec(`
		UPDATE books
		SET title = $1, author = $2, author_id = $3
		WHERE id = $4
	`, dto.Title, dto.Author, authorID, bookID); err != nil {
		if apperr.HandleDBError(w, r, err, "update failed") {
			return
		}
	}

	if _, err := tx.Exec(`DELETE FROM book_categories WHERE book_id = $1`, bookID); err != nil {
		if apperr.HandleDBError(w, r, err, "failed to clear categories") {
			return
		}
	}
	for _, s := range slugs {
		res, err := tx.Exec(`
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
