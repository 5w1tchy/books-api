package books

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/5w1tchy/books-api/internal/api/apperr"
	"github.com/5w1tchy/books-api/internal/repo/booksrepo"
	"github.com/5w1tchy/books-api/internal/validate"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

func handleList(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	limit, offset := validate.ClampLimitOffset(
		r.URL.Query().Get("limit"),
		r.URL.Query().Get("offset"),
		defaultLimit, maxLimit,
	)

	f := booksrepo.ListFilters{
		Q:          strings.TrimSpace(r.URL.Query().Get("q")),
		Author:     strings.TrimSpace(r.URL.Query().Get("author")),
		Match:      validate.ParseMatch(strings.TrimSpace(r.URL.Query().Get("match"))),
		Limit:      limit,
		Offset:     offset,
		Categories: validate.ParseCategoriesCSV(r.URL.Query().Get("categories")),
	}
	if f.Q != "" {
		f.MinSim = validate.ParseMinSim(f.Q, r.URL.Query().Get("min_sim"))
	}

	books, total, err := booksrepo.List(r.Context(), db, f)
	if err != nil {
		apperr.WriteStatus(w, r, http.StatusInternalServerError, "DB error", "Failed to list books")
		return
	}

	hasMore := offset+len(books) < total
	var nextOffset *int
	if hasMore {
		n := offset + limit
		nextOffset = &n
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":      "success",
		"limit":       limit,
		"offset":      offset,
		"count":       len(books),
		"total":       total,
		"has_more":    hasMore,
		"next_offset": nextOffset,
		"data":        books,
	})
}
