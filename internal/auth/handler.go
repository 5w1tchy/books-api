package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/5w1tchy/books-api/internal/api/middlewares"
	jwtutil "github.com/5w1tchy/books-api/internal/security/jwt"
	"github.com/5w1tchy/books-api/internal/security/password"
	"github.com/redis/go-redis/v9"
)

type Handler struct {
	Store UserStore
	RDB   *redis.Client
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type MeResponse struct {
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	Username      string     `json:"username"`
	Status        string     `json:"status"`
	EmailVerified *time.Time `json:"email_verified_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func New(store UserStore, rdb *redis.Client) *Handler {
	return &Handler{Store: store, RDB: rdb}
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "Invalid JSON")
		return
	}

	// Password policy: trim + min length (8). Warn-only strength info.
	req.Password = strings.TrimSpace(req.Password)
	if len(req.Password) < 8 || req.Email == "" {
		writeErr(w, http.StatusBadRequest, "invalid_input", "Invalid email or password")
		return
	}
	score, warnMsg, sugg := simpleStrength(req.Password, req.Email, req.Username)
	var pwWarn any
	if score < 3 {
		pwWarn = map[string]any{
			"score":       score,
			"message":     warnMsg,
			"suggestions": sugg,
		}
	}

	hash, err := password.Hash(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash_error", "Failed to hash password")
		return
	}

	u, err := h.Store.CreateUser(req.Email, req.Username, hash)
	if err != nil {
		writeErr(w, http.StatusConflict, "conflict", "Cannot create user")
		return
	}

	access, _, err := jwtutil.SignAccess(u.ID, u.TokenVersion, jwtutil.DefaultAccessTTL())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "jwt_error", "Failed to sign access token")
		return
	}

	refresh, err := h.issueRefresh(r.Context(), u.ID, u.TokenVersion)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "refresh_error", "Failed to issue refresh token")
		return
	}

	resp := map[string]any{
		"access_token":  access,
		"refresh_token": refresh,
	}
	if pwWarn != nil {
		resp["password_warning"] = pwWarn
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "Invalid JSON")
		return
	}
	u, err := h.Store.FindUserByEmail(req.Email)
	if err != nil || u.ID == "" {
		writeErr(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
		return
	}
	ok, needsRehash, err := password.Verify(req.Password, u.PasswordHash)
	if err != nil || !ok {
		writeErr(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
		return
	}
	if needsRehash {
		if newPHC, err := password.Hash(req.Password); err == nil {
			_ = h.Store.UpdateUserPasswordHash(u.ID, newPHC)
		}
	}

	access, _, err := jwtutil.SignAccess(u.ID, u.TokenVersion, jwtutil.DefaultAccessTTL())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "jwt_error", "Failed to sign access token")
		return
	}
	refresh, err := h.issueRefresh(r.Context(), u.ID, u.TokenVersion)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "refresh_error", "Failed to issue refresh token")
		return
	}

	writeJSON(w, http.StatusOK, TokenPair{AccessToken: access, RefreshToken: refresh})
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "Invalid JSON")
		return
	}
	key := "rt:" + req.RefreshToken

	ctx := r.Context()
	val, err := h.RDB.Get(ctx, key).Result()
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid_refresh", "Invalid refresh token")
		return
	}

	parts := strings.SplitN(val, "|", 2) // value: userID|tokenVersion
	if len(parts) != 2 {
		writeErr(w, http.StatusUnauthorized, "invalid_refresh", "Invalid refresh token")
		return
	}
	userID := parts[0]
	tv, _ := strconv.Atoi(parts[1])

	// confirm token_version is current
	var dbVer int
	if err := h.Store.(*SQLStore).DB.QueryRowContext(ctx,
		`SELECT COALESCE(token_version,1) FROM public.users WHERE id=$1`, userID).Scan(&dbVer); err != nil || dbVer != tv {
		writeErr(w, http.StatusUnauthorized, "token_revoked", "Token has been revoked")
		return
	}

	// rotate refresh
	_ = h.RDB.Del(ctx, key).Err()
	newRefresh, err := h.issueRefresh(ctx, userID, dbVer)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "refresh_error", "Failed to issue refresh token")
		return
	}

	access, _, err := jwtutil.SignAccess(userID, dbVer, jwtutil.DefaultAccessTTL())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "jwt_error", "Failed to sign access token")
		return
	}

	writeJSON(w, http.StatusOK, TokenPair{AccessToken: access, RefreshToken: newRefresh})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.RefreshToken == "" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	_ = h.RDB.Del(r.Context(), "rt:"+req.RefreshToken).Err()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := middlewares.UserIDFrom(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}
	const q = `
		SELECT id, email, username, status, email_verified_at, created_at
		FROM public.users WHERE id=$1 LIMIT 1;
	`
	var resp MeResponse
	if err := h.Store.(*SQLStore).DB.QueryRowContext(r.Context(), q, userID).Scan(
		&resp.ID, &resp.Email, &resp.Username, &resp.Status, &resp.EmailVerified, &resp.CreatedAt,
	); err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---- refresh helpers (Redis allowlist Option B) ----

func (h *Handler) issueRefresh(ctx context.Context, userID string, tokenVersion int) (string, error) {
	token, err := randToken()
	if err != nil {
		return "", err
	}
	if h.RDB == nil {
		return "", errors.New("redis not configured")
	}
	key := "rt:" + token
	val := userID + "|" + itoa(tokenVersion)
	if err := h.RDB.Set(ctx, key, val, refreshTTL()).Err(); err != nil {
		return "", err
	}
	return token, nil
}

func (h *Handler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	userID, ok := middlewares.UserIDFrom(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}
	_, err := h.Store.(*SQLStore).DB.ExecContext(r.Context(),
		`UPDATE public.users SET token_version = COALESCE(token_version,1) + 1, updated_at=now() WHERE id=$1`, userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "update_failed", "Failed to update token version")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := middlewares.UserIDFrom(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_input", "Invalid input")
		return
	}
	np := strings.TrimSpace(req.NewPassword)
	if len(np) < 8 || req.OldPassword == "" {
		writeErr(w, http.StatusBadRequest, "invalid_input", "Invalid input")
		return
	}

	// load current hash + tv
	var storedHash string
	var tv int
	err := h.Store.(*SQLStore).DB.QueryRowContext(r.Context(),
		`SELECT password_hash, COALESCE(token_version,1) FROM public.users WHERE id=$1`, userID).
		Scan(&storedHash, &tv)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "User not found")
		return
	}

	okPass, _, err := password.Verify(req.OldPassword, storedHash)
	if err != nil || !okPass {
		writeErr(w, http.StatusForbidden, "forbidden", "Invalid old password")
		return
	}

	// Warn-only strength: set headers (keep 200 JSON shape)
	score, warnMsg, _ := simpleStrength(np)
	if score < 3 {
		w.Header().Set("X-Password-Score", strconv.Itoa(score))
		if warnMsg != "" {
			w.Header().Set("X-Password-Warning", warnMsg)
		} else {
			w.Header().Set("X-Password-Warning", "Password could be stronger")
		}
	}

	newPHC, err := password.Hash(np)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash_error", "Failed to hash new password")
		return
	}

	// set new hash + bump token_version
	_, err = h.Store.(*SQLStore).DB.ExecContext(r.Context(),
		`UPDATE public.users
		   SET password_hash=$1, token_version=COALESCE(token_version,1)+1, updated_at=now()
		 WHERE id=$2`,
		newPHC, userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "update_failed", "Failed to update password")
		return
	}

	// issue fresh tokens (tv+1)
	access, _, err := jwtutil.SignAccess(userID, tv+1, jwtutil.DefaultAccessTTL())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "jwt_error", "Failed to sign access token")
		return
	}
	newRefresh, err := h.issueRefresh(r.Context(), userID, tv+1)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "refresh_error", "Failed to issue refresh token")
		return
	}

	writeJSON(w, http.StatusOK, TokenPair{AccessToken: access, RefreshToken: newRefresh})
}

// utilities

func refreshTTL() time.Duration {
	if s := os.Getenv("AUTH_REFRESH_TTL"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			return d
		}
	}
	return 30 * 24 * time.Hour
}

func randToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func itoa(i int) string { return strconv.FormatInt(int64(i), 10) }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// simpleStrength gives a coarse 0..4 score + short warning/suggestions (warn-only use).
func simpleStrength(pwd string, hints ...string) (int, string, []string) {
	l := len(pwd)
	var hasL, hasU, hasD, hasS bool
	for _, r := range pwd {
		switch {
		case r >= 'a' && r <= 'z':
			hasL = true
		case r >= 'A' && r <= 'Z':
			hasU = true
		case r >= '0' && r <= '9':
			hasD = true
		default:
			hasS = true
		}
	}
	classes := 0
	if hasL {
		classes++
	}
	if hasU {
		classes++
	}
	if hasD {
		classes++
	}
	if hasS {
		classes++
	}

	// small penalty if password contains a hint (like email/user)
	for _, h := range hints {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		if strings.Contains(strings.ToLower(pwd), h) && l < 16 {
			if classes > 1 {
				classes--
			}
			break
		}
	}

	// scoring heuristic
	switch {
	case l >= 14 && classes >= 3:
		return 4, "", nil
	case l >= 12 && classes >= 3:
		return 3, "", []string{"Consider using 3–4 word passphrase for even stronger security."}
	case l >= 10 && classes >= 2:
		return 2, "Short or low variety.", []string{"Add more length and mix of letters, numbers, symbols."}
	case l >= 8:
		return 1, "Too short or predictable.", []string{"Use at least 10–12 chars with mixed types."}
	default:
		return 0, "Very weak password.", []string{"Use 12+ chars with upper/lower, numbers, symbols."}
	}
}
