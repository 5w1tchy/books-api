package books

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/5w1tchy/books-api/internal/metrics/viewqueue"
	storebooks "github.com/5w1tchy/books-api/internal/store/books"
)

type bookWithContent struct {
	storebooks.PublicBook
	Summary *string `json:"summary,omitempty"`
	Coda    *string `json:"coda,omitempty"`
}

func Get(db *sql.DB) http.HandlerFunc {
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

		// Use summary/coda from the books table directly
		var sumPtr, codaPtr *string
		if b.Summary != "" {
			s := b.Summary
			sumPtr = &s
		}
		if b.Coda != "" {
			c := b.Coda
			codaPtr = &c
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
