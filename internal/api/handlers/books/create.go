package books

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
)

func handleCreate(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var dto CreateBookDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}
	if strings.TrimSpace(dto.Title) == "" || strings.TrimSpace(dto.Author) == "" || len(dto.CategorySlugs) == 0 {
		http.Error(w, "title, author, category_slugs are required", 400)
		return
	}
	slugs := dedupSlugs(dto.CategorySlugs)
	if len(slugs) == 0 {
		http.Error(w, "category_slugs cannot be empty", 400)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "TX begin failed", 500)
		return
	}
	defer tx.Rollback()

	authorID, authorSlug, err := getOrCreateAuthor(tx, dto.Author)
	if err != nil {
		http.Error(w, "author upsert failed: "+err.Error(), http.StatusConflict)
		return
	}

	base := slugify(dto.Title)
	bookSlug, err := ensureUniqueSlug(tx, "books", "slug", base, 10)
	if err != nil {
		http.Error(w, "slug generation failed: "+err.Error(), http.StatusConflict)
		return
	}

	var id string
	var shortID int64
	if err := tx.QueryRow(`
		INSERT INTO books (title, author, author_id, slug)
		VALUES ($1, $2, $3, $4)
		RETURNING id, short_id`, dto.Title, dto.Author, authorID, bookSlug,
	).Scan(&id, &shortID); err != nil {
		http.Error(w, "insert book failed: "+err.Error(), 500)
		return
	}

	for _, s := range slugs {
		res, err := tx.Exec(`
			INSERT INTO book_categories (book_id, category_id)
			SELECT $1, c.id FROM categories c WHERE c.slug = $2`, id, s)
		if err != nil {
			http.Error(w, "attach category failed", 500)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			http.Error(w, "unknown category slug: "+s, 400)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "TX commit failed", 500)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "success", "id": id, "short_id": shortID, "slug": bookSlug,
		"author_slug": authorSlug, "url": "/books/" + bookSlug,
	})
}
