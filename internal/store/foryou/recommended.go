package foryou

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
)

func BuildRecs(ctx context.Context, db *sql.DB, limit int, shorts []ShortItem, rng *rand.Rand, f Fields) ([]BookLite, error) {
	if limit <= 0 {
		return []BookLite{}, nil
	}
	if len(shorts) == 0 {
		return BuildNewest(ctx, db, limit, f)
	}

	ids := make([]any, 0, len(shorts))
	for _, s := range shorts {
		ids = append(ids, s.Book.ID)
	}

	q := `
WITH featured AS (
  SELECT unnest(ARRAY[%s])::uuid AS id
),
short_cats AS (
  SELECT DISTINCT c.id AS cat_id
  FROM books b
  JOIN book_categories bc ON bc.book_id = b.id
  JOIN categories c       ON c.id = bc.category_id
  WHERE b.id IN (SELECT id FROM featured)
),
recs AS (
  SELECT DISTINCT
    b.id, b.slug, b.title, a.name,
    COALESCE(jsonb_agg(DISTINCT c.slug) FILTER (WHERE c.slug IS NOT NULL), '[]'::jsonb) AS slugs,
    COALESCE(MAX(bo.summary), '') AS summary,
    MAX(b.created_at) AS newest
  FROM books b
  JOIN authors a          ON a.id = b.author_id
  JOIN book_categories bc ON bc.book_id = b.id
  JOIN categories c       ON c.id = bc.category_id
  LEFT JOIN book_outputs bo ON bo.book_id = b.id
  WHERE c.id IN (SELECT cat_id FROM short_cats)
    AND b.id NOT IN (SELECT id FROM featured)
  GROUP BY b.id, b.slug, b.title, a.name
  ORDER BY newest DESC
  LIMIT $1
)
SELECT id, slug, title, name, slugs, summary FROM recs;`

	ph := ""
	for i := range ids {
		if i > 0 {
			ph += ","
		}
		ph += fmt.Sprintf("$%d", i+2)
	}
	q = fmt.Sprintf(q, ph)

	args := make([]any, 0, 1+len(ids))
	args = append(args, limit)
	args = append(args, ids...)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]BookLite, 0, limit)
	for rows.Next() {
		var b BookLite
		var slugsJSON []byte
		var summary string
		if err := rows.Scan(&b.ID, &b.Slug, &b.Title, &b.Author, &slugsJSON, &summary); err != nil {
			return nil, err
		}
		if !f.Lite {
			_ = json.Unmarshal(slugsJSON, &b.CategorySlugs)
		}
		if f.IncludeSummary && summary != "" {
			b.Summary = summary
		}
		b.URL = "/books/" + b.Slug
		out = append(out, b)
	}

	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
