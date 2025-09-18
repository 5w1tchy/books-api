package books

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	storebooks "github.com/5w1tchy/books-api/internal/store/books"
	storeforyou "github.com/5w1tchy/books-api/internal/store/foryou"
	"github.com/redis/go-redis/v9"
)

type createReq struct {
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	CategorySlugs []string `json:"categories,omitempty"`
}

func create(db *sql.DB, rdb *redis.Client) http.HandlerFunc {
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

		// Best-effort global cache bust for /for-you
		if err := storeforyou.BumpVersion(r.Context(), rdb); err != nil {
			log.Printf("[for-you] bump version failed: %v", err)
		}

		resp := struct {
			Status string                `json:"status"`
			Data   storebooks.PublicBook `json:"data"`
		}{
			Status: "success",
			Data:   book,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}
