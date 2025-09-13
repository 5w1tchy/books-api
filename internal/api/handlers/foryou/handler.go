package foryou

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type ForYouResponse struct {
	Status string         `json:"status"`
	Data   ForYouDataWrap `json:"data"`
}

type ForYouDataWrap struct {
	Shorts          []ShortItem `json:"shorts"`
	Recs            []any       `json:"recs"`
	Trending        []BookLite  `json:"trending"`
	New             []BookLite  `json:"new"`
	MostViewed      []BookLite  `json:"most_viewed"`
	ContinueReading []any       `json:"continue_reading"`
}

type ShortItem struct {
	Content string    `json:"content"`
	Book    ShortBook `json:"book"`
}

type ShortBook struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Title  string `json:"title"`
	Author string `json:"author"`
	URL    string `json:"url"`
}

type BookLite struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Title  string `json:"title"`
	Author string `json:"author"`
	URL    string `json:"url"`
}

func Handler(db *sql.DB, rdb *redis.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		fields := strings.TrimSpace(r.URL.Query().Get("fields"))
		limit := parseLimit(r.URL.Query().Get("limit"), 8)

		want := wanted(fields) // map[string]bool

		resp := ForYouResponse{
			Status: "success",
			Data: ForYouDataWrap{
				Shorts:          []ShortItem{},
				Recs:            []any{},
				Trending:        []BookLite{},
				New:             []BookLite{},
				MostViewed:      []BookLite{},
				ContinueReading: []any{},
			},
		}

		// shorts (from books.short; non-empty only)
		if want["shorts"] {
			items, err := fetchShorts(ctx, db, limit)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			resp.Data.Shorts = items
		}

		// trending (views in last 7 days via book_view_events)
		if want["trending"] {
			items, err := fetchTrending(ctx, db, limit)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			resp.Data.Trending = items
		}

		// new (recent by created_at desc)
		if want["new"] {
			items, err := fetchNew(ctx, db, limit)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			resp.Data.New = items
		}

		// most_viewed (all-time via book_view_events)
		if want["most_viewed"] {
			items, err := fetchMostViewed(ctx, db, limit)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			resp.Data.MostViewed = items
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	})
}

func parseLimit(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 || n > 50 {
		return def
	}
	return n
}

func wanted(fieldsCSV string) map[string]bool {
	// default = all 4 blocks if fields not provided
	m := map[string]bool{
		"shorts":      true,
		"trending":    true,
		"new":         true,
		"most_viewed": true,
	}
	if strings.TrimSpace(fieldsCSV) == "" {
		return m
	}
	for k := range m {
		m[k] = false
	}
	for _, f := range strings.Split(fieldsCSV, ",") {
		k := strings.ToLower(strings.TrimSpace(f))
		if k != "" {
			m[k] = true
		}
	}
	return m
}

// ---- queries ----

func fetchShorts(ctx context.Context, db *sql.DB, limit int) ([]ShortItem, error) {
	const q = `
		SELECT id, COALESCE(slug, '') AS slug, title, author, short
		FROM books
		WHERE short IS NOT NULL AND length(trim(short)) > 0
		ORDER BY title ASC
		LIMIT $1
	`
	rows, err := db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ShortItem, 0, limit)
	for rows.Next() {
		var id, slug, title, author, content string
		if err := rows.Scan(&id, &slug, &title, &author, &content); err != nil {
			return nil, err
		}
		out = append(out, ShortItem{
			Content: content,
			Book: ShortBook{
				ID:     id,
				Slug:   slug,
				Title:  title,
				Author: author,
				URL:    "/books/" + slug,
			},
		})
	}
	return out, rows.Err()
}

func fetchTrending(ctx context.Context, db *sql.DB, limit int) ([]BookLite, error) {
	const q = `
		SELECT b.id, COALESCE(b.slug,'') AS slug, b.title, b.author
		FROM books b
		JOIN (
			SELECT book_id, COUNT(*) AS views_7d
			FROM book_view_events
			WHERE viewed_at >= now() - INTERVAL '7 days'
			GROUP BY book_id
		) v ON v.book_id = b.id
		ORDER BY v.views_7d DESC
		LIMIT $1
	`
	return scanBookLite(ctx, db, q, limit)
}

func fetchMostViewed(ctx context.Context, db *sql.DB, limit int) ([]BookLite, error) {
	const q = `
		SELECT b.id, COALESCE(b.slug,'') AS slug, b.title, b.author
		FROM books b
		JOIN (
			SELECT book_id, COUNT(*) AS views
			FROM book_view_events
			GROUP BY book_id
		) v ON v.book_id = b.id
		ORDER BY v.views DESC
		LIMIT $1
	`
	return scanBookLite(ctx, db, q, limit)
}

func fetchNew(ctx context.Context, db *sql.DB, limit int) ([]BookLite, error) {
	const q = `
		SELECT id, COALESCE(slug,'') AS slug, title, author
		FROM books
		ORDER BY created_at DESC
		LIMIT $1
	`
	return scanBookLite(ctx, db, q, limit)
}

func scanBookLite(ctx context.Context, db *sql.DB, q string, limit int) ([]BookLite, error) {
	rows, err := db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]BookLite, 0, limit)
	for rows.Next() {
		var id, slug, title, author string
		if err := rows.Scan(&id, &slug, &title, &author); err != nil {
			return nil, err
		}
		out = append(out, BookLite{
			ID:     id,
			Slug:   slug,
			Title:  title,
			Author: author,
			URL:    "/books/" + slug,
		})
	}
	return out, rows.Err()
}
