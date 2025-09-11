package books

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/5w1tchy/books-api/internal/api/apperr"
)

func handleCreate(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	defer r.Body.Close()

	var dto CreateBookDTO
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&dto); err != nil {
		apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "Invalid JSON")
		return
	}
	dto.Title = strings.TrimSpace(dto.Title)
	dto.Author = strings.TrimSpace(dto.Author)
	if dto.Title == "" || dto.Author == "" || len(dto.CategorySlugs) == 0 {
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

	authorID, authorSlug, err := getOrCreateAuthor(tx, dto.Author)
	if err != nil {
		if apperr.HandleDBError(w, r, err, "author upsert failed") {
			return
		}
	}

	// Stable slug (no auto-regen on future title changes)
	base := slugify(dto.Title)
	bookSlug, err := ensureUniqueSlug(tx, "books", "slug", base, 50)
	if err != nil {
		if apperr.HandleDBError(w, r, err, "slug generation failed") {
			return
		}
	}

	var id string
	var shortID int64
	if err := tx.QueryRow(`
		INSERT INTO books (title, author, author_id, slug)
		VALUES ($1, $2, $3, $4)
		RETURNING id, short_id
	`, dto.Title, dto.Author, authorID, bookSlug).Scan(&id, &shortID); err != nil {
		if apperr.HandleDBError(w, r, err, "insert failed") {
			return
		}
	}

	for _, s := range slugs {
		res, err := tx.Exec(`
			INSERT INTO book_categories (book_id, category_id)
			SELECT $1, c.id FROM categories c WHERE c.slug = $2
		`, id, s)
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

	// Return minimal success payload incl. slug + url (frontend-friendly)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":      "success",
		"id":          id,
		"short_id":    shortID,
		"slug":        bookSlug,
		"author_slug": authorSlug,
		"url":         "/books/" + bookSlug,
	})
}
