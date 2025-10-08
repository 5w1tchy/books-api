package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

const StatsCacheKey = "admin:stats"
const StatsCacheDuration = 30 * time.Second

// GET /admin/stats
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Try cache first
	if cached := h.getCachedStats(ctx, w); cached {
		return
	}

	// Fetch fresh data
	stats, err := h.fetchStats(ctx)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	// Cache and return
	h.cacheAndWriteStats(ctx, w, stats)
}

func (h *Handler) getCachedStats(ctx context.Context, w http.ResponseWriter) bool {
	if h.RDB == nil {
		return false
	}

	cached, err := h.RDB.Get(ctx, StatsCacheKey).Result()
	if err != nil || cached == "" {
		return false
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = w.Write([]byte(cached))
	return true
}

func (h *Handler) fetchStats(ctx context.Context) (*StatsResponse, error) {
	total, verified, err := h.Sto.CountUsers(ctx)
	if err != nil {
		return nil, err
	}

	books, err := h.Sto.CountBooks(ctx)
	if err != nil {
		return nil, err
	}

	signups, err := h.Sto.CountSignupsLast24h(ctx)
	if err != nil {
		return nil, err
	}

	return &StatsResponse{
		UsersTotal:     total,
		UsersVerified:  verified,
		BooksTotal:     books,
		SignupsLast24h: signups,
	}, nil
}

func (h *Handler) cacheAndWriteStats(ctx context.Context, w http.ResponseWriter, stats *StatsResponse) {
	statsJSON, _ := json.Marshal(stats)

	// Cache for next time
	if h.RDB != nil {
		_ = h.RDB.SetEx(ctx, StatsCacheKey, statsJSON, StatsCacheDuration).Err()
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = w.Write(statsJSON)
}
