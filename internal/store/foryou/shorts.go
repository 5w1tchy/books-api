package foryou

import (
	"context"
	"database/sql"
	"math/rand"
	"time"
)

func BuildShorts(ctx context.Context, db *sql.DB, limit int, today, tomorrow time.Time, rng *rand.Rand) ([]ShortItem, error) {
	if limit <= 0 {
		return []ShortItem{}, nil
	}

	const qToday = `
SELECT b.id, b.slug, b.title, a.name, b.short::text AS short
FROM books b
JOIN authors a ON a.id = b.author_id
WHERE b.short_enabled
  AND b.short IS NOT NULL AND b.short::text <> '""'
  AND b.short_last_featured_at >= $1
  AND b.short_last_featured_at <  $2
ORDER BY b.short_last_featured_at ASC, b.created_at DESC
LIMIT $3;`

	var picks []shortPick
	rows, err := db.QueryContext(ctx, qToday, today, tomorrow, limit)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r shortPick
		if err := rows.Scan(&r.ID, &r.Slug, &r.Title, &r.Author, &r.Short); err != nil {
			rows.Close()
			return nil, err
		}
		picks = append(picks, r)
	}
	rows.Close()

	if len(picks) < limit {
		missing := limit - len(picks)
		cutoff := today.Add(-featureCooldown)

		const qEligible = `
SELECT b.id, b.slug, b.title, a.name, b.short::text AS short, b.short_last_featured_at
FROM books b
JOIN authors a ON a.id = b.author_id
WHERE b.short_enabled
  AND b.short IS NOT NULL AND b.short::text <> '""'
  AND (b.short_last_featured_at IS NULL OR b.short_last_featured_at < $1)
ORDER BY b.short_last_featured_at NULLS FIRST, b.created_at DESC
LIMIT $2;`

		erows, err := db.QueryContext(ctx, qEligible, cutoff, missing*8)
		if err != nil {
			return nil, err
		}
		var elig []shortPick
		for erows.Next() {
			var r shortPick
			var ignore sql.NullTime
			if err := erows.Scan(&r.ID, &r.Slug, &r.Title, &r.Author, &r.Short, &ignore); err != nil {
				erows.Close()
				return nil, err
			}
			elig = append(elig, r)
		}
		erows.Close()

		rng.Shuffle(len(elig), func(i, j int) { elig[i], elig[j] = elig[j], elig[i] })
		for i := 0; i < len(elig) && len(picks) < limit; i++ {
			if !containsID(picks, elig[i].ID) {
				picks = append(picks, elig[i])
			}
		}

		if len(picks) < limit {
			const qFallback = `
SELECT b.id, b.slug, b.title, a.name, b.short::text AS short
FROM books b
JOIN authors a ON a.id = b.author_id
WHERE b.short_enabled
  AND b.short IS NOT NULL AND b.short::text <> '""'
  AND b.short_last_featured_at IS NOT NULL
ORDER BY b.short_last_featured_at ASC, b.created_at DESC
LIMIT $1;`
			frows, err := db.QueryContext(ctx, qFallback, (limit-len(picks))*8)
			if err != nil {
				return nil, err
			}
			var fb []shortPick
			for frows.Next() {
				var r shortPick
				if err := frows.Scan(&r.ID, &r.Slug, &r.Title, &r.Author, &r.Short); err != nil {
					frows.Close()
					return nil, err
				}
				if !containsID(picks, r.ID) {
					fb = append(fb, r)
				}
			}
			frows.Close()

			rng.Shuffle(len(fb), func(i, j int) { fb[i], fb[j] = fb[j], fb[i] })
			for i := 0; i < len(fb) && len(picks) < limit; i++ {
				picks = append(picks, fb[i])
			}
		}

		// mark today's picks
		if len(picks) > 0 {
			if tx, err := db.BeginTx(ctx, nil); err == nil {
				ok := true
				for _, p := range picks {
					_, err2 := tx.ExecContext(ctx, `
UPDATE books
SET short_last_featured_at = $1
WHERE id = $2
`, today, p.ID)
					if err2 != nil {
						_ = tx.Rollback()
						ok = false
						break
					}
				}
				if ok {
					_ = tx.Commit()
				}
			}
		}
	}

	out := make([]ShortItem, 0, len(picks))
	for _, p := range picks {
		out = append(out, ShortItem{
			Content: p.Short,
			Book: BookLite{
				ID:     p.ID,
				Slug:   p.Slug,
				Title:  p.Title,
				Author: p.Author,
				URL:    "/books/" + p.Slug,
			},
		})
	}
	return out, nil
}
