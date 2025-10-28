package auth

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/5w1tchy/books-api/internal/api/httpx"
	"github.com/5w1tchy/books-api/internal/api/middlewares"
	jwtutil "github.com/5w1tchy/books-api/internal/security/jwt"
	"github.com/5w1tchy/books-api/internal/security/password"
)

// Me returns the current user's profile
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := middlewares.UserIDFrom(r.Context())
	if !ok {
		httpx.ErrorCode(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}
	const q = `
        SELECT id, email, username, role, status, email_verified_at, created_at
        FROM public.users WHERE id=$1 LIMIT 1;
    `
	var resp MeResponse
	if err := h.Store.(*SQLStore).DB.QueryRowContext(r.Context(), q, userID).Scan(
		&resp.ID, &resp.Email, &resp.Username, &resp.Role, &resp.Status, &resp.EmailVerified, &resp.CreatedAt,
	); err != nil {
		httpx.ErrorCode(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// ChangePassword updates the user's password and rotates tokens
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := middlewares.UserIDFrom(r.Context())
	if !ok {
		httpx.ErrorCode(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.ErrorCode(w, http.StatusBadRequest, "invalid_input", "Invalid input")
		return
	}
	np := strings.TrimSpace(req.NewPassword)
	if len(np) < 8 || req.OldPassword == "" {
		httpx.ErrorCode(w, http.StatusBadRequest, "invalid_input", "Invalid input")
		return
	}

	// load current hash + tv
	var storedHash string
	var tv int
	err := h.Store.(*SQLStore).DB.QueryRowContext(r.Context(),
		`SELECT password_hash, COALESCE(token_version,1) FROM public.users WHERE id=$1`, userID).
		Scan(&storedHash, &tv)
	if err != nil {
		httpx.ErrorCode(w, http.StatusNotFound, "not_found", "User not found")
		return
	}

	okPass, _, err := password.Verify(req.OldPassword, storedHash)
	if err != nil || !okPass {
		httpx.ErrorCode(w, http.StatusForbidden, "forbidden", "Invalid old password")
		return
	}

	// Warn-only strength: headers + capture score/message for body
	score, warnMsg, sugg := simpleStrength(np)
	if score < 3 { // keep your existing header behavior
		w.Header().Set("X-Password-Score", strconv.Itoa(score))
		if warnMsg != "" {
			w.Header().Set("X-Password-Warning", warnMsg)
		} else {
			w.Header().Set("X-Password-Warning", "Password could be stronger")
		}
	}

	newPHC, err := password.Hash(np)
	if err != nil {
		httpx.ErrorCode(w, http.StatusInternalServerError, "hash_error", "Failed to hash new password")
		return
	}

	// set new hash + bump token_version
	_, err = h.Store.(*SQLStore).DB.ExecContext(r.Context(),
		`UPDATE public.users
		   SET password_hash=$1, token_version=COALESCE(token_version,1)+1, updated_at=now()
		 WHERE id=$2`,
		newPHC, userID)
	if err != nil {
		httpx.ErrorCode(w, http.StatusInternalServerError, "update_failed", "Failed to update password")
		return
	}

	// issue fresh tokens (tv+1)
	access, _, err := jwtutil.SignAccess(userID, tv+1, jwtutil.DefaultAccessTTL())
	if err != nil {
		httpx.ErrorCode(w, http.StatusInternalServerError, "jwt_error", "Failed to sign access token")
		return
	}
	newRefresh, err := h.issueRefresh(r.Context(), userID, tv+1)
	if err != nil {
		httpx.ErrorCode(w, http.StatusInternalServerError, "refresh_error", "Failed to issue refresh token")
		return
	}

	// JSON: always include password_score; include warning when score < 4 (for consistency with Register)
	resp := map[string]any{
		"access_token":   access,
		"refresh_token":  newRefresh,
		"password_score": score,
	}
	if score < 4 && warnMsg != "" {
		resp["password_warning"] = map[string]any{
			"message":     warnMsg,
			"suggestions": sugg,
		}
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

// LogoutAll invalidates all user sessions by incrementing token version
func (h *Handler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	userID, ok := middlewares.UserIDFrom(r.Context())
	if !ok {
		httpx.ErrorCode(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}
	_, err := h.Store.(*SQLStore).DB.ExecContext(r.Context(),
		`UPDATE public.users SET token_version = COALESCE(token_version,1) + 1, updated_at=now() WHERE id=$1`, userID)
	if err != nil {
		httpx.ErrorCode(w, http.StatusInternalServerError, "update_failed", "Failed to update token version")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
