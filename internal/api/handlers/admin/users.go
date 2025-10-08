package admin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// GET /admin/users
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("size"))
	page, size = validatePagination(page, size)

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
		writeError(w, 500, "list_failed")
		return
	}

	writeJSON(w, 200, map[string]any{
		"data": users, "total": total, "page": page, "size": size,
	})
}

// GET /admin/users/{id}
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)

	user, err := h.Sto.GetUser(r.Context(), id)
	if err != nil || user == nil {
		writeError(w, 404, "not_found")
		return
	}

	writeJSON(w, 200, user)
}

// POST /admin/users/{id}/ban
func (h *Handler) BanUser(w http.ResponseWriter, r *http.Request) {
	adminID := getAdminID(r.Context())
	userID := pathID(r)

	if adminID == userID {
		writeError(w, 400, "cannot_self_ban")
		return
	}

	if !h.checkRateLimit(r.Context(), w, "ban", adminID, 20, time.Hour) {
		return
	}

	if err := h.Sto.SetUserStatus(r.Context(), userID, "banned"); err != nil {
		writeError(w, 500, "ban_failed")
		return
	}

	_ = h.Sto.InsertAudit(r.Context(), adminID, "user.ban", userID, nil)
	writeJSON(w, 204, nil)
}

// POST /admin/users/{id}/unban
func (h *Handler) UnbanUser(w http.ResponseWriter, r *http.Request) {
	adminID := getAdminID(r.Context())
	userID := pathID(r)

	if !h.checkRateLimit(r.Context(), w, "unban", adminID, 20, time.Hour) {
		return
	}

	if err := h.Sto.SetUserStatus(r.Context(), userID, "active"); err != nil {
		writeError(w, 500, "unban_failed")
		return
	}

	_ = h.Sto.InsertAudit(r.Context(), adminID, "user.unban", userID, nil)
	writeJSON(w, 204, nil)
}

// POST /admin/users/{id}/role
func (h *Handler) SetRole(w http.ResponseWriter, r *http.Request) {
	adminID := getAdminID(r.Context())
	userID := pathID(r)

	var body SetRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !validateRole(body.Role) {
		writeError(w, 400, "invalid_role")
		return
	}

	// Prevent demoting yourself if you're the last admin
	if adminID == userID && body.Role != "admin" {
		count, err := h.Sto.AdminCount(r.Context())
		if err != nil {
			writeError(w, 500, "check_admins_failed")
			return
		}
		if count <= 1 {
			writeError(w, 400, "cannot_demote_last_admin")
			return
		}
	}

	if !h.checkRateLimit(r.Context(), w, "setrole", adminID, 50, time.Hour) {
		return
	}

	if err := h.Sto.SetUserRole(r.Context(), userID, body.Role); err != nil {
		writeError(w, 500, "role_set_failed")
		return
	}

	_ = h.Sto.InsertAudit(r.Context(), adminID, "user.role.set", userID, body)
	writeJSON(w, 204, nil)
}

// POST /admin/users/{id}/logout-all
func (h *Handler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	adminID := getAdminID(r.Context())
	userID := pathID(r)

	if !h.checkRateLimit(r.Context(), w, "logoutall", adminID, 50, time.Hour) {
		return
	}

	if err := h.Sto.BumpTokenVersion(r.Context(), userID); err != nil {
		writeError(w, 500, "logout_all_failed")
		return
	}

	_ = h.Sto.InsertAudit(r.Context(), adminID, "user.logout_all", userID, nil)
	writeJSON(w, 204, nil)
}

// POST /admin/users/{id}/resend-verification
func (h *Handler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	adminID := getAdminID(r.Context())
	userID := pathID(r)

	if !h.checkRateLimit(r.Context(), w, "resend", adminID, 100, time.Hour) {
		return
	}

	// Hook to your mailer (no-op here, but we audit the attempt)
	_ = h.Sto.InsertAudit(r.Context(), adminID, "user.resend_verification", userID, nil)
	writeJSON(w, 204, nil)
}
