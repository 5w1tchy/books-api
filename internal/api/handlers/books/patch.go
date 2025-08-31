package books

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

func handlePatch(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	idPart := strings.Trim(strings.TrimPrefix(r.URL.Path, "/books/"), "/")
	if idPart == "" {
		http.Error(w, "missing book key", 400)
		return
	}
	cond, arg := resolveBookKey(idPart)

	var dto UpdateBookDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "TX begin failed", 500)
		return
	}
	defer tx.Rollback()

	set := []string{}
	args := []any{}
	if dto.Title != nil {
		t := strings.TrimSpace(*dto.Title)
		if t == "" {
			http.Error(w, "title cannot be empty", 400)
			return
		}
		set = append(set, "title = $"+strconv.Itoa(len(args)+1))
		args = append(args, t)
	}
	if dto.Author != nil {
		a := strings.TrimSpace(*dto.Author)
		if a == "" {
			http.Error(w, "author cannot be empty", 400)
			return
		}
		set = append(set, "author = $"+strconv.Itoa(len(args)+1))
		args = append(args, a)
	}
	if len(set) > 0 {
		args = append(args, arg)
		q := "UPDATE books SET " + strings.Join(set, ", ") + " WHERE " + strings.ReplaceAll(cond, "b.", "")
		if _, err := tx.Exec(q, args...); err != nil {
			http.Error(w, "update failed", 500)
			return
		}
	}

	if dto.CategorySlugs != nil {
		slugs := dedupSlugs(*dto.CategorySlugs)
		if len(slugs) == 0 {
			http.Error(w, "category_slugs cannot be empty", 400)
			return
		}
		var bookID string
		if err := tx.QueryRow("SELECT id FROM books WHERE "+strings.ReplaceAll(cond, "b.", ""), arg).Scan(&bookID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "Book not found", 404)
				return
			}
			http.Error(w, "lookup failed", 500)
			return
		}
		if _, err := tx.Exec(`DELETE FROM book_categories WHERE book_id = $1`, bookID); err != nil {
			http.Error(w, "clear categories failed", 500)
			return
		}
		for _, s := range slugs {
			res, err := tx.Exec(`
				INSERT INTO book_categories (book_id, category_id)
				SELECT $1, c.id FROM categories c WHERE c.slug = $2`, bookID, s)
			if err != nil {
				http.Error(w, "attach category failed", 500)
				return
			}
			if n, _ := res.RowsAffected(); n == 0 {
				http.Error(w, "unknown category slug: "+s, 400)
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "TX commit failed", 500)
		return
	}
	handleGet(db, w, r, idPart)
}
