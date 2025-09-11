package books

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/5w1tchy/books-api/internal/api/apperr"
)

func handleDelete(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	// TODO(auth): require admin role here once auth is wired.
	w.Header().Set("Content-Type", "application/json")

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

	tx, err := db.Begin()
	if err != nil {
		apperr.WriteStatus(w, r, http.StatusInternalServerError, "TX begin failed", "")
		return
	}
	defer tx.Rollback()

	// Clear joins first (safe if none)
	if _, err := tx.Exec(`DELETE FROM book_categories WHERE book_id = $1`, bookID); err != nil {
		if apperr.HandleDBError(w, r, err, "failed to clear categories") {
			return
		}
	}

	// Delete the book (404 if not found)
	res, err := tx.Exec(`DELETE FROM books WHERE id = $1`, bookID)
	if err != nil {
		if apperr.HandleDBError(w, r, err, "delete failed") {
			return
		}
	}
	if n, _ := res.RowsAffected(); n == 0 {
		apperr.WriteStatus(w, r, http.StatusNotFound, "Not Found", "Book not found")
		return
	}

	if err := tx.Commit(); err != nil {
		apperr.WriteStatus(w, r, http.StatusInternalServerError, "TX commit failed", "")
		return
	}
	w.WriteHeader(http.StatusNoContent) // 204
}
