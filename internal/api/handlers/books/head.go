package books

import (
	"database/sql"
	"net/http"

	storebooks "github.com/5w1tchy/books-api/internal/store/books"
)

func head(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		key := r.PathValue("key")
		if key == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ok, err := storebooks.ExistsByKey(r.Context(), db, key)
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
}
