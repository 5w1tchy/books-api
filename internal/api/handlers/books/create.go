package books

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/5w1tchy/books-api/internal/api/apperr"
	"github.com/5w1tchy/books-api/internal/repo/booksrepo"
	"github.com/5w1tchy/books-api/internal/validate"
)

func handleCreate(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	defer r.Body.Close()

	var body struct {
		Title         string   `json:"title"`
		Author        string   `json:"author"`
		CategorySlugs []string `json:"categories,omitempty"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "invalid JSON")
		return
	}

	title, err := validate.RequireBounded("title", body.Title, 1, 200)
	if err != nil {
		apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	authorName, err := validate.RequireBounded("author", body.Author, 1, 120)
	if err != nil {
		apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	if len(body.CategorySlugs) > 0 {
		if _, err := validate.ValidateCategorySlugs(body.CategorySlugs, 20); err != nil {
			apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", err.Error())
			return
		}
	}

	dto := booksrepo.CreateBookDTO{
		Title:         title,
		Author:        authorName,
		CategorySlugs: body.CategorySlugs,
	}
	pb, err := booksrepo.Create(r.Context(), db, dto)
	if err != nil {
		switch {
		case errors.Is(err, booksrepo.ErrInvalid):
			apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "invalid data")
		case errors.Is(err, booksrepo.ErrConflict):
			apperr.WriteStatus(w, r, http.StatusConflict, "Conflict", "duplicate")
		default:
			apperr.WriteStatus(w, r, http.StatusInternalServerError, "DB error", "create failed")
		}
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": pb})
}
