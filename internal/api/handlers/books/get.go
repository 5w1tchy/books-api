package books

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
)

func handleGet(db *sql.DB, w http.ResponseWriter, r *http.Request, key string) {
	w.Header().Set("Content-Type", "application/json")

	cond, arg := resolveBookKey(key)

	// NOTE: var (not const) because we concatenate cond.
	qOne := `
	SELECT
		b.id, b.short_id, b.slug, b.title, a.name,
		COALESCE(json_agg(c.slug) FILTER (WHERE c.slug IS NOT NULL), '[]'),
		COALESCE(bo.summary, ''), COALESCE(bo.short, ''), COALESCE(bo.coda, '')
	FROM books b
	JOIN authors a               ON a.id = b.author_id
	LEFT JOIN book_categories bc ON bc.book_id = b.id
	LEFT JOIN categories c        ON c.id = bc.category_id
	LEFT JOIN book_outputs bo     ON bo.book_id = b.id
	WHERE ` + cond + `
	GROUP BY b.id, b.short_id, b.slug, b.title, a.name, bo.summary, bo.short, bo.coda`

	var pb PublicBook
	var slugsJSON []byte
	if err := db.QueryRow(qOne, arg).
		Scan(&pb.ID, &pb.ShortID, &pb.Slug, &pb.Title, &pb.Author, &slugsJSON, &pb.Summary, &pb.Short, &pb.Coda); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Book not found", http.StatusNotFound)
			return
		}
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	_ = json.Unmarshal(slugsJSON, &pb.CategorySlugs)
	pb.URL = "/books/" + pb.Slug

	// Optional: fields=summary,short,coda to trim heavy fields
	if f := strings.TrimSpace(r.URL.Query().Get("fields")); f != "" {
		fields := strings.Split(f, ",")
		keep := func(s string) bool { return slices.Contains(fields, s) }
		if !keep("summary") {
			pb.Summary = ""
		}
		if !keep("short") {
			pb.Short = ""
		}
		if !keep("coda") {
			pb.Coda = ""
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": pb})
}
