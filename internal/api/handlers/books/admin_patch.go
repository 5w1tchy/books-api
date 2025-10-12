package books

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	storebooks "github.com/5w1tchy/books-api/internal/store/books"
	storeforyou "github.com/5w1tchy/books-api/internal/store/foryou"
	"github.com/redis/go-redis/v9"
)

type adminPatchReq struct {
	Coda       *string   `json:"coda,omitempty"`
	Title      *string   `json:"title,omitempty"`
	Authors    *[]string `json:"authors,omitempty"`
	Categories *[]string `json:"categories,omitempty"`
	Short      *string   `json:"short,omitempty"`
	Summary    *string   `json:"summary,omitempty"`
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

		// You'll need to implement this in sql_v2.go
		b, err := storebooks.PatchV2(r.Context(), db, key, storebooks.UpdateBookV2DTO{
			Coda:       req.Coda,
			Title:      req.Title,
			Authors:    req.Authors,
			Categories: req.Categories,
			Short:      req.Short,
			Summary:    req.Summary,
		})
		if err == sql.ErrNoRows {
			http.Error(w, `{"status":"error","error":"not found"}`, http.StatusNotFound)
			return
		} else if err != nil {
			log.Printf("[admin_patch] error while patching book %s: %v", key, err)
			http.Error(w, fmt.Sprintf(`{"status":"error","error":"%v"}`, err), http.StatusInternalServerError)
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
