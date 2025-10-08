package foryou

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/5w1tchy/books-api/internal/api/httpx"
	"github.com/5w1tchy/books-api/internal/api/middlewares"
	storeforyou "github.com/5w1tchy/books-api/internal/store/foryou"
	storeuserbooks "github.com/5w1tchy/books-api/internal/store/userbooks"
	"github.com/redis/go-redis/v9"
)

func Handler(db *sql.DB, rdb *redis.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.ErrorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		ctx := r.Context()
		q := r.URL.Query()

		// Require auth (feed is user-specific)
		userID, isAuth := middlewares.UserIDFrom(ctx)
		if !isAuth {
			httpx.ErrorJSON(w, http.StatusUnauthorized, "authentication required")
			return
		}

		// Parse options
		f := storeforyou.Fields{
			Lite:           q.Get("lite") == "true",
			IncludeSummary: q.Get("summary") == "true",
		}

		// ---- Simple per-user cache for the full payload (5 minutes) ----
		// Key includes user, day, and flags that affect output.
		day := time.Now().Format("2006-01-02")
		cacheKey := fmt.Sprintf(
			"fy:user:%s:%s:lite=%t;summary=%t;v1",
			userID, day, f.Lite, f.IncludeSummary,
		)

		// Try cache hit first
		if rdb != nil {
			if cached, err := rdb.Get(ctx, cacheKey).Bytes(); err == nil && len(cached) > 0 {
				// Serve cached JSON directly (already encoded)
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.Header().Set("Cache-Control", "private, max-age=300")
				_, _ = w.Write(cached)
				return
			}
		}

		// Create RNG + date window for shorts
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		now := time.Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		tomorrow := today.Add(24 * time.Hour)

		// Continue Reading (best-effort; don't fail the whole feed)
		continueReading, err := storeuserbooks.GetContinueReading(ctx, db, userID, 5)
		if err != nil {
			// just ignore; continueReading stays empty
			continueReading = nil
		}

		// Shorts (required)
		shorts, err := storeforyou.BuildShorts(ctx, db, 10, today, tomorrow, rng)
		if err != nil {
			httpx.ErrorJSON(w, http.StatusInternalServerError, "failed to get shorts")
			return
		}

		// Recommendations (required)
		recs, err := storeforyou.BuildRecs(ctx, db, 20, shorts, rng, f)
		if err != nil {
			httpx.ErrorJSON(w, http.StatusInternalServerError, "failed to get recommendations")
			return
		}

		payload := map[string]any{
			"status": "success",
			"data": map[string]any{
				"continue_reading": continueReading,
				"shorts":           shorts,
				"recommended":      recs,
			},
		}

		// Cache the whole JSON body for 5 minutes (private)
		if rdb != nil {
			if buf, err := json.Marshal(payload); err == nil {
				_ = rdb.Set(ctx, cacheKey, buf, 5*time.Minute).Err()
			}
		}

		w.Header().Set("Cache-Control", "private, max-age=300")
		httpx.WriteJSON(w, http.StatusOK, payload)
	})
}
