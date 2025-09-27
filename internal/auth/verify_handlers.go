package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	evKeyPrefix   = "ev:"       // token → user_id
	evQuotaPrefix = "ev:quota:" // user_id → count (24h TTL)
	evTTL         = 24 * time.Hour
	evQuotaMax    = 3
)

type VerifyDeps struct {
	DB  *sql.DB
	RDB *redis.Client
	// If you want to show a full URL, set BaseURL (e.g., https://localhost:3000).
	// Leave empty to return just the path for dev.
	BaseURL string
}

// POST /auth/send-verification (auth required)
// Behavior:
//   - If already verified → 204
//   - Else generate token, store in Redis for 24h, increment per-user quota (max 3/24h)
//   - DEV: returns verify_url in JSON so frontend can call it directly
func (d *VerifyDeps) HandleSendVerification(getUserID func(*http.Request) (string, bool)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		userID, ok := getUserID(r)
		if !ok || userID == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Check already verified
		var verifiedAt sql.NullTime
		if err := d.DB.QueryRowContext(ctx, `SELECT email_verified_at FROM users WHERE id=$1`, userID).Scan(&verifiedAt); err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		if verifiedAt.Valid {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Quota: 3 sends per 24h
		qKey := evQuotaPrefix + userID
		pipe := d.RDB.TxPipeline()
		incr := pipe.Incr(ctx, qKey)
		pipe.Expire(ctx, qKey, 24*time.Hour)
		if _, err := pipe.Exec(ctx); err != nil {
			http.Error(w, "rate limit error", http.StatusInternalServerError)
			return
		}
		if incr.Val() > int64(evQuotaMax) {
			// Optional: help client decide when to retry
			w.Header().Set("Retry-After", strconv.Itoa(int(24*time.Hour/time.Second)))
			http.Error(w, "verification send limit reached", http.StatusTooManyRequests)
			return
		}

		// New token
		token, err := randomToken(32)
		if err != nil {
			http.Error(w, "token error", http.StatusInternalServerError)
			return
		}
		if err := d.RDB.SetEx(ctx, evKeyPrefix+token, userID, evTTL).Err(); err != nil {
			http.Error(w, "redis error", http.StatusInternalServerError)
			return
		}

		// DEV convenience: return clickable path (or absolute if BaseURL set)
		path := d.buildVerifyURL(token)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":     "ok",
			"verify_url": path,
		})
	}
}

// GET /auth/verify?token=...
// Behavior:
//   - Valid token → set users.email_verified_at=now(), delete token, 204
//   - Invalid/expired → 400
func (d *VerifyDeps) HandleVerify() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		// normalize token (dev-safety): trim spaces and trailing slashes
		token := strings.TrimSpace(r.URL.Query().Get("token"))
		token = strings.TrimRight(token, "/")
		if token == "" {
			http.Error(w, "missing token", http.StatusBadRequest)
			return
		}

		key := evKeyPrefix + token
		userID, err := d.RDB.Get(ctx, key).Result()
		if errors.Is(err, redis.Nil) || userID == "" {
			http.Error(w, "invalid or expired token", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "redis error", http.StatusInternalServerError)
			return
		}

		tx, err := d.DB.BeginTx(ctx, nil)
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		_, err = tx.ExecContext(ctx, `UPDATE users SET email_verified_at=NOW() WHERE id=$1 AND email_verified_at IS NULL`, userID)
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			_ = tx.Rollback()
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		_ = d.RDB.Del(ctx, key).Err() // best effort

		w.WriteHeader(http.StatusNoContent)
	}
}

func (d *VerifyDeps) buildVerifyURL(token string) string {
	path := "/auth/verify?token=" + token
	if d.BaseURL == "" {
		return path
	}
	base := strings.TrimRight(d.BaseURL, "/")
	return base + path
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
