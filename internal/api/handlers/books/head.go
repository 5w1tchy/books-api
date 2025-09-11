package books

import (
	"database/sql"
	"net/http"
	"strings"
)

// HEAD semantics:
// - /books/      → 200 (collection exists)
// - /books/{key} → 200 if exists, 404 if not (no body)
func handleHead(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	idPart := strings.Trim(strings.TrimPrefix(r.URL.Path, "/books/"), "/")
	if idPart == "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	cond, arg := resolveBookKey(idPart)

	var exists bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM books b WHERE `+cond+`)`, arg).Scan(&exists); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}
