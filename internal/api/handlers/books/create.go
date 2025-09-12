package books

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	storebooks "github.com/5w1tchy/books-api/internal/store/books"
)

type createReq struct {
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	CategorySlugs []string `json:"categories,omitempty"`
}

func create(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		var req createReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"status":"error","error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}

		req.Title = strings.TrimSpace(req.Title)
		req.Author = strings.TrimSpace(req.Author)
		if req.Title == "" || req.Author == "" {
			http.Error(w, `{"status":"error","error":"title and author are required"}`, http.StatusBadRequest)
			return
		}

		dto := storebooks.CreateBookDTO{
			Title:         req.Title,
			Author:        req.Author,
			CategorySlugs: req.CategorySlugs,
		}
		book, err := storebooks.Create(r.Context(), db, dto)
		if err != nil {
			http.Error(w, `{"status":"error","error":"failed to create book"}`, http.StatusInternalServerError)
			return
		}

		resp := struct {
			Status string                `json:"status"`
			Data   storebooks.PublicBook `json:"data"`
		}{
			Status: "success",
			Data:   book,
		}
		enc := json.NewEncoder(w)
		_ = enc.Encode(resp)
	}
}
