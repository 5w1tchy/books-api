package books

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/5w1tchy/books-api/internal/api/apperr"
)

func handlePatch(db *sql.DB, w http.ResponseWriter, r *http.Request) {
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

	var dto UpdateBookDTO
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&dto); err != nil {
		apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "Invalid JSON")
		return
	}
	if dto.Title == nil && dto.Author == nil && dto.CategorySlugs == nil {
		apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "no fields to update")
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

	// Build UPDATE dynamically
	set := []string{}
	args := []any{}

	if dto.Title != nil {
		t := strings.TrimSpace(*dto.Title)
		if t == "" {
			apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "title cannot be empty")
			return
		}
		set = append(set, "title = $"+strconv.Itoa(len(args)+1))
		args = append(args, t)
	}

	if dto.Author != nil {
		a := strings.TrimSpace(*dto.Author)
		if a == "" {
			apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "author cannot be empty")
			return
		}
		authorID, _, err := getOrCreateAuthor(tx, a) // keep author_id in sync
		if err != nil {
			if apperr.HandleDBError(w, r, err, "author upsert failed") {
				return
			}
		}
		set = append(set, "author = $"+strconv.Itoa(len(args)+1))
		args = append(args, a)
		set = append(set, "author_id = $"+strconv.Itoa(len(args)+1))
		args = append(args, authorID)
	}

	if len(set) > 0 {
		args = append(args, bookID)
		q := "UPDATE books SET " + strings.Join(set, ", ") + " WHERE id = $" + strconv.Itoa(len(args))
		if _, err := tx.Exec(q, args...); err != nil {
			if apperr.HandleDBError(w, r, err, "update failed") {
				return
			}
		}
	}

	// Replace categories if provided
	if dto.CategorySlugs != nil {
		slugs := dedupSlugs(*dto.CategorySlugs)
		if len(slugs) == 0 {
			apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "category_slugs cannot be empty")
			return
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
	}

	if err := tx.Commit(); err != nil {
		apperr.WriteStatus(w, r, http.StatusInternalServerError, "TX commit failed", "")
		return
	}
	handleGet(db, w, r, bookID) // we now pass the UUID key
}
