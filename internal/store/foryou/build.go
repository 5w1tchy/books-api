package foryou

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	productTZName   = "Asia/Tbilisi"
	feedCacheTTL    = 2 * time.Hour
	featureCooldown = 90 * 24 * time.Hour
)

func Build(ctx context.Context, db *sql.DB, rdb *redis.Client, lim Limits, f Fields) (Sections, error) {
	tz := mustTZ(productTZName)
	now := time.Now().In(tz)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz)
	tomorrow := today.Add(24 * time.Hour)

	fieldsKey := ""
	if f.Lite {
		fieldsKey = "lite"
	} else if f.IncludeSummary {
		fieldsKey = "summary"
	}

	cacheKey := fmt.Sprintf("for_you:%s:s=%d:r=%d:t=%d:n=%d:f=%s",
		today.Format("2006-01-02"), lim.Shorts, lim.Recs, lim.Trending, lim.New, fieldsKey)

	// try cache
	if rdb != nil {
		if b, err := rdb.Get(ctx, cacheKey).Bytes(); err == nil && len(b) > 0 {
			var sec Sections
			if json.Unmarshal(b, &sec) == nil {
				return sec, nil
			}
		}
	}

	seed := dailySeed(today)
	rng := rand.New(rand.NewSource(int64(seed)))

	shorts, err := pickShorts(ctx, db, lim.Shorts, today, tomorrow, rng)
	if err != nil {
		return Sections{}, fmt.Errorf("pickShorts: %w", err)
	}
	recs, err := pickRecs(ctx, db, lim.Recs, shorts, rng, f)
	if err != nil {
		return Sections{}, fmt.Errorf("pickRecs: %w", err)
	}
	if recs == nil {
		recs = make([]BookLite, 0)
	}
	trending, err := pickTrending(ctx, db, lim.Trending, f)
	if err != nil {
		return Sections{}, fmt.Errorf("pickTrending: %w", err)
	}
	if trending == nil {
		trending = make([]BookLite, 0)
	}
	newest, err := pickNewest(ctx, db, lim.New, f)
	if err != nil {
		return Sections{}, fmt.Errorf("pickNewest: %w", err)
	}
	if newest == nil {
		newest = make([]BookLite, 0)
	}

	sec := Sections{
		Shorts:          shorts,
		Recs:            recs,
		Trending:        trending,
		New:             newest,
		ContinueReading: []BookLite{},
	}

	if rdb != nil {
		if b, err := json.Marshal(sec); err == nil {
			_ = rdb.Set(ctx, cacheKey, b, feedCacheTTL).Err()
		}
	}
	return sec, nil
}

// ---------- selection helpers ----------

type shortPick struct{ ID, Slug, Title, Author, Short string }

func pickShorts(ctx context.Context, db *sql.DB, limit int, today, tomorrow time.Time, rng *rand.Rand) ([]ShortItem, error) {
	if limit <= 0 {
		return []ShortItem{}, nil
	}

	const qToday = `
SELECT b.id, b.slug, b.title, a.name, bo.short::text AS short
FROM book_outputs bo
JOIN books b   ON b.id = bo.book_id
JOIN authors a ON a.id = b.author_id
WHERE bo.short_enabled
  AND bo.short IS NOT NULL AND bo.short::text <> '""'
  AND bo.short_last_featured_at >= $1
  AND bo.short_last_featured_at <  $2
ORDER BY bo.short_last_featured_at ASC, b.created_at DESC
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
SELECT b.id, b.slug, b.title, a.name, bo.short::text AS short, bo.short_last_featured_at
FROM book_outputs bo
JOIN books b   ON b.id = bo.book_id
JOIN authors a ON a.id = b.author_id
WHERE bo.short_enabled
  AND bo.short IS NOT NULL AND bo.short::text <> '""'
  AND (bo.short_last_featured_at IS NULL OR bo.short_last_featured_at < $1)
ORDER BY bo.short_last_featured_at NULLS FIRST, b.created_at DESC
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
SELECT b.id, b.slug, b.title, a.name, bo.short::text AS short
FROM book_outputs bo
JOIN books b   ON b.id = bo.book_id
JOIN authors a ON a.id = b.author_id
WHERE bo.short_enabled
  AND bo.short IS NOT NULL AND bo.short::text <> '""'
  AND bo.short_last_featured_at IS NOT NULL
ORDER BY bo.short_last_featured_at ASC, b.created_at DESC
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
					if _, err2 := tx.ExecContext(ctx, `UPDATE book_outputs SET short_last_featured_at = $1 WHERE book_id = $2`, today, p.ID); err2 != nil {
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

func pickRecs(ctx context.Context, db *sql.DB, limit int, shorts []ShortItem, rng *rand.Rand, f Fields) ([]BookLite, error) {
	if limit <= 0 {
		return []BookLite{}, nil
	}
	if len(shorts) == 0 {
		return pickNewest(ctx, db, limit, f)
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

func pickTrending(ctx context.Context, db *sql.DB, limit int, f Fields) ([]BookLite, error) {
	if limit <= 0 {
		return []BookLite{}, nil
	}
	const q = `
SELECT b.id, b.slug, b.title, a.name,
       COALESCE(jsonb_agg(DISTINCT c.slug) FILTER (WHERE c.slug IS NOT NULL), '[]'::jsonb) AS slugs,
       COALESCE(MAX(bo.summary), '') AS summary
FROM books b
JOIN authors a               ON a.id = b.author_id
LEFT JOIN book_categories bc ON bc.book_id = b.id
LEFT JOIN categories c       ON c.id = bc.category_id
LEFT JOIN book_outputs bo    ON bo.book_id = b.id
WHERE bo.short IS NOT NULL
  AND bo.short::text NOT IN ('', '""')
  AND bo.short_enabled
GROUP BY b.id, b.slug, b.title, a.name, bo.short_last_featured_at
ORDER BY bo.short_last_featured_at DESC NULLS LAST, b.created_at DESC
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
	return out, nil
}

func pickNewest(ctx context.Context, db *sql.DB, limit int, f Fields) ([]BookLite, error) {
	if limit <= 0 {
		return []BookLite{}, nil
	}
	const q = `
SELECT b.id, b.slug, b.title, a.name,
       COALESCE(jsonb_agg(DISTINCT c.slug) FILTER (WHERE c.slug IS NOT NULL), '[]'::jsonb) AS slugs,
       COALESCE(MAX(bo.summary), '') AS summary
FROM books b
JOIN authors a               ON a.id = b.author_id
LEFT JOIN book_categories bc ON bc.book_id = b.id
LEFT JOIN categories c       ON c.id = bc.category_id
LEFT JOIN book_outputs bo    ON bo.book_id = b.id
GROUP BY b.id, b.slug, b.title, a.name, b.created_at
ORDER BY b.created_at DESC
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
	return out, nil
}

// ---------- small utils ----------

func containsID(xs []shortPick, id string) bool {
	for _, x := range xs {
		if x.ID == id {
			return true
		}
	}
	return false
}

func mustTZ(name string) *time.Location {
	if loc, err := time.LoadLocation(name); err == nil {
		return loc
	}
	return time.FixedZone(name, 4*3600)
}

func dailySeed(day time.Time) uint64 {
	key := day.Format("2006-01-02")
	sum := sha1.Sum([]byte(key))
	return binary.BigEndian.Uint64(sum[:8])
}
