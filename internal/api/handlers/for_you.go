package handlers

import (
	"crypto/sha1"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type ForYouShort struct {
	Content string `json:"content"`
	Book    struct {
		ID    string `json:"id"`
		Slug  string `json:"slug"`
		Title string `json:"title"`
		URL   string `json:"url"`
	} `json:"book"`
}

// internal record for selection
type rec struct {
	BookID string
	Slug   string
	Title  string
	Short  string
}

const dayCacheTTL = 36 * time.Hour

func ForYouHandler(db *sql.DB, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		limit := 5
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 10 {
				limit = n
			}
		}

		// Product timezone: Asia/Tbilisi
		tz, err := time.LoadLocation("Asia/Tbilisi")
		if err != nil {
			tz = time.FixedZone("Asia/Tbilisi", 4*3600)
		}
		now := time.Now().In(tz)
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz)
		tomorrow := today.Add(24 * time.Hour)
		cooldownCutoff := today.Add(-90 * 24 * time.Hour)
		cacheKey := fmt.Sprintf("for_you:%s:limit=%d", today.Format("2006-01-02"), limit)

		ctx := r.Context()
		// 0) Day cache (Redis) — serve directly if present
		if rdb != nil {
			if b, err := rdb.Get(ctx, cacheKey).Bytes(); err == nil && len(b) > 0 {
				_, _ = w.Write(b)
				return
			}
		}

		// 1) If today's picks already exist, read them
		var picks []rec
		const qToday = `
SELECT b.id, b.slug, b.title, bo.short
FROM book_outputs bo
JOIN books b ON b.id = bo.book_id
WHERE bo.short_enabled
  AND COALESCE(bo.short,'') <> ''
  AND bo.short_last_featured_at >= $1
  AND bo.short_last_featured_at <  $2
ORDER BY bo.short_last_featured_at ASC, b.created_at DESC
LIMIT $3;`
		rows, err := db.Query(qToday, today, tomorrow, limit)
		if err != nil {
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		for rows.Next() {
			var r rec
			if err := rows.Scan(&r.BookID, &r.Slug, &r.Title, &r.Short); err != nil {
				_ = rows.Close()
				http.Error(w, "DB scan error", http.StatusInternalServerError)
				return
			}
			picks = append(picks, r)
		}
		rows.Close()

		// 2) Not enough? Fill with eligible (never/90d+) then soft fallback (least-recently featured)
		if len(picks) < limit {
			missing := limit - len(picks)

			// 2a) Eligible pool
			const qEligible = `
SELECT b.id, b.slug, b.title, bo.short, bo.short_last_featured_at
FROM book_outputs bo
JOIN books b ON b.id = bo.book_id
WHERE bo.short_enabled
  AND COALESCE(bo.short,'') <> ''
  AND (bo.short_last_featured_at IS NULL OR bo.short_last_featured_at < $1)
ORDER BY bo.short_last_featured_at NULLS FIRST, b.created_at DESC
LIMIT $2;`
			erows, err := db.Query(qEligible, cooldownCutoff, missing*8)
			if err != nil {
				http.Error(w, "DB error", http.StatusInternalServerError)
				return
			}
			var elig []rec
			for erows.Next() {
				var r rec
				var ignore sql.NullTime // can be NULL
				if err := erows.Scan(&r.BookID, &r.Slug, &r.Title, &r.Short, &ignore); err != nil {
					_ = erows.Close()
					http.Error(w, "DB scan error", http.StatusInternalServerError)
					return
				}
				elig = append(elig, r)
			}
			erows.Close()

			seed := dailySeed(today)
			rand.New(rand.NewSource(int64(seed))).Shuffle(len(elig), func(i, j int) { elig[i], elig[j] = elig[j], elig[i] })
			for i := 0; i < len(elig) && len(picks) < limit; i++ {
				if !containsID(picks, elig[i].BookID) {
					picks = append(picks, elig[i])
				}
			}

			// 2b) Soft fallback: least-recently featured
			if len(picks) < limit {
				remaining := limit - len(picks)
				const qFallback = `
SELECT b.id, b.slug, b.title, bo.short
FROM book_outputs bo
JOIN books b ON b.id = bo.book_id
WHERE bo.short_enabled
  AND COALESCE(bo.short,'') <> ''
  AND bo.short_last_featured_at IS NOT NULL
ORDER BY bo.short_last_featured_at ASC, b.created_at DESC
LIMIT $1;`
				frows, err := db.Query(qFallback, remaining*8)
				if err != nil {
					http.Error(w, "DB error", http.StatusInternalServerError)
					return
				}
				var fb []rec
				for frows.Next() {
					var r rec
					if err := frows.Scan(&r.BookID, &r.Slug, &r.Title, &r.Short); err != nil {
						_ = frows.Close()
						http.Error(w, "DB scan error", http.StatusInternalServerError)
						return
					}
					if !containsID(picks, r.BookID) {
						fb = append(fb, r)
					}
				}
				frows.Close()

				rand.New(rand.NewSource(int64(seed))).Shuffle(len(fb), func(i, j int) { fb[i], fb[j] = fb[j], fb[i] })
				for i := 0; i < len(fb) && len(picks) < limit; i++ {
					picks = append(picks, fb[i])
				}
			}

			// 2c) Mark today's picks
			if len(picks) > 0 {
				if tx, err := db.Begin(); err == nil {
					ok := true
					for _, p := range picks {
						if _, err2 := tx.Exec(`UPDATE book_outputs SET short_last_featured_at = $1 WHERE book_id = $2`, today, p.BookID); err2 != nil {
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

		// Build final JSON payload
		out := make([]ForYouShort, 0, len(picks))
		for _, p := range picks {
			var it ForYouShort
			it.Content = p.Short
			it.Book.ID = p.BookID
			it.Book.Slug = p.Slug
			it.Book.Title = p.Title
			it.Book.URL = "/books/" + p.Slug
			out = append(out, it)
		}
		payload := map[string]any{"status": "success", "data": map[string]any{"shorts": out}}

		// Cache the full payload for the day (if Redis configured)
		if rdb != nil {
			if b, err := json.Marshal(payload); err == nil {
				_ = rdb.Set(ctx, cacheKey, b, dayCacheTTL).Err()
				_, _ = w.Write(b)
				return
			}
		}

		_ = json.NewEncoder(w).Encode(payload)
	}
}

func containsID(xs []rec, id string) bool {
	for _, x := range xs {
		if x.BookID == id {
			return true
		}
	}
	return false
}

func dailySeed(day time.Time) uint64 {
	key := day.Format("2006-01-02")
	sum := sha1.Sum([]byte(key))
	return binary.BigEndian.Uint64(sum[:8])
}
