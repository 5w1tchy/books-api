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
	Coda       string   `json:"coda,omitempty"`
	Title      string   `json:"title"`
	Authors    []string `json:"authors"`    // Changed from single Author
	Categories []string `json:"categories"` // Changed from CategorySlugs
	Short      string   `json:"short,omitempty"`
	Summary    string   `json:"summary,omitempty"`
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

		// Validation similar to create
		req.Title = strings.TrimSpace(req.Title)
		if req.Title == "" {
			http.Error(w, `{"status":"error","error":"title is required"}`, http.StatusBadRequest)
			return
		}
		if len(req.Authors) == 0 {
			http.Error(w, `{"status":"error","error":"at least one author is required"}`, http.StatusBadRequest)
			return
		}
		if len(req.Categories) == 0 {
			http.Error(w, `{"status":"error","error":"at least one category is required"}`, http.StatusBadRequest)
			return
		}

		dto := storebooks.CreateBookV2DTO{
			Code:       req.Coda,
			Title:      req.Title,
			Authors:    req.Authors,
			Categories: req.Categories,
			Short:      req.Short,
			Summary:    req.Summary,
		}

		// You'll need to implement this in sql_v2.go
		b, err := storebooks.ReplaceV2(r.Context(), db, key, dto)
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
			Status string               `json:"status"`
			Data   storebooks.AdminBook `json:"data"`
		}{"success", b}
		_ = json.NewEncoder(w).Encode(resp)
	})
}
