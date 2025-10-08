package userbooks

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/5w1tchy/books-api/internal/api/httpx"
	"github.com/5w1tchy/books-api/internal/api/middlewares"
	storeuserbooks "github.com/5w1tchy/books-api/internal/store/userbooks"
)

// UpdateProgress: POST /user/reading-progress
func UpdateProgress(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpx.ErrorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		userID, ok := middlewares.UserIDFrom(r.Context())
		if !ok {
			httpx.ErrorJSON(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var req struct {
			BookID          string  `json:"book_id"`
			PageNumber      int     `json:"page_number"`
			ProgressPercent float64 `json:"progress_percent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.ErrorJSON(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		if req.BookID == "" {
			httpx.ErrorJSON(w, http.StatusBadRequest, "book_id is required")
			return
		}
		if req.ProgressPercent < 0 || req.ProgressPercent > 100 {
			httpx.ErrorJSON(w, http.StatusBadRequest, "progress_percent must be 0-100")
			return
		}

		if err := storeuserbooks.UpdateReadingProgress(r.Context(), db, userID, req.BookID, req.PageNumber, req.ProgressPercent); err != nil {
			httpx.ErrorJSON(w, http.StatusInternalServerError, "failed to update progress")
			return
		}

		httpx.OKNoData(w)
	})
}

// GetProgress: GET /user/reading-progress/{bookId}
func GetProgress(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.ErrorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		userID, ok := middlewares.UserIDFrom(r.Context())
		if !ok {
			httpx.ErrorJSON(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		bookID := r.PathValue("bookId")
		if bookID == "" {
			httpx.ErrorJSON(w, http.StatusBadRequest, "missing bookId")
			return
		}

		progress, err := storeuserbooks.GetReadingProgress(r.Context(), db, userID, bookID)
		if err == sql.ErrNoRows {
			httpx.ErrorJSON(w, http.StatusNotFound, "no progress found")
			return
		} else if err != nil {
			httpx.ErrorJSON(w, http.StatusInternalServerError, "failed to get progress")
			return
		}

		httpx.OK(w, progress)
	})
}

// ContinueReading: GET /user/continue-reading
func ContinueReading(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.ErrorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		userID, ok := middlewares.UserIDFrom(r.Context())
		if !ok {
			httpx.ErrorJSON(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		limit := 10
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
				limit = parsed
			}
		}

		items, err := storeuserbooks.GetContinueReading(r.Context(), db, userID, limit)
		if err != nil {
			httpx.ErrorJSON(w, http.StatusInternalServerError, "failed to get continue reading")
			return
		}

		httpx.OK(w, items)
	})
}

// AddFavorite: POST /user/favorites/{bookId}
func AddFavorite(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpx.ErrorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		userID, ok := middlewares.UserIDFrom(r.Context())
		if !ok {
			httpx.ErrorJSON(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		bookID := r.PathValue("bookId")
		if bookID == "" {
			httpx.ErrorJSON(w, http.StatusBadRequest, "missing bookId")
			return
		}

		if err := storeuserbooks.AddToFavorites(r.Context(), db, userID, bookID); err != nil {
			httpx.ErrorJSON(w, http.StatusInternalServerError, "failed to add favorite")
			return
		}

		httpx.OKNoData(w)
	})
}

// RemoveFavorite: DELETE /user/favorites/{bookId}
func RemoveFavorite(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			httpx.ErrorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		userID, ok := middlewares.UserIDFrom(r.Context())
		if !ok {
			httpx.ErrorJSON(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		bookID := r.PathValue("bookId")
		if bookID == "" {
			httpx.ErrorJSON(w, http.StatusBadRequest, "missing bookId")
			return
		}

		if err := storeuserbooks.RemoveFromFavorites(r.Context(), db, userID, bookID); err != nil {
			httpx.ErrorJSON(w, http.StatusInternalServerError, "failed to remove favorite")
			return
		}

		httpx.OKNoData(w)
	})
}

// GetFavorites: GET /user/favorites
func GetFavorites(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.ErrorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		userID, ok := middlewares.UserIDFrom(r.Context())
		if !ok {
			httpx.ErrorJSON(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		limit := 20
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
				limit = parsed
			}
		}

		favorites, err := storeuserbooks.GetUserFavorites(r.Context(), db, userID, limit)
		if err != nil {
			httpx.ErrorJSON(w, http.StatusInternalServerError, "failed to get favorites")
			return
		}

		httpx.OK(w, favorites)
	})
}

// AddNote: POST /user/books/{bookId}/notes
func AddNote(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpx.ErrorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		userID, ok := middlewares.UserIDFrom(r.Context())
		if !ok {
			httpx.ErrorJSON(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		bookID := r.PathValue("bookId")
		if bookID == "" {
			httpx.ErrorJSON(w, http.StatusBadRequest, "missing bookId")
			return
		}

		var req struct {
			NoteType     string                 `json:"note_type"`
			Content      string                 `json:"content"`
			PageNumber   *int                   `json:"page_number,omitempty"`
			PositionInfo map[string]interface{} `json:"position_info,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.ErrorJSON(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		if req.NoteType != "note" && req.NoteType != "highlight" {
			httpx.ErrorJSON(w, http.StatusBadRequest, "note_type must be 'note' or 'highlight'")
			return
		}
		if req.Content == "" {
			httpx.ErrorJSON(w, http.StatusBadRequest, "content is required")
			return
		}

		noteID, err := storeuserbooks.AddBookNote(r.Context(), db, userID, bookID, req.NoteType, req.Content, req.PageNumber, req.PositionInfo)
		if err != nil {
			httpx.ErrorJSON(w, http.StatusInternalServerError, "failed to add note")
			return
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "success", "note_id": noteID})
	})
}

// GetNotes: GET /user/books/{bookId}/notes
func GetNotes(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.ErrorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		userID, ok := middlewares.UserIDFrom(r.Context())
		if !ok {
			httpx.ErrorJSON(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		bookID := r.PathValue("bookId")
		if bookID == "" {
			httpx.ErrorJSON(w, http.StatusBadRequest, "missing bookId")
			return
		}

		notes, err := storeuserbooks.GetBookNotes(r.Context(), db, userID, bookID)
		if err != nil {
			httpx.ErrorJSON(w, http.StatusInternalServerError, "failed to get notes")
			return
		}

		httpx.OK(w, notes)
	})
}
