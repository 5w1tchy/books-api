package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

type PublicBook struct {
	ID            string   `json:"id"`
	ShortID       int64    `json:"short_id"`
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	CategorySlugs []string `json:"category_slugs"`
}

type CreateBookDTO struct {
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	CategorySlugs []string `json:"category_slugs"`
}

type UpdateBookDTO struct {
	Title         *string   `json:"title,omitempty"`
	Author        *string   `json:"author,omitempty"`
	CategorySlugs *[]string `json:"category_slugs,omitempty"`
}

func BooksHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getBooks(db, w, r)
		case http.MethodPost:
			postBook(db, w, r)
		case http.MethodPatch:
			patchBook(db, w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func getBooks(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	idPart := strings.Trim(strings.TrimPrefix(r.URL.Path, "/books/"), "/")
	w.Header().Set("Content-Type", "application/json")

	// LIST
	if idPart == "" {
		rows, err := db.Query(`
			SELECT b.id, b.short_id, b.title, b.author,
			       COALESCE(json_agg(c.slug) FILTER (WHERE c.slug IS NOT NULL), '[]') AS category_slugs
			FROM books b
			LEFT JOIN book_categories bc ON bc.book_id = b.id
			LEFT JOIN categories c       ON c.id = bc.category_id
			GROUP BY b.id
			ORDER BY b.created_at DESC`)
		if err != nil {
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var out []PublicBook
		for rows.Next() {
			var pb PublicBook
			var slugsJSON []byte
			if err := rows.Scan(&pb.ID, &pb.ShortID, &pb.Title, &pb.Author, &slugsJSON); err != nil {
				http.Error(w, "DB scan error", http.StatusInternalServerError)
				return
			}
			_ = json.Unmarshal(slugsJSON, &pb.CategorySlugs)
			out = append(out, pb)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "count": len(out), "data": out})
		return
	}

	// GET by UUID or numeric short_id
	cond := "b.id = $1"
	var arg any = idPart
	if isDigits(idPart) {
		n, err := strconv.ParseInt(idPart, 10, 64)
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		cond, arg = "b.short_id = $1", n
	}

	var pb PublicBook
	var slugsJSON []byte
	err := db.QueryRow(`
		SELECT b.id, b.short_id, b.title, b.author,
		       COALESCE(json_agg(c.slug) FILTER (WHERE c.slug IS NOT NULL), '[]')
		FROM books b
		LEFT JOIN book_categories bc ON bc.book_id = b.id
		LEFT JOIN categories c       ON c.id = bc.category_id
		WHERE `+cond+`
		GROUP BY b.id`, arg).
		Scan(&pb.ID, &pb.ShortID, &pb.Title, &pb.Author, &slugsJSON)

	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	_ = json.Unmarshal(slugsJSON, &pb.CategorySlugs)
	_ = json.NewEncoder(w).Encode(pb)
}

func postBook(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	var dto CreateBookDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(dto.Title) == "" || strings.TrimSpace(dto.Author) == "" || len(dto.CategorySlugs) == 0 {
		http.Error(w, "title, author, category_slugs are required", http.StatusBadRequest)
		return
	}
	uniq := dedupSlugs(dto.CategorySlugs)
	if len(uniq) == 0 {
		http.Error(w, "category_slugs cannot be empty", http.StatusBadRequest)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "TX begin failed", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var id string
	var shortID int64
	if err := tx.QueryRow(`INSERT INTO books (title, author) VALUES ($1, $2) RETURNING id, short_id`, dto.Title, dto.Author).Scan(&id, &shortID); err != nil {
		http.Error(w, "Insert book failed", http.StatusInternalServerError)
		return
	}
	for _, slug := range uniq {
		res, err := tx.Exec(`
			INSERT INTO book_categories (book_id, category_id)
			SELECT $1, c.id FROM categories c WHERE c.slug = $2`, id, slug)
		if err != nil {
			http.Error(w, "Attach category failed", http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			http.Error(w, "Unknown category slug: "+slug, http.StatusBadRequest)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "TX commit failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "id": id, "short_id": shortID})
}

func patchBook(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	idPart := strings.Trim(strings.TrimPrefix(r.URL.Path, "/books/"), "/")

	var dto UpdateBookDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "TX begin failed", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	cond := "id = $1"
	var arg any = idPart
	if isDigits(idPart) {
		n, err := strconv.ParseInt(idPart, 10, 64)
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		cond, arg = "short_id = $1", n
	}

	// Update title/author if provided
	set := []string{}
	args := []any{}
	if dto.Title != nil {
		t := strings.TrimSpace(*dto.Title)
		if t == "" {
			http.Error(w, "title cannot be empty", http.StatusBadRequest)
			return
		}
		set = append(set, "title = $"+strconv.Itoa(len(args)+1))
		args = append(args, t)
	}
	if dto.Author != nil {
		a := strings.TrimSpace(*dto.Author)
		if a == "" {
			http.Error(w, "author cannot be empty", http.StatusBadRequest)
			return
		}
		set = append(set, "author = $"+strconv.Itoa(len(args)+1))
		args = append(args, a)
	}
	if len(set) > 0 {
		args = append(args, arg)
		q := "UPDATE books SET " + strings.Join(set, ", ") + " WHERE " + cond
		if _, err := tx.Exec(q, args...); err != nil {
			http.Error(w, "Update book failed", http.StatusInternalServerError)
			return
		}
	}

	// Replace categories if provided
	if dto.CategorySlugs != nil {
		slugs := dedupSlugs(*dto.CategorySlugs)
		if len(slugs) == 0 {
			http.Error(w, "category_slugs cannot be empty", http.StatusBadRequest)
			return
		}

		var bookID string
		if err := tx.QueryRow("SELECT id FROM books WHERE "+cond, arg).Scan(&bookID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "Book not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Lookup failed", http.StatusInternalServerError)
			return
		}

		if _, err := tx.Exec(`DELETE FROM book_categories WHERE book_id = $1`, bookID); err != nil {
			http.Error(w, "Clear categories failed", http.StatusInternalServerError)
			return
		}
		for _, slug := range slugs {
			res, err := tx.Exec(`
				INSERT INTO book_categories (book_id, category_id)
				SELECT $1, c.id FROM categories c WHERE c.slug = $2`, bookID, slug)
			if err != nil {
				http.Error(w, "Attach category failed", http.StatusInternalServerError)
				return
			}
			if n, _ := res.RowsAffected(); n == 0 {
				http.Error(w, "Unknown category slug: "+slug, http.StatusBadRequest)
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "TX commit failed", http.StatusInternalServerError)
		return
	}

	// return updated book
	getBooks(db, w, r)
}

// helpers

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func dedupSlugs(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
