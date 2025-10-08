package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/5w1tchy/books-api/internal/api/middlewares"
	"github.com/redis/go-redis/v9"
)

// ===== Types & Contracts =====

type UserRow struct {
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	Username      string     `json:"username"`
	Role          string     `json:"role"`   // "user" | "admin"
	Status        string     `json:"status"` // "active" | "banned"
	EmailVerified *time.Time `json:"email_verified_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

type ListFilter struct {
	Query    string
	Role     string
	Verified *bool
	Status   string
	Page     int
	Size     int
}

// Audit list DTOs
type AuditRow struct {
	ID        int64     `json:"id"`
	AdminID   string    `json:"admin_id"`
	Action    string    `json:"action"`
	TargetID  *string   `json:"target_id,omitempty"`
	Meta      any       `json:"meta"`
	CreatedAt time.Time `json:"created_at"`
}

type AuditFilter struct {
	ActorID  string
	TargetID string
	Action   string
	Since    *time.Time
	Until    *time.Time
	Page     int
	Size     int
}

type Store interface {
	// Users
	ListUsers(ctx context.Context, filter ListFilter) ([]UserRow, int, error)
	GetUser(ctx context.Context, id string) (*UserRow, error)
	SetUserStatus(ctx context.Context, id, status string) error
	SetUserRole(ctx context.Context, id, role string) error
	BumpTokenVersion(ctx context.Context, id string) error
	CountUsers(ctx context.Context) (total, verified int, err error)
	CountBooks(ctx context.Context) (int, error)
	CountSignupsLast24h(ctx context.Context) (int, error)

	// Audit
	InsertAudit(ctx context.Context, adminID, action, targetID string, meta any) error
	ListAudit(ctx context.Context, f AuditFilter) ([]AuditRow, int, error)

	// Admins
	AdminCount(ctx context.Context) (int, error)
}

type Handler struct {
	DB  *sql.DB
	RDB *redis.Client
	Sto Store
}

// ===== Helpers =====

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func parseBool(q string) *bool {
	if q == "" {
		return nil
	}
	b := strings.EqualFold(q, "true") || q == "1"
	return &b
}

func pathID(r *http.Request) string { return r.PathValue("id") }

func rateKey(prefix, adminID string) string { return "admin:rl:" + prefix + ":" + adminID }

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

// ===== Endpoints =====

// GET /admin/users
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(q.Get("size"))
	if size < 1 || size > 200 {
		size = 25
	}
	filter := ListFilter{
		Query:    q.Get("query"),
		Role:     q.Get("role"),
		Verified: parseBool(q.Get("verified")),
		Status:   q.Get("status"),
		Page:     page,
		Size:     size,
	}
	users, total, err := h.Sto.ListUsers(r.Context(), filter)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "list_failed"})
		return
	}
	writeJSON(w, 200, map[string]any{
		"data": users, "total": total, "page": page, "size": size,
	})
}

// GET /admin/users/{id}
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	u, err := h.Sto.GetUser(r.Context(), id)
	if err != nil || u == nil {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	writeJSON(w, 200, u)
}

// POST /admin/users/{id}/ban
func (h *Handler) BanUser(w http.ResponseWriter, r *http.Request) {
	adminID, _ := middlewares.UserIDFrom(r.Context())
	id := pathID(r)

	if adminID == id {
		writeJSON(w, 400, map[string]string{"error": "cannot_self_ban"})
		return
	}
	ok, err := h.allowAction(r.Context(), rateKey("ban", adminID), 20, time.Hour)
	if err != nil || !ok {
		writeJSON(w, 429, map[string]string{"error": "rate_limited"})
		return
	}
	if err := h.Sto.SetUserStatus(r.Context(), id, "banned"); err != nil {
		writeJSON(w, 500, map[string]string{"error": "ban_failed"})
		return
	}
	_ = h.Sto.InsertAudit(r.Context(), adminID, "user.ban", id, nil)
	writeJSON(w, 204, nil)
}

// POST /admin/users/{id}/unban
func (h *Handler) UnbanUser(w http.ResponseWriter, r *http.Request) {
	adminID, _ := middlewares.UserIDFrom(r.Context())
	id := pathID(r)

	ok, err := h.allowAction(r.Context(), rateKey("unban", adminID), 20, time.Hour)
	if err != nil || !ok {
		writeJSON(w, 429, map[string]string{"error": "rate_limited"})
		return
	}
	if err := h.Sto.SetUserStatus(r.Context(), id, "active"); err != nil {
		writeJSON(w, 500, map[string]string{"error": "unban_failed"})
		return
	}
	_ = h.Sto.InsertAudit(r.Context(), adminID, "user.unban", id, nil)
	writeJSON(w, 204, nil)
}

// POST /admin/users/{id}/role
type setRoleReq struct {
	Role string `json:"role"`
}

func (h *Handler) SetRole(w http.ResponseWriter, r *http.Request) {
	adminID, _ := middlewares.UserIDFrom(r.Context())
	id := pathID(r)

	var body setRoleReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || (body.Role != "admin" && body.Role != "user") {
		writeJSON(w, 400, map[string]string{"error": "invalid_role"})
		return
	}

	// Prevent demoting yourself if you're the last admin.
	if adminID == id && body.Role != "admin" {
		count, err := h.Sto.AdminCount(r.Context())
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "check_admins_failed"})
			return
		}
		if count <= 1 {
			writeJSON(w, 400, map[string]string{"error": "cannot_demote_last_admin"})
			return
		}
	}

	ok, err := h.allowAction(r.Context(), rateKey("setrole", adminID), 50, time.Hour)
	if err != nil || !ok {
		writeJSON(w, 429, map[string]string{"error": "rate_limited"})
		return
	}
	if err := h.Sto.SetUserRole(r.Context(), id, body.Role); err != nil {
		writeJSON(w, 500, map[string]string{"error": "role_set_failed"})
		return
	}
	_ = h.Sto.InsertAudit(r.Context(), adminID, "user.role.set", id, body)
	writeJSON(w, 204, nil)
}

// POST /admin/users/{id}/logout-all
func (h *Handler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	adminID, _ := middlewares.UserIDFrom(r.Context())
	id := pathID(r)

	ok, err := h.allowAction(r.Context(), rateKey("logoutall", adminID), 50, time.Hour)
	if err != nil || !ok {
		writeJSON(w, 429, map[string]string{"error": "rate_limited"})
		return
	}
	if err := h.Sto.BumpTokenVersion(r.Context(), id); err != nil {
		writeJSON(w, 500, map[string]string{"error": "logout_all_failed"})
		return
	}
	_ = h.Sto.InsertAudit(r.Context(), adminID, "user.logout_all", id, nil)
	writeJSON(w, 204, nil)
}

// POST /admin/users/{id}/resend-verification
func (h *Handler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	adminID, _ := middlewares.UserIDFrom(r.Context())
	id := pathID(r)

	ok, err := h.allowAction(r.Context(), rateKey("resend", adminID), 100, time.Hour)
	if err != nil || !ok {
		writeJSON(w, 429, map[string]string{"error": "rate_limited"})
		return
	}

	// Hook to your mailer (no-op here, but we audit the attempt).
	_ = h.Sto.InsertAudit(r.Context(), adminID, "user.resend_verification", id, nil)
	writeJSON(w, 204, nil)
}

// GET /admin/stats
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	type resp struct {
		UsersTotal     int `json:"users_total"`
		UsersVerified  int `json:"users_verified"`
		BooksTotal     int `json:"books_total"`
		SignupsLast24h int `json:"signups_last_24h"`
	}

	// tiny cache (30s)
	cacheKey := "admin:stats"
	if v, err := h.RDB.Get(ctx, cacheKey).Result(); err == nil && v != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(v))
		return
	}

	total, verified, err := h.Sto.CountUsers(ctx)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "count_users_failed"})
		return
	}
	books, err := h.Sto.CountBooks(ctx)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "count_books_failed"})
		return
	}
	signups, err := h.Sto.CountSignupsLast24h(ctx)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "count_signups_failed"})
		return
	}

	out := resp{UsersTotal: total, UsersVerified: verified, BooksTotal: books, SignupsLast24h: signups}
	b, _ := json.Marshal(out)
	_ = h.RDB.SetEx(ctx, cacheKey, b, 30*time.Second).Err()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = w.Write(b)
}

// GET /admin/audit
func (h *Handler) ListAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(q.Get("size"))
	if size < 1 || size > 200 {
		size = 25
	}

	var sincePtr, untilPtr *time.Time
	if s := q.Get("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			sincePtr = &t
		}
	}
	if s := q.Get("until"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			untilPtr = &t
		}
	}

	filter := AuditFilter{
		ActorID:  q.Get("actor_id"),
		TargetID: q.Get("target_id"),
		Action:   q.Get("action"),
		Since:    sincePtr,
		Until:    untilPtr,
		Page:     page,
		Size:     size,
	}

	items, total, err := h.Sto.ListAudit(r.Context(), filter)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "audit_list_failed"})
		return
	}
	writeJSON(w, 200, map[string]any{
		"data": items, "total": total, "page": page, "size": size,
	})
}
