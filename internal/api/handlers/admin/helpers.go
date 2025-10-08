package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/5w1tchy/books-api/internal/api/middlewares"
)

// ===== HTTP Helpers =====

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]string{"error": message})
}

// ===== Request Helpers =====

func pathID(r *http.Request) string {
	return r.PathValue("id")
}

func getAdminID(ctx context.Context) string {
	adminID, _ := middlewares.UserIDFrom(ctx)
	return adminID
}

func parseBool(q string) *bool {
	if q == "" {
		return nil
	}
	b := strings.EqualFold(q, "true") || q == "1"
	return &b
}

// ===== Rate Limiting =====

func rateKey(prefix, adminID string) string {
	return "admin:rl:" + prefix + ":" + adminID
}

func (h *Handler) allowAction(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	pipe := h.RDB.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}
	return int(incr.Val()) <= limit, nil
}

func (h *Handler) checkRateLimit(ctx context.Context, w http.ResponseWriter, action, adminID string, limit int, window time.Duration) bool {
	ok, err := h.allowAction(ctx, rateKey(action, adminID), limit, window)
	if err != nil || !ok {
		writeError(w, 429, "rate_limited")
		return false
	}
	return true
}

// ===== Validation =====

func validateRole(role string) bool {
	return role == "admin" || role == "user"
}

func validatePagination(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 25
	}
	return page, size
}
