package books

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

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

		key := r.PathValue("key")
		if key == "" {
			http.Error(w, `{"status":"error","error":"missing key"}`, http.StatusBadRequest)
			return
		}

		// Load the public book (existing logic)
		b, err := storebooks.FetchByKey(r.Context(), db, key)
		if err == sql.ErrNoRows {
			http.Error(w, `{"status":"error","error":"not found"}`, http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, `{"status":"error","error":"failed to fetch"}`, http.StatusInternalServerError)
			return
		}

		// Record a view (bounded worker; non-blocking)
		if b.ID != "" {
			viewqueue.Enqueue(b.ID)
		}

		// Fetch long-form content (summary + coda), but NOT short
		summary, coda, _ := fetchContentByID(r.Context(), db, b.ID)

		resp := struct {
			Status string          `json:"status"`
			Data   bookWithContent `json:"data"`
		}{
			Status: "success",
			Data: bookWithContent{
				PublicBook: b,
				Summary:    summary,
				Coda:       coda,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// fetchContentByID gets summary & coda from books table by UUID (best-effort).
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

	err := db.QueryRowContext(cctx,
		`SELECT summary, coda FROM books WHERE id = $1`, id,
	).Scan(&r.summary, &r.coda)
	if err != nil {
		// If not found or columns absent, just return nils (don’t break the endpoint)
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
