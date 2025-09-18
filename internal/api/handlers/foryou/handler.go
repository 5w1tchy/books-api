package foryou

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
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
		// top-level guard (kept at 2s)
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		fields := strings.TrimSpace(r.URL.Query().Get("fields"))
		limit := parseLimit(r.URL.Query().Get("limit"), 8)

		// DEBUG (you can remove later)
		log.Printf("[for-you] raw=%q fields=%q", r.URL.RawQuery, fields)

		want := wanted(fields)

		// DEBUG (you can remove later)
		log.Printf("[for-you] want: shorts=%t trending=%t new=%t most_viewed=%t",
			want["shorts"], want["trending"], want["new"], want["most_viewed"])

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

		bt := blockTimeout()

		if want["shorts"] {
			resp.Data.Shorts = isolateBlock(ctx, "shorts", bt, func(c context.Context) ([]ShortItem, error) {
				return fetchShorts(c, db, limit)
			})
		}

		if want["trending"] {
			resp.Data.Trending = isolateBlock(ctx, "trending", bt, func(c context.Context) ([]BookLite, error) {
				return fetchTrending(c, db, limit)
			})
		}

		if want["new"] {
			resp.Data.New = isolateBlock(ctx, "new", bt, func(c context.Context) ([]BookLite, error) {
				return fetchNew(c, db, limit)
			})
		}

		if want["most_viewed"] {
			resp.Data.MostViewed = isolateBlock(ctx, "most_viewed", bt, func(c context.Context) ([]BookLite, error) {
				return fetchMostViewed(c, db, limit)
			})
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

// from book_outputs.short joined to books; COALESCE all string columns
func fetchShorts(ctx context.Context, db *sql.DB, limit int) ([]ShortItem, error) {
	const q = `
		SELECT
			b.id,
			COALESCE(b.slug,'')   AS slug,
			COALESCE(b.title,'')  AS title,
			COALESCE(b.author,'') AS author,
			bo.short
		FROM book_outputs bo
		JOIN books b ON b.id = bo.book_id
		WHERE bo.short IS NOT NULL AND length(trim(bo.short)) > 0
		ORDER BY bo.created_at DESC
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
		SELECT
			b.id,
			COALESCE(b.slug,'')   AS slug,
			COALESCE(b.title,'')  AS title,
			COALESCE(b.author,'') AS author
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
		SELECT
			b.id,
			COALESCE(b.slug,'')   AS slug,
			COALESCE(b.title,'')  AS title,
			COALESCE(b.author,'') AS author
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
		SELECT
			id,
			COALESCE(slug,'')   AS slug,
			COALESCE(title,'')  AS title,
			COALESCE(author,'') AS author
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
