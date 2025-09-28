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

type adminReplaceReq struct {
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	CategorySlugs []string `json:"categories,omitempty"`
}

// AdminPut: PUT /admin/books/{key}
func AdminPut(db *sql.DB, rdb *redis.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		key := r.PathValue("key")
		if key == "" {
			http.Error(w, `{"status":"error","error":"missing key"}`, http.StatusBadRequest)
			return
		}

		var req adminReplaceReq
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
		b, err := storebooks.Replace(r.Context(), db, key, dto)
		if err == sql.ErrNoRows {
			http.Error(w, `{"status":"error","error":"not found"}`, http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, `{"status":"error","error":"failed to replace"}`, http.StatusInternalServerError)
			return
		}

		if err := storeforyou.BumpVersion(r.Context(), rdb); err != nil {
			log.Printf("[for-you] bump version failed: %v", err)
		}

		resp := struct {
			Status string                `json:"status"`
			Data   storebooks.PublicBook `json:"data"`
		}{"success", b}
		_ = json.NewEncoder(w).Encode(resp)
	})
}
