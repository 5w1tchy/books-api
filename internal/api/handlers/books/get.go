package books

import (
	"database/sql"
	"encoding/json"
	"net/http"

	storebooks "github.com/5w1tchy/books-api/internal/store/books"
)

func get(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		key := r.PathValue("key")
		if key == "" {
			http.Error(w, `{"status":"error","error":"missing key"}`, http.StatusBadRequest)
			return
		}

		b, err := storebooks.FetchByKey(r.Context(), db, key)
		if err == sql.ErrNoRows {
			http.Error(w, `{"status":"error","error":"not found"}`, http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, `{"status":"error","error":"failed to fetch"}`, http.StatusInternalServerError)
			return
		}

		resp := struct {
			Status string                `json:"status"`
			Data   storebooks.PublicBook `json:"data"`
		}{"success", b}
		_ = json.NewEncoder(w).Encode(resp)
	}
}
