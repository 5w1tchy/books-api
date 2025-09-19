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
	kMost := fmt.Sprintf("most_viewed:%s:f=%s:m=%d", day, fieldsKey, lim.MostViewed)

	var (
		shorts     []ShortItem
		recs       []BookLite
		trending   []BookLite
		newest     []BookLite
		mostViewed []BookLite

		hitShorts bool
		hitRecs   bool
		hitTrend  bool
		hitNew    bool
		hitMV     bool
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

	// most_viewed
	if lim.MostViewed > 0 {
		if hit, ok := c.mget(ctx, kMost); ok && len(hit) == 1 && hit[0] != nil {
			if err := json.Unmarshal(hit[0], &mostViewed); err == nil {
				hitMV = true
				dbg("cache hit: most_viewed (%d items)", len(mostViewed))
			} else {
				errf("cache unmarshal most_viewed failed", err)
				mostViewed = nil
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

		s, err := BuildShorts(ctxS, db, lim.Shorts, today, tomorrow, rng)
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

			r, err := BuildRecs(ctxR, db, lim.Recs, shorts, rng, f)
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

		t, err := BuildTrending(ctxT, db, lim.Trending, f)
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

		n, err := BuildNewest(ctxN, db, lim.New, f)
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

	// ------------------ MOST_VIEWED (timeout + cache 2h) ------------------
	if mostViewed == nil && lim.MostViewed > 0 {
		dbg("cache miss: most_viewed -> computing")
		ctxMV, cancel := context.WithTimeout(ctx, blockTO)
		defer cancel()

		mv, err := BuildMostViewed(ctxMV, db, lim.MostViewed, f)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				errf("most_viewed", err)
			}
			mostViewed = []BookLite{}
		} else if mv == nil {
			mostViewed = []BookLite{}
		} else {
			mostViewed = mv
			if b, err := json.Marshal(mostViewed); err == nil {
				toCache2h[c.key(kMost)] = b
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
		MostViewed:      mostViewed,
		ContinueReading: []BookLite{},
	}

	dbg("cache summary: shorts=%t recs=%t trending=%t new=%t most_viewed=%t", hitShorts, hitRecs, hitTrend, hitNew, hitMV)
	return sec, nil
}

// ---------- selection helpers ----------

type shortPick struct{ ID, Slug, Title, Author, Short string }

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
