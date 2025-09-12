package books

import (
	"database/sql"
	"encoding/json"
	"net/http"

	storebooks "github.com/5w1tchy/books-api/internal/store/books"
)

type patchReq struct {
	Title         *string   `json:"title,omitempty"`
	Author        *string   `json:"author,omitempty"`
	CategorySlugs *[]string `json:"categories,omitempty"`
}

func patch(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		key := r.PathValue("key")
		if key == "" {
			http.Error(w, `{"status":"error","error":"missing key"}`, http.StatusBadRequest)
			return
		}

		var req patchReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"status":"error","error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}

		dto := storebooks.UpdateBookDTO{
			Title:         req.Title,
			Author:        req.Author,
			CategorySlugs: req.CategorySlugs,
		}
		b, err := storebooks.Patch(r.Context(), db, key, dto)
		if err == sql.ErrNoRows {
			http.Error(w, `{"status":"error","error":"not found"}`, http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, `{"status":"error","error":"failed to patch"}`, http.StatusInternalServerError)
			return
		}

		resp := struct {
			Status string                `json:"status"`
			Data   storebooks.PublicBook `json:"data"`
		}{"success", b}
		_ = json.NewEncoder(w).Encode(resp)
	}
}
