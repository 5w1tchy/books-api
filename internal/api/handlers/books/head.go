package books

import (
	"database/sql"
	"net/http"
)

func handleHead(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	cond, arg := resolveBookKey(key)

	var exists bool
	if err := db.QueryRowContext(
		r.Context(),
		`SELECT EXISTS(
		   SELECT 1 FROM books b
		   JOIN authors a ON a.id = b.author_id
		   WHERE `+cond+`)`,
		arg,
	).Scan(&exists); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}
