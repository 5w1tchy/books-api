package books

import (
	"database/sql"
	"net/http"

	storebooks "github.com/5w1tchy/books-api/internal/store/books"
)

func del(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		key := r.PathValue("key")
		if key == "" {
			http.Error(w, `{"status":"error","error":"missing key"}`, http.StatusBadRequest)
			return
		}

		if err := storebooks.Delete(r.Context(), db, key); err == sql.ErrNoRows {
			http.Error(w, `{"status":"error","error":"not found"}`, http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, `{"status":"error","error":"failed to delete"}`, http.StatusInternalServerError)
			return
		}

		// No response body on successful delete.
		w.WriteHeader(http.StatusNoContent)
	}
}
