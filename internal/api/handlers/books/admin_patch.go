package books

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	storebooks "github.com/5w1tchy/books-api/internal/store/books"
	storeforyou "github.com/5w1tchy/books-api/internal/store/foryou"
	"github.com/redis/go-redis/v9"
)

type adminPatchReq struct {
	Title         *string   `json:"title,omitempty"`
	Author        *string   `json:"author,omitempty"`
	CategorySlugs *[]string `json:"categories,omitempty"`
}

// AdminPatch: PATCH /admin/books/{key}
func AdminPatch(db *sql.DB, rdb *redis.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		var req adminPatchReq
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
