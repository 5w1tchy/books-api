package books

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/5w1tchy/books-api/internal/api/middlewares"
	"github.com/5w1tchy/books-api/internal/metrics/viewqueue"
	storebooks "github.com/5w1tchy/books-api/internal/store/books"
)

type bookWithContent struct {
	storebooks.PublicBook
	Summary *string `json:"summary,omitempty"`
	Coda    *string `json:"coda,omitempty"`
}

func get(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		// ADD AUTHENTICATION REQUIREMENT HERE
		_, isAuth := middlewares.UserIDFrom(r.Context())
		if !isAuth {
			http.Error(w, `{"status":"error","error":"login required to read books"}`, http.StatusUnauthorized)
			return
		}

		key := r.PathValue("key")
		if key == "" {
			http.Error(w, `{"status":"error","error":"missing key"}`, http.StatusBadRequest)
			return
		}

		// ... rest of your existing code stays the same
		// Load the public book
		b, err := storebooks.FetchByKey(r.Context(), db, key)
		if err == sql.ErrNoRows {
			http.Error(w, `{"status":"error","error":"not found"}`, http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, `{"status":"error","error":"failed to fetch"}`, http.StatusInternalServerError)
			return
		}

		// Record a view (non-blocking)
		if b.ID != "" {
			viewqueue.Enqueue(b.ID)
		}

		// We do NOT want short on the book page
		b.Short = ""

		// Prefer summary/coda from store; fallback to content fetch if missing
		var sumPtr, codaPtr *string
		if b.Summary != "" {
			s := b.Summary
			sumPtr = &s
		}
		if b.Coda != "" {
			c := b.Coda
			codaPtr = &c
		}
		if sumPtr == nil || codaPtr == nil {
			fs, fc, _ := fetchContentByID(r.Context(), db, b.ID)
			if sumPtr == nil {
				sumPtr = fs
			}
			if codaPtr == nil {
				codaPtr = fc
			}
		}

		resp := struct {
			Status string          `json:"status"`
			Data   bookWithContent `json:"data"`
		}{
			Status: "success",
			Data: bookWithContent{
				PublicBook: b,
				Summary:    sumPtr,
				Coda:       codaPtr,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// fetchContentByID gets latest summary & coda from book_outputs by book_id (best-effort).
func fetchContentByID(ctx context.Context, db *sql.DB, id string) (*string, *string, error) {
	if id == "" {
		return nil, nil, nil
	}

	type row struct {
		summary sql.NullString
		coda    sql.NullString
	}

	var r row
	cctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	err := db.QueryRowContext(cctx, `
		SELECT o.summary, o.coda
		FROM book_outputs o
		WHERE o.book_id = $1
		ORDER BY o.created_at DESC
		LIMIT 1
	`, id).Scan(&r.summary, &r.coda)
	if err != nil {
		// If not found, just return nils (don’t break the endpoint)
		return nil, nil, nil
	}

	var sPtr, cPtr *string
	if r.summary.Valid {
		s := r.summary.String
		sPtr = &s
	}
	if r.coda.Valid {
		c := r.coda.String
		cPtr = &c
	}
	return sPtr, cPtr, nil
}
