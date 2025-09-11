package books

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/5w1tchy/books-api/internal/api/apperr"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

func handleList(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// pagination
	limit := clamp(toInt(r.URL.Query().Get("limit"), defaultLimit), 1, maxLimit)
	offset := max(0, toInt(r.URL.Query().Get("offset"), 0))

	// filters
	q := strings.TrimSpace(r.URL.Query().Get("q"))           // free-text (fuzzy)
	author := strings.TrimSpace(r.URL.Query().Get("author")) // author slug
	match := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("match")))
	if match != "all" {
		match = "any"
	}
	var cats []string
	if csv := strings.TrimSpace(r.URL.Query().Get("categories")); csv != "" {
		for _, s := range strings.Split(csv, ",") {
			if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
				cats = append(cats, s)
			}
		}
	}

	// ----- shared WHERE (reused by count + rows) -----
	where := []string{}
	args := []any{}
	i := 1

	// author (by slug)
	if author != "" {
		where = append(where, "a.slug = $"+strconv.Itoa(i))
		args = append(args, author)
		i++
	}

	// categories (ANY vs ALL)
	if len(cats) > 0 {
		if match == "any" {
			where = append(where, `
EXISTS (
  SELECT 1
  FROM book_categories bc2
  JOIN categories c2 ON c2.id = bc2.category_id
  WHERE bc2.book_id = b.id AND c2.slug = ANY($`+strconv.Itoa(i)+`::text[])
)`)
		} else {
			where = append(where, `
(
  SELECT COUNT(DISTINCT c2.slug)
  FROM book_categories bc2
  JOIN categories c2 ON c2.id = bc2.category_id
  WHERE bc2.book_id = b.id AND c2.slug = ANY($`+strconv.Itoa(i)+`::text[])
) = `+strconv.Itoa(len(cats)))
		}
		args = append(args, cats)
		i++
	}

	// q = fuzzy, accent-insensitive
	qIdx, minIdx := -1, -1
	if q != "" {
		defMin := 0.20
		if len([]rune(q)) <= 3 {
			defMin = 0.10
		}
		minSim := defMin
		if raw := strings.TrimSpace(r.URL.Query().Get("min_sim")); raw != "" {
			if f, err := strconv.ParseFloat(raw, 64); err == nil && f >= 0 && f <= 1 {
				minSim = f
			}
		}
		qIdx = i
		args = append(args, q)
		i++
		minIdx = i
		args = append(args, minSim)
		i++
		where = append(where, `(
  public.immutable_unaccent(lower(b.title)) LIKE '%' || public.immutable_unaccent(lower($`+strconv.Itoa(qIdx)+`)) || '%'
  OR public.immutable_unaccent(lower(a.name))  LIKE '%' || public.immutable_unaccent(lower($`+strconv.Itoa(qIdx)+`)) || '%'
  OR GREATEST(
       similarity(public.immutable_unaccent(lower(b.title)), public.immutable_unaccent(lower($`+strconv.Itoa(qIdx)+`))),
       similarity(public.immutable_unaccent(lower(a.name)),  public.immutable_unaccent(lower($`+strconv.Itoa(qIdx)+`)))
     ) >= $`+strconv.Itoa(minIdx)+`
)`)
	}

	// ----- total count (with filters) -----
	qCount := `
SELECT COUNT(*)
FROM books b
JOIN authors a ON a.id = b.author_id
`
	if len(where) > 0 {
		qCount += "WHERE " + strings.Join(where, " AND ") + "\n"
	}

	var total int
	if err := db.QueryRow(qCount, args...).Scan(&total); err != nil {
		apperr.WriteStatus(w, r, http.StatusInternalServerError, "DB error", "Failed to count books")
		return
	}

	// ----- page rows (NOW includes slug) -----
	qRows := `
SELECT
  b.id, b.short_id, b.slug, b.title, a.name,
  COALESCE(json_agg(DISTINCT c_all.slug) FILTER (WHERE c_all.slug IS NOT NULL), '[]')
FROM books b
JOIN authors a                ON a.id = b.author_id
LEFT JOIN book_categories bc1 ON bc1.book_id = b.id
LEFT JOIN categories c_all    ON c_all.id = bc1.category_id
`
	if len(where) > 0 {
		qRows += "WHERE " + strings.Join(where, " AND ") + "\n"
	}
	qRows += `
GROUP BY b.id, b.short_id, b.slug, b.title, a.name
`

	// ranked when q present, else recency
	if qIdx != -1 {
		qRows += `
ORDER BY GREATEST(
  similarity(public.immutable_unaccent(lower(b.title)), public.immutable_unaccent(lower($` + strconv.Itoa(qIdx) + `))),
  similarity(public.immutable_unaccent(lower(a.name)),  public.immutable_unaccent(lower($` + strconv.Itoa(qIdx) + `)))
) DESC, b.created_at DESC
`
	} else {
		qRows += "ORDER BY b.created_at DESC\n"
	}

	// add limit/offset bindings
	qRows += "LIMIT $" + strconv.Itoa(i) + " OFFSET $" + strconv.Itoa(i+1)

	rows, err := db.Query(qRows, append(args, limit, offset)...)
	if err != nil {
		apperr.WriteStatus(w, r, http.StatusInternalServerError, "DB error", "Failed to list books")
		return
	}
	defer rows.Close()

	var out []PublicBook
	for rows.Next() {
		var pb PublicBook
		var slugsJSON []byte
		if err := rows.Scan(&pb.ID, &pb.ShortID, &pb.Slug, &pb.Title, &pb.Author, &slugsJSON); err != nil {
			apperr.WriteStatus(w, r, http.StatusInternalServerError, "DB scan error", "Failed to read books")
			return
		}
		_ = json.Unmarshal(slugsJSON, &pb.CategorySlugs)
		pb.URL = "/books/" + pb.Slug
		out = append(out, pb)
	}

	// meta
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
		"next_offset": nextOffset,
		"data":        out,
	})
}
