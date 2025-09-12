package books

import (
	"database/sql"
	"net/http"

	"github.com/5w1tchy/books-api/internal/api/apperr"
)

func handleDelete(db *sql.DB, w http.ResponseWriter, r *http.Request) {
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

	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		apperr.WriteStatus(w, r, http.StatusInternalServerError, "TX begin failed", "")
		return
	}
	defer tx.Rollback()

	// Clear join rows first (FKs may already cascade; this is safe either way)
	if _, err := tx.ExecContext(r.Context(),
		`DELETE FROM book_categories WHERE book_id = $1`, bookID); err != nil {
		if apperr.HandleDBError(w, r, err, "failed to clear categories") {
			return
		}
	}

	res, err := tx.ExecContext(r.Context(),
		`DELETE FROM books WHERE id = $1`, bookID)
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
	w.WriteHeader(http.StatusNoContent)
}
