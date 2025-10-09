package books

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
)

// List returns page of books (same filters/behavior as before) and total count.
func List(ctx context.Context, db *sql.DB, f ListFilters) ([]PublicBook, int, error) {
	where := []string{}
	args := []any{}
	i := 1

	// author filter (any of provided author slugs/names)
	if len(f.Authors) > 0 {
		where = append(where, `
EXISTS (
  SELECT 1
  FROM book_authors ba
  JOIN authors a2 ON a2.id = ba.author_id
  WHERE ba.book_id = b.id AND a2.slug = ANY($`+strconv.Itoa(i)+`::text[])
)`)
		args = append(args, f.Authors)
		i++
	}

	// categories filter (any|all)
	if n := len(f.Categories); n > 0 {
		if f.Match != "all" {
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
) = `+strconv.Itoa(n))
		}
		args = append(args, f.Categories)
		i++
	}

	// q/min_sim filter (LIKE + pg_trgm similarity)
	qIdx, minIdx := -1, -1
	if f.Q != "" {
		qIdx = i
		args = append(args, f.Q)
		i++
		minIdx = i
		args = append(args, f.MinSim)
		i++
		where = append(where, `(
  public.immutable_unaccent(lower(b.title)) LIKE '%' || public.immutable_unaccent(lower($`+strconv.Itoa(qIdx)+`)) || '%'
  OR EXISTS (
        SELECT 1 FROM authors a2
        JOIN book_authors ba2 ON a2.id = ba2.author_id
        WHERE ba2.book_id = b.id
          AND public.immutable_unaccent(lower(a2.name)) LIKE '%' || public.immutable_unaccent(lower($`+strconv.Itoa(qIdx)+`)) || '%'
    )
  OR GREATEST(
       similarity(public.immutable_unaccent(lower(b.title)), public.immutable_unaccent(lower($`+strconv.Itoa(qIdx)+`))),
       (SELECT MAX(similarity(public.immutable_unaccent(lower(a2.name)), public.immutable_unaccent(lower($`+strconv.Itoa(qIdx)+`))))
        FROM authors a2 JOIN book_authors ba2 ON a2.id = ba2.author_id WHERE ba2.book_id = b.id)
     ) >= $`+strconv.Itoa(minIdx)+`
)`)
	}

	// total count
	qCount := `
SELECT COUNT(*)
FROM books b
`
	if len(where) > 0 {
		qCount += "WHERE " + strings.Join(where, " AND ") + "\n"
	}
	var total int
	if err := db.QueryRowContext(ctx, qCount, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// page rows
	qRows := `
SELECT
  b.id,
  b.short_id,
  b.slug,
  b.title,
  COALESCE(jsonb_agg(DISTINCT a.name) FILTER (WHERE a.name IS NOT NULL), '[]'::jsonb) AS authors,
  COALESCE(jsonb_agg(DISTINCT c_all.slug) FILTER (WHERE c_all.slug IS NOT NULL), '[]'::jsonb) AS categories
FROM books b
LEFT JOIN book_authors ba ON ba.book_id = b.id
LEFT JOIN authors a ON a.id = ba.author_id
LEFT JOIN book_categories bc1 ON bc1.book_id = b.id
LEFT JOIN categories c_all ON c_all.id = bc1.category_id
`
	if len(where) > 0 {
		qRows += "WHERE " + strings.Join(where, " AND ") + "\n"
	}
	qRows += `
GROUP BY b.id, b.short_id, b.slug, b.title
`
	// ranking when q present; else recency
	if qIdx != -1 {
		qRows += `
ORDER BY GREATEST(
  similarity(public.immutable_unaccent(lower(b.title)), public.immutable_unaccent(lower($` + strconv.Itoa(qIdx) + `)))
) DESC, b.created_at DESC
`
	} else {
		qRows += "ORDER BY b.created_at DESC\n"
	}
	// limit/offset
	qRows += "LIMIT $" + strconv.Itoa(i) + " OFFSET $" + strconv.Itoa(i+1)

	rows, err := db.QueryContext(ctx, qRows, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []PublicBook
	for rows.Next() {
		var pb PublicBook
		var authorsJSON, catsJSON []byte
		if err := rows.Scan(&pb.ID, &pb.ShortID, &pb.Slug, &pb.Title, &authorsJSON, &catsJSON); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal(authorsJSON, &pb.Authors)
		_ = json.Unmarshal(catsJSON, &pb.CategorySlugs)
		pb.URL = "/books/" + pb.Slug
		out = append(out, pb)
	}
	return out, total, rows.Err()
}
