package books

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/5w1tchy/books-api/internal/api/apperr"
)

const allowBooks = "GET, POST, PATCH, PUT, DELETE, OPTIONS, HEAD"

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

		case http.MethodHead:
			handleHead(db, w, r)

		case http.MethodPost:
			handleCreate(db, w, r)

		case http.MethodPatch:
			handlePatch(db, w, r)

		case http.MethodPut:
			handlePut(db, w, r)

		case http.MethodDelete:
			handleDelete(db, w, r)

		default:
			w.Header().Set("Allow", allowBooks)
			apperr.WriteStatus(w, r, http.StatusMethodNotAllowed, "Method Not Allowed", "")
		}
	}
}
