package books

import (
	"database/sql"
	"net/http"
	"strings"
)

func Handler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			idPart := strings.Trim(strings.TrimPrefix(r.URL.Path, "/books/"), "/")
			if idPart == "" {
				handleList(db, w, r)
				return
			}
			handleGet(db, w, r, idPart)
		case http.MethodPost:
			handleCreate(db, w, r)
		case http.MethodPatch:
			handlePatch(db, w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
