package foryou

import (
	"context"
	"database/sql"
	"encoding/json"
)

func BuildMostViewed(ctx context.Context, db *sql.DB, limit int, f Fields) ([]BookLite, error) {
	if limit <= 0 {
		return []BookLite{}, nil
	}

	const q = `
WITH views AS (
  SELECT book_id, COUNT(*) AS views
  FROM book_view_events
  GROUP BY book_id
)
SELECT
  b.id,
  b.slug,
  b.title,
  a.name,
  COALESCE(jsonb_agg(DISTINCT c.slug) FILTER (WHERE c.slug IS NOT NULL), '[]'::jsonb) AS slugs,
  COALESCE(b.summary, '') AS summary,
  v.views
FROM views v
JOIN books b               ON b.id = v.book_id
JOIN authors a             ON a.id = b.author_id
LEFT JOIN book_categories bc ON bc.book_id = b.id
LEFT JOIN categories c        ON c.id = bc.category_id
GROUP BY b.id, b.slug, b.title, a.name, v.views
ORDER BY v.views DESC
LIMIT $1;`

	rows, err := db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]BookLite, 0, limit)
	for rows.Next() {
		var b BookLite
		var slugsJSON []byte
		var summary string
		var _views int64
		if err := rows.Scan(&b.ID, &b.Slug, &b.Title, &b.Author, &slugsJSON, &summary, &_views); err != nil {
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
	return out, nil
}
