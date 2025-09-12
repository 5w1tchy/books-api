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

func handlePatch(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	defer r.Body.Close()

	id := r.PathValue("key")
	if id == "" || !isUUID(id) {
		apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "id must be a UUID")
		return
	}

	var body struct {
		Title         *string   `json:"title,omitempty"`
		Author        *string   `json:"author,omitempty"`
		CategorySlugs *[]string `json:"categories,omitempty"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "Invalid JSON")
		return
	}
	if body.Title == nil && body.Author == nil && body.CategorySlugs == nil {
		apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "no fields to update")
		return
	}

	if body.Title != nil {
		t, err := validate.RequireBounded("title", *body.Title, 1, 200)
		if err != nil {
			apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", err.Error())
			return
		}
		body.Title = &t
	}
	if body.Author != nil {
		a, err := validate.RequireBounded("author", *body.Author, 1, 120)
		if err != nil {
			apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", err.Error())
			return
		}
		body.Author = &a
	}
	if body.CategorySlugs != nil {
		slugs, err := validate.ValidateCategorySlugs(*body.CategorySlugs, 20)
		if err != nil {
			apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", err.Error())
			return
		}
		body.CategorySlugs = &slugs
	}

	dto := booksrepo.UpdateBookDTO{
		Title:         body.Title,
		Author:        body.Author,
		CategorySlugs: body.CategorySlugs,
	}
	pb, err := booksrepo.Patch(r.Context(), db, id, dto)
	if err != nil {
		switch {
		case errors.Is(err, booksrepo.ErrNotFound):
			apperr.WriteStatus(w, r, http.StatusNotFound, "Not Found", "Book not found")
		case errors.Is(err, booksrepo.ErrInvalid):
			apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "invalid data")
		default:
			apperr.WriteStatus(w, r, http.StatusInternalServerError, "DB error", "update failed")
		}
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": pb})
}
