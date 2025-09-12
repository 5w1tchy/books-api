package books

import (
	"database/sql"
	"net/http"

	"github.com/5w1tchy/books-api/internal/repo/booksrepo"
)

func handleHead(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		// Not routed in your mux; treat as not found.
		w.WriteHeader(http.StatusNotFound)
		return
	}
	ok, err := booksrepo.ExistsByKey(r.Context(), db, key)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}
