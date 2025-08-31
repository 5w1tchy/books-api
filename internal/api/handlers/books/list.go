package books

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

func handleList(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	limit := clamp(toInt(r.URL.Query().Get("limit"), defaultLimit), 1, maxLimit)
	offset := max(0, toInt(r.URL.Query().Get("offset"), 0))

	// 1) total count for pagination meta
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM books`).Scan(&total); err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	// 2) page data (lightweight list)
	const q = `
		SELECT b.id, b.short_id, b.title, a.name,
		       COALESCE(json_agg(c.slug) FILTER (WHERE c.slug IS NOT NULL), '[]')
		FROM books b
		JOIN authors a               ON a.id = b.author_id
		LEFT JOIN book_categories bc ON bc.book_id = b.id
		LEFT JOIN categories c        ON c.id = bc.category_id
		GROUP BY b.id, b.short_id, b.title, a.name
		ORDER BY b.created_at DESC
		LIMIT $1 OFFSET $2`

	rows, err := db.Query(q, limit, offset)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var out []PublicBook
	for rows.Next() {
		var pb PublicBook
		var slugsJSON []byte
		if err := rows.Scan(&pb.ID, &pb.ShortID, &pb.Title, &pb.Author, &slugsJSON); err != nil {
			http.Error(w, "DB scan error", http.StatusInternalServerError)
			return
		}
		_ = json.Unmarshal(slugsJSON, &pb.CategorySlugs)
		out = append(out, pb)
	}

	// 3) meta
	hasMore := offset+len(out) < total
	var nextOffset *int
	if hasMore {
		n := offset + limit
		nextOffset = &n
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":      "success",
		"limit":       limit,
		"offset":      offset,
		"count":       len(out),
		"total":       total,
		"has_more":    hasMore,
		"next_offset": nextOffset, // null when no more pages
		"data":        out,
	})
}

func toInt(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
