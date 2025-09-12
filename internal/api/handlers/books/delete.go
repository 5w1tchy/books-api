package books

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/5w1tchy/books-api/internal/api/apperr"
	"github.com/5w1tchy/books-api/internal/repo/booksrepo"
)

func handleDelete(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("key")
	if id == "" || !isUUID(id) {
		apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "id must be a UUID")
		return
	}

	if err := booksrepo.Delete(r.Context(), db, id); err != nil {
		if errors.Is(err, booksrepo.ErrNotFound) {
			apperr.WriteStatus(w, r, http.StatusNotFound, "Not Found", "Book not found")
			return
		}
		apperr.WriteStatus(w, r, http.StatusInternalServerError, "DB error", "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
