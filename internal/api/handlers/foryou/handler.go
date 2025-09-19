package foryou

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	storefy "github.com/5w1tchy/books-api/internal/store/foryou"
	"github.com/redis/go-redis/v9"
)

type ForYouResponse struct {
	Status string         `json:"status"`
	Data   ForYouDataWrap `json:"data"`
}

type ForYouDataWrap struct {
	Shorts          []storefy.ShortItem `json:"shorts"`
	Recs            []storefy.BookLite  `json:"recs"`
	Trending        []storefy.BookLite  `json:"trending"`
	New             []storefy.BookLite  `json:"new"`
	MostViewed      []storefy.BookLite  `json:"most_viewed"`
	ContinueReading []storefy.BookLite  `json:"continue_reading"`
}

func Handler(db *sql.DB, rdb *redis.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// top-level guard (keep 2s)
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		// which blocks?
		want := wanted(r.URL.Query().Get("fields")) // shorts,trending,new,recs,most_viewed

		// global limit (per section), plus optional per-block overrides
		defLimit := parseLimit(r.URL.Query().Get("limit"), 8)
		lim := storefy.Limits{
			Shorts:     takeIf(want["shorts"], parseLimit(r.URL.Query().Get("shorts"), defLimit)),
			Recs:       takeIf(want["recs"], parseLimit(r.URL.Query().Get("recs"), defLimit)),
			Trending:   takeIf(want["trending"], parseLimit(r.URL.Query().Get("trending"), defLimit)),
			New:        takeIf(want["new"], parseLimit(r.URL.Query().Get("new"), defLimit)),
			MostViewed: takeIf(want["most_viewed"], parseLimit(r.URL.Query().Get("most_viewed"), defLimit)),
		}

		// flags: lite/summary (via booleans or include CSV)
		includeSet := parseSet(r.URL.Query().Get("include"))
		fields := storefy.Fields{
			Lite:           isTrue(r.URL.Query().Get("lite")) || includeSet["lite"],
			IncludeSummary: isTrue(r.URL.Query().Get("summary")) || includeSet["summary"],
		}

		// build all requested cached sections in one shot (store layer handles caching/timeouts)
		secs, buildErr := storefy.Build(ctx, db, rdb, lim, fields)
		if buildErr != nil {
			// Log but keep returning partials
			log.Printf("[for-you][ERROR] store.Build: %v", buildErr)
		}

		resp := ForYouResponse{
			Status: "success",
			Data: ForYouDataWrap{
				Shorts:          secs.Shorts,
				Recs:            secs.Recs,
				Trending:        secs.Trending,
				New:             secs.New,
				MostViewed:      secs.MostViewed,
				ContinueReading: []storefy.BookLite{},
			},
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	})
}

/* --- helpers --- */

func parseLimit(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 || n > 50 {
		return def
	}
	return n
}

func wanted(fieldsCSV string) map[string]bool {
	m := map[string]bool{
		"shorts":      true,
		"recs":        true,
		"trending":    true,
		"new":         true,
		"most_viewed": true,
	}
	if strings.TrimSpace(fieldsCSV) == "" {
		return m
	}
	for k := range m {
		m[k] = false
	}
	for _, f := range strings.Split(fieldsCSV, ",") {
		k := strings.ToLower(strings.TrimSpace(f))
		if k != "" {
			m[k] = true
		}
	}
	return m
}

func parseSet(csv string) map[string]bool {
	out := map[string]bool{}
	if strings.TrimSpace(csv) == "" {
		return out
	}
	for _, p := range strings.Split(csv, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" {
			out[p] = true
		}
	}
	return out
}

func isTrue(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func takeIf(ok bool, n int) int {
	if ok {
		return n
	}
	return 0
}
