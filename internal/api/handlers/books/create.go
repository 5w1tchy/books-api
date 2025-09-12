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

	// Author (upsert) and slug (unique)
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

	// Insert book
	var bookID string
	if err := tx.QueryRowContext(
		r.Context(),
		`INSERT INTO books (title, slug, author, author_id)
		 VALUES ($1,$2,$3,$4)
		 RETURNING id`,
		title, slug, authorName, authorID,
	).Scan(&bookID); err != nil {
		if apperr.HandleDBError(w, r, err, "create failed") {
			return
		}
	}

	// Attach categories if provided
	if len(dto.CategorySlugs) > 0 {
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
	}

	if err := tx.Commit(); err != nil {
		apperr.WriteStatus(w, r, http.StatusInternalServerError, "TX commit failed", "")
		return
	}

	w.WriteHeader(http.StatusCreated)
	handleGet(db, w, r, bookID) // respond with the created resource
}
