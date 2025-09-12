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

func handlePut(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	defer r.Body.Close()

	id := r.PathValue("key")
	if id == "" || !isUUID(id) {
		apperr.WriteStatus(w, r, http.StatusBadRequest, "Bad Request", "id must be a UUID")
		return
	}

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
	pb, err := booksrepo.Replace(r.Context(), db, id, dto)
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
