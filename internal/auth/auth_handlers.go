package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/5w1tchy/books-api/internal/api/httpx"
	jwtutil "github.com/5w1tchy/books-api/internal/security/jwt"
	"github.com/5w1tchy/books-api/internal/security/password"
)

// Register creates a new user account
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.ErrorCode(w, http.StatusBadRequest, "bad_request", "Invalid JSON")
		return
	}

	// DEBUG: Log what we received
	fmt.Printf("DEBUG: Received registration request - Email: %s, Role: %s\n", req.Email, req.Role)

	// Password policy: trim + min length (8). Warn-only strength info.
	req.Password = strings.TrimSpace(req.Password)
	if len(req.Password) < 8 || req.Email == "" {
		httpx.ErrorCode(w, http.StatusBadRequest, "invalid_input", "Invalid email or password")
		return
	}

	// DEFAULT role to "user" ONLY if not specified or empty
	if req.Role == "" {
		req.Role = "user"
	}

	// Strength scoring (warn-only)
	score, warnMsg, sugg := simpleStrength(req.Password, req.Email, req.Username)

	hash, err := password.Hash(req.Password)
	if err != nil {
		httpx.ErrorCode(w, http.StatusInternalServerError, "hash_error", "Failed to hash password")
		return
	}

	u, err := h.Store.CreateUser(req.Email, req.Username, hash, req.Role)
	if err != nil {
		httpx.ErrorCode(w, http.StatusConflict, "conflict", "Cannot create user")
		return
	}

	access, _, err := jwtutil.SignAccess(u.ID, u.TokenVersion, jwtutil.DefaultAccessTTL())
	if err != nil {
		httpx.ErrorCode(w, http.StatusInternalServerError, "jwt_error", "Failed to sign access token")
		return
	}

	refresh, err := h.issueRefresh(r.Context(), u.ID, u.TokenVersion)
	if err != nil {
		httpx.ErrorCode(w, http.StatusInternalServerError, "refresh_error", "Failed to issue refresh token")
		return
	}

	// Build response: always include password_score; include warning when score < 4
	resp := map[string]any{
		"access_token":   access,
		"refresh_token":  refresh,
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

// Login authenticates a user and returns tokens
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.ErrorCode(w, http.StatusBadRequest, "bad_request", "Invalid JSON")
		return
	}
	u, err := h.Store.FindUserByEmail(req.Email)
	if err != nil || u.ID == "" {
		httpx.ErrorCode(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
		return
	}
	ok, needsRehash, err := password.Verify(req.Password, u.PasswordHash)
	if err != nil || !ok {
		httpx.ErrorCode(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
		return
	}
	if needsRehash {
		if newPHC, err := password.Hash(req.Password); err == nil {
			_ = h.Store.UpdateUserPasswordHash(u.ID, newPHC)
		}
	}

	access, _, err := jwtutil.SignAccess(u.ID, u.TokenVersion, jwtutil.DefaultAccessTTL())
	if err != nil {
		httpx.ErrorCode(w, http.StatusInternalServerError, "jwt_error", "Failed to sign access token")
		return
	}
	refresh, err := h.issueRefresh(r.Context(), u.ID, u.TokenVersion)
	if err != nil {
		httpx.ErrorCode(w, http.StatusInternalServerError, "refresh_error", "Failed to issue refresh token")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, TokenPair{AccessToken: access, RefreshToken: refresh})
}

// Refresh generates new tokens using a refresh token
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		httpx.ErrorCode(w, http.StatusBadRequest, "bad_request", "Invalid JSON")
		return
	}
	key := "rt:" + req.RefreshToken

	ctx := r.Context()
	val, err := h.RDB.Get(ctx, key).Result()
	if err != nil {
		httpx.ErrorCode(w, http.StatusUnauthorized, "invalid_refresh", "Invalid refresh token")
		return
	}

	parts := strings.SplitN(val, "|", 2) // value: userID|tokenVersion
	if len(parts) != 2 {
		httpx.ErrorCode(w, http.StatusUnauthorized, "invalid_refresh", "Invalid refresh token")
		return
	}
	userID := parts[0]
	tv, _ := strconv.Atoi(parts[1])

	// confirm token_version is current
	var dbVer int
	if err := h.Store.(*SQLStore).DB.QueryRowContext(ctx,
		`SELECT COALESCE(token_version,1) FROM public.users WHERE id=$1`, userID).Scan(&dbVer); err != nil || dbVer != tv {
		httpx.ErrorCode(w, http.StatusUnauthorized, "token_revoked", "Token has been revoked")
		return
	}

	// rotate refresh
	_ = h.RDB.Del(ctx, key).Err()
	newRefresh, err := h.issueRefresh(ctx, userID, dbVer)
	if err != nil {
		httpx.ErrorCode(w, http.StatusInternalServerError, "refresh_error", "Failed to issue refresh token")
		return
	}

	access, _, err := jwtutil.SignAccess(userID, dbVer, jwtutil.DefaultAccessTTL())
	if err != nil {
		httpx.ErrorCode(w, http.StatusInternalServerError, "jwt_error", "Failed to sign access token")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, TokenPair{AccessToken: access, RefreshToken: newRefresh})
}

// Logout invalidates a single refresh token
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.RefreshToken == "" {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	_ = h.RDB.Del(r.Context(), "rt:"+req.RefreshToken).Err()
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
