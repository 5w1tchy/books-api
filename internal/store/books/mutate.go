package books

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/5w1tchy/books-api/internal/store/dbx"
	"github.com/5w1tchy/books-api/internal/store/shared"
)

// Create inserts a new book (creates author & categories if needed) and attaches categories.
func Create(ctx context.Context, db *sql.DB, dto CreateBookDTO) (PublicBook, error) {
	dto.Title = strings.TrimSpace(dto.Title)
	dto.Author = strings.TrimSpace(dto.Author)
	if dto.Title == "" || dto.Author == "" {
		return PublicBook{}, errors.New("title and author are required")
	}
	dto.CategorySlugs = shared.DedupSlugs(dto.CategorySlugs)

	var bookID string
	if err := dbx.WithinTx(ctx, db, func(tx *sql.Tx) error {
		authorID, _, err := ensureAuthor(tx, dto.Author)
		if err != nil {
			return err
		}

		base := shared.Slugify(dto.Title)
		slug, err := shared.EnsureUniqueSlug(tx, "books", "slug", base, 200)
		if err != nil {
			return err
		}

		if err := tx.QueryRowContext(ctx,
			`INSERT INTO books (title, author_id, slug) VALUES ($1,$2,$3) RETURNING id`,
			dto.Title, authorID, slug,
		).Scan(&bookID); err != nil {
			return err
		}

		if len(dto.CategorySlugs) > 0 {
			if err := upsertAndAttachCategories(tx, bookID, dto.CategorySlugs); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return PublicBook{}, err
	}

	return fetchByKey(ctx, db, bookID)
}

// Patch updates provided fields; categories replaced only if the slice pointer is non-nil.
func Patch(ctx context.Context, db *sql.DB, key string, dto UpdateBookDTO) (PublicBook, error) {
	id, err := getBookIDByKey(ctx, db, key)
	if err != nil {
		return PublicBook{}, err
	}

	if err := dbx.WithinTx(ctx, db, func(tx *sql.Tx) error {
		// Title (+ slug if base changes)
		if dto.Title != nil {
			newTitle := strings.TrimSpace(*dto.Title)
			if newTitle == "" {
				return errors.New("title cannot be empty")
			}

			var currentSlug string
			if err := tx.QueryRowContext(ctx, `SELECT slug FROM books WHERE id=$1`, id).Scan(&currentSlug); err != nil {
				return err
			}
			base := shared.Slugify(newTitle)
			newSlug := currentSlug
			if base != currentSlug {
				var err2 error
				newSlug, err2 = shared.EnsureUniqueSlug(tx, "books", "slug", base, 200)
				if err2 != nil {
					return err2
				}
			}
			if _, err := tx.ExecContext(ctx, `UPDATE books SET title=$1, slug=$2 WHERE id=$3`, newTitle, newSlug, id); err != nil {
				return err
			}
		}

		// Author
		if dto.Author != nil {
			name := strings.TrimSpace(*dto.Author)
			if name == "" {
				return errors.New("author cannot be empty")
			}
			authorID, _, err := ensureAuthor(tx, name)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE books SET author_id=$1 WHERE id=$2`, authorID, id); err != nil {
				return err
			}
		}

		// Categories (replace iff provided) — auto-create missing slugs
		if dto.CategorySlugs != nil {
			slugs := shared.DedupSlugs(*dto.CategorySlugs)
			if err := upsertAndAttachCategories(tx, id, slugs); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return PublicBook{}, err
	}

	return fetchByKey(ctx, db, id)
}

// Replace sets all main fields (title, author, categories) using CreateBookDTO semantics.
func Replace(ctx context.Context, db *sql.DB, key string, dto CreateBookDTO) (PublicBook, error) {
	id, err := getBookIDByKey(ctx, db, key)
	if err != nil {
		return PublicBook{}, err
	}

	dto.Title = strings.TrimSpace(dto.Title)
	dto.Author = strings.TrimSpace(dto.Author)
	if dto.Title == "" || dto.Author == "" {
		return PublicBook{}, errors.New("title and author are required")
	}
	dto.CategorySlugs = shared.DedupSlugs(dto.CategorySlugs)

	if err := dbx.WithinTx(ctx, db, func(tx *sql.Tx) error {
		authorID, _, err := ensureAuthor(tx, dto.Author)
		if err != nil {
			return err
		}

		base := shared.Slugify(dto.Title)
		newSlug, err := shared.EnsureUniqueSlug(tx, "books", "slug", base, 200)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE books SET title=$1, author_id=$2, slug=$3 WHERE id=$4`,
			dto.Title, authorID, newSlug, id,
		); err != nil {
			return err
		}
		if err := upsertAndAttachCategories(tx, id, dto.CategorySlugs); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return PublicBook{}, err
	}

	return fetchByKey(ctx, db, id)
}

func Delete(ctx context.Context, db *sql.DB, key string) error {
	id, err := getBookIDByKey(ctx, db, key)
	if err != nil {
		return err
	}

	return dbx.WithinTx(ctx, db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM book_categories WHERE book_id=$1`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM book_outputs   WHERE book_id=$1`, id); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM books WHERE id=$1`, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return sql.ErrNoRows
		}
		return nil
	})
}

// --- helpers (store/books internal) ---

func getBookIDByKey(ctx context.Context, db *sql.DB, key string) (string, error) {
	cond, arg := shared.ResolveBookKeyCondArg(ctx, key)
	var id string
	err := db.QueryRowContext(ctx, `SELECT b.id FROM books b WHERE `+cond, arg).Scan(&id)
	return id, err
}

func ensureAuthor(tx *sql.Tx, name string) (authorID string, created bool, err error) {
	if err = tx.QueryRow(`SELECT id FROM authors WHERE lower(name)=lower($1)`, name).Scan(&authorID); err == nil {
		return authorID, false, nil
	}
	if err != sql.ErrNoRows {
		return "", false, err
	}
	base := shared.Slugify(name)
	slug, err := shared.EnsureUniqueSlug(tx, "authors", "slug", base, 200)
	if err != nil {
		return "", false, err
	}
	if err = tx.QueryRow(`INSERT INTO authors (name, slug) VALUES ($1,$2) RETURNING id`, name, slug).Scan(&authorID); err != nil {
		return "", false, err
	}
	return authorID, true, nil
}

// NEW: auto-create categories and attach to the book (replaces previous replaceBookCategories)
func upsertAndAttachCategories(tx *sql.Tx, bookID string, slugs []string) error {
	// Clear existing relations
	if _, err := tx.Exec(`DELETE FROM book_categories WHERE book_id=$1`, bookID); err != nil {
		return err
	}
	if len(slugs) == 0 {
		return nil
	}

	// fetch existing
	rows, err := tx.Query(`SELECT id, slug FROM categories WHERE slug = ANY($1::text[])`, slugs)
	if err != nil {
		return err
	}
	defer rows.Close()

	idBySlug := map[string]string{}
	for rows.Next() {
		var id, slug string
		if err := rows.Scan(&id, &slug); err != nil {
			return err
		}
		idBySlug[slug] = id
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// insert missing (simple loop; relies on UNIQUE(categories.slug))
	for _, s := range slugs {
		if _, ok := idBySlug[s]; ok {
			continue
		}
		name := prettifySlug(s)
		var id string
		// upsert to be safe if unique exists
		if err := tx.QueryRow(
			`INSERT INTO categories (slug, name) VALUES ($1,$2)
             ON CONFLICT (slug) DO UPDATE SET name=EXCLUDED.name
             RETURNING id`, s, name,
		).Scan(&id); err != nil {
			return err
		}
		idBySlug[s] = id
	}

	// attach
	for _, s := range slugs {
		if id, ok := idBySlug[s]; ok {
			if _, err := tx.Exec(
				`INSERT INTO book_categories (book_id, category_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
				bookID, id,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func prettifySlug(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "-", " "))
	if s == "" {
		return "N/A"
	}
	parts := strings.Fields(s)
	for i := range parts {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + strings.ToLower(parts[i][1:])
		}
	}
	return strings.Join(parts, " ")
}
