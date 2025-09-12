package books

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/5w1tchy/books-api/internal/api/apperr"
	"github.com/5w1tchy/books-api/internal/repo/booksrepo"
	"github.com/5w1tchy/books-api/internal/validate"
)

func handleGet(db *sql.DB, w http.ResponseWriter, r *http.Request, key string) {
	w.Header().Set("Content-Type", "application/json")

	pb, err := booksrepo.FetchByKey(r.Context(), db, key)
	if err != nil {
		if errors.Is(err, booksrepo.ErrNotFound) {
			apperr.WriteStatus(w, r, http.StatusNotFound, "Not Found", "Book not found")
			return
		}
		apperr.WriteStatus(w, r, http.StatusInternalServerError, "DB error", "Failed to fetch book")
		return
	}

	keep := validate.ParseFields(r.URL.Query().Get("fields"), []string{"summary", "short", "coda"})
	if _, ok := keep["summary"]; !ok {
		pb.Summary = ""
	}
	if _, ok := keep["short"]; !ok {
		pb.Short = ""
	}
	if _, ok := keep["coda"]; !ok {
		pb.Coda = ""
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": pb})
}
