package foryou

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	productTZName   = "Asia/Tbilisi"
	featureCooldown = 90 * 24 * time.Hour
)

// -------- debug helpers (runtime-checked) --------
func debugEnabled() bool { return os.Getenv("FOR_YOU_DEBUG") == "1" }

func dbg(format string, args ...any) {
	if debugEnabled() {
		log.Printf("[for-you] "+format, args...)
	}
}

func errf(where string, err error) error {
	log.Printf("[for-you][ERROR] %s: %v", where, err)
	return fmt.Errorf("%s: %w", where, err)
}

// ----------------- public entry ------------------

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
	} else {
		fieldsKey = "full"
	}

	// Per-block timeout (env-tunable; default 450ms)
	blockTO := 450 * time.Millisecond
	if v := os.Getenv("FOR_YOU_BLOCK_TIMEOUT_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			blockTO = time.Duration(ms) * time.Millisecond
		}
	}

	// Request-scoped cache (no PING, single-warn on error)
	c := newCache(rdb)

	day := today.Format("2006-01-02")
	kShorts := fmt.Sprintf("shorts:%s:f=%s:s=%d", day, fieldsKey, lim.Shorts)
	kTrending := fmt.Sprintf("trending:%s:f=%s:t=%d", day, fieldsKey, lim.Trending)
	kNew := fmt.Sprintf("new:%s:f=%s:n=%d", day, fieldsKey, lim.New)

	var (
		shorts    []ShortItem
		recs      []BookLite
		trending  []BookLite
		newest    []BookLite
		hitShorts bool
		hitRecs   bool
		hitTrend  bool
		hitNew    bool
	)

	// --------- FAST PATH: cache pulls ----------
	// shorts
	if hit, ok := c.mget(ctx, kShorts); ok && len(hit) == 1 && hit[0] != nil {
		if err := json.Unmarshal(hit[0], &shorts); err == nil {
			hitShorts = true
			dbg("cache hit: shorts (%d items)", len(shorts))
		} else {
			errf("cache unmarshal shorts failed", err)
			shorts = nil
		}
	}

	// trending + new
	if hits, ok := c.mget(ctx, kTrending, kNew); ok && hits != nil {
		if hits[0] != nil {
			if err := json.Unmarshal(hits[0], &trending); err == nil {
				hitTrend = true
				dbg("cache hit: trending (%d items)", len(trending))
			} else {
				errf("cache unmarshal trending failed", err)
				trending = nil
			}
		}
		if hits[1] != nil {
			if err := json.Unmarshal(hits[1], &newest); err == nil {
				hitNew = true
				dbg("cache hit: new (%d items)", len(newest))
			} else {
				errf("cache unmarshal new failed", err)
				newest = nil
			}
		}
	}

	// Staging map for 2h TTL sets
	toCache2h := make(map[string][]byte)

	// Deterministic daily seed
	seed := dailySeed(today)
	rng := rand.New(rand.NewSource(int64(seed)))

	// ------------------ SHORTS (timeout + cache) ------------------
	if shorts == nil {
		dbg("cache miss: shorts -> computing")
		ctxS, cancel := context.WithTimeout(ctx, blockTO)
		defer cancel()

		s, err := pickShorts(ctxS, db, lim.Shorts, today, tomorrow, rng)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				errf("shorts", err)
			}
			shorts = []ShortItem{}
		} else if s == nil {
			shorts = []ShortItem{}
		} else {
			shorts = s
		}
		if b, err := json.Marshal(shorts); err == nil {
			toCache2h[c.key(kShorts)] = b
		}
	}

	// ------------------ RECS (timeout + cache by shorts signature) ------------------
	{
		ids := make([]string, 0, len(shorts))
		for _, s := range shorts {
			ids = append(ids, s.Book.ID)
		}
		sum := sha1.Sum([]byte(strings.Join(ids, ",")))
		sig := hex.EncodeToString(sum[:8]) // short signature

		kRecs := fmt.Sprintf("recs:%s:f=%s:r=%d:sig=%s", day, fieldsKey, lim.Recs, sig)

		// Try cache
		if hit, ok2 := c.mget(ctx, kRecs); ok2 && len(hit) == 1 && hit[0] != nil {
			if err := json.Unmarshal(hit[0], &recs); err == nil {
				hitRecs = true
				dbg("cache hit: recs (%d items)", len(recs))
			} else {
				errf("cache unmarshal recs failed", err)
				recs = nil
			}
		}

		// Build if miss
		if recs == nil {
			dbg("cache miss: recs -> computing")
			ctxR, cancel := context.WithTimeout(ctx, blockTO)
			defer cancel()

			r, err := pickRecs(ctxR, db, lim.Recs, shorts, rng, f)
			if err != nil {
				if !errors.Is(err, sql.ErrNoRows) {
					errf("recs", err)
				}
				recs = []BookLite{}
			} else if r == nil {
				recs = []BookLite{}
			} else {
				recs = r
				if b, err := json.Marshal(recs); err == nil {
					toCache2h[c.key(kRecs)] = b
				}
			}
		}
	}

	// ------------------ TRENDING (timeout + cache 2h) ------------------
	if trending == nil {
		dbg("cache miss: trending -> computing")
		ctxT, cancel := context.WithTimeout(ctx, blockTO)
		defer cancel()

		t, err := pickTrending(ctxT, db, lim.Trending, f)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				errf("trending", err)
			}
			trending = []BookLite{}
		} else if t == nil {
			trending = []BookLite{}
		} else {
			trending = t
			if b, err := json.Marshal(trending); err == nil {
				toCache2h[c.key(kTrending)] = b
			}
		}
	}

	// ------------------ NEW (timeout + cache 2h) ------------------
	if newest == nil {
		dbg("cache miss: new -> computing")
		ctxN, cancel := context.WithTimeout(ctx, blockTO)
		defer cancel()

		n, err := pickNewest(ctxN, db, lim.New, f)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				errf("new", err)
			}
			newest = []BookLite{}
		} else if n == nil {
			newest = []BookLite{}
		} else {
			newest = n
			if b, err := json.Marshal(newest); err == nil {
				toCache2h[c.key(kNew)] = b
			}
		}
	}

	// Best-effort pipelined cache set (2h TTL keys)
	c.setPipeline(ctx, toCache2h)

	// ------------------ Assemble response ------------------
	sec := Sections{
		Shorts:          shorts,
		Recs:            recs,
		Trending:        trending,
		New:             newest,
		ContinueReading: []BookLite{},
	}

	dbg("cache summary: shorts=%t recs=%t trending=%t new=%t", hitShorts, hitRecs, hitTrend, hitNew)
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

		// mark today's picks (update ONLY latest book_outputs row per book_id)
		if len(picks) > 0 {
			if tx, err := db.BeginTx(ctx, nil); err == nil {
				ok := true
				for _, p := range picks {
					_, err2 := tx.ExecContext(ctx, `
UPDATE book_outputs bo
SET short_last_featured_at = $1
FROM (
  SELECT id
  FROM book_outputs
  WHERE book_id = $2
  ORDER BY created_at DESC
  LIMIT 1
) latest
WHERE bo.id = latest.id
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
