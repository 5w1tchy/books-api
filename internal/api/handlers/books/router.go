package books

import (
	"database/sql"
	"net/http"

	"github.com/5w1tchy/books-api/internal/api/apperr"
)

const allowBooks = "GET, POST, PATCH, PUT, DELETE, OPTIONS, HEAD"

func Handler(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if key := r.PathValue("key"); key != "" {
				handleGet(db, w, r, key)
				return
			}
			handleList(db, w, r)

		case http.MethodHead:
			handleHead(db, w, r)

		case http.MethodPost:
			handleCreate(db, w, r)

		case http.MethodPatch:
			if r.PathValue("key") == "" {
				w.Header().Set("Allow", allowBooks)
				apperr.WriteStatus(w, r, http.StatusMethodNotAllowed, "Method Not Allowed", "PATCH requires /books/{key}")
				return
			}
			handlePatch(db, w, r)

		case http.MethodPut:
			if r.PathValue("key") == "" {
				w.Header().Set("Allow", allowBooks)
				apperr.WriteStatus(w, r, http.StatusMethodNotAllowed, "Method Not Allowed", "PUT requires /books/{key}")
				return
			}
			handlePut(db, w, r)

		case http.MethodDelete:
			if r.PathValue("key") == "" {
				w.Header().Set("Allow", allowBooks)
				apperr.WriteStatus(w, r, http.StatusMethodNotAllowed, "Method Not Allowed", "DELETE requires /books/{key}")
				return
			}
			handleDelete(db, w, r)

		case http.MethodOptions:
			w.Header().Set("Allow", allowBooks)
			w.WriteHeader(http.StatusNoContent)

		default:
			w.Header().Set("Allow", allowBooks)
			apperr.WriteStatus(w, r, http.StatusMethodNotAllowed, "Method Not Allowed", "")
		}
	})
}
