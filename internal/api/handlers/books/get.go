package books

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/5w1tchy/books-api/internal/api/apperr"
)

func handleGet(db *sql.DB, w http.ResponseWriter, r *http.Request, key string) {
	w.Header().Set("Content-Type", "application/json")

	cond, arg := resolveBookKey(key)
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
	if err := db.QueryRowContext(r.Context(), qOne, arg).
		Scan(&pb.ID, &pb.ShortID, &pb.Slug, &pb.Title, &pb.Author, &slugsJSON, &pb.Summary, &pb.Short, &pb.Coda); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apperr.WriteStatus(w, r, http.StatusNotFound, "Not Found", "Book not found")
			return
		}
		apperr.WriteStatus(w, r, http.StatusInternalServerError, "DB error", "Failed to fetch book")
		return
	}
	_ = json.Unmarshal(slugsJSON, &pb.CategorySlugs)
	pb.URL = "/books/" + pb.Slug

	// fields=summary,short,coda (case/space-insensitive)
	if raw := strings.TrimSpace(r.URL.Query().Get("fields")); raw != "" {
		keep := map[string]struct{}{}
		for _, f := range strings.Split(raw, ",") {
			f = strings.ToLower(strings.TrimSpace(f))
			if f != "" {
				keep[f] = struct{}{}
			}
		}
		if _, ok := keep["summary"]; !ok {
			pb.Summary = ""
		}
		if _, ok := keep["short"]; !ok {
			pb.Short = ""
		}
		if _, ok := keep["coda"]; !ok {
			pb.Coda = ""
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": pb})
}
