package books

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// AdminBook is the rich shape returned by CreateV2.
type AdminBook struct {
	ID         string    `json:"id"`
	Code       string    `json:"code,omitempty"`
	Title      string    `json:"title"`
	Authors    []string  `json:"authors"`
	Categories []string  `json:"categories"`
	Short      string    `json:"short,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type CreateBookV2DTO struct {
	Code       string
	Title      string
	Authors    []string // names
	Categories []string // names
	Short      string
	Summary    string
}

type UpdateBookV2DTO struct {
	Code       *string   `json:"code,omitempty"`
	Title      *string   `json:"title,omitempty"`
	Authors    *[]string `json:"authors,omitempty"`
	Categories *[]string `json:"categories,omitempty"`
	Short      *string   `json:"short,omitempty"`
	Summary    *string   `json:"summary,omitempty"`
}

var codeRE = regexp.MustCompile(`^[a-z0-9-]{3,64}$`)

// CreateV2 inserts a book with rich fields, upserts authors & categories, and returns the full record.
func CreateV2(ctx context.Context, db *sql.DB, dto CreateBookV2DTO) (AdminBook, error) {
	trimAll(&dto)
	if err := validateV2(dto); err != nil {
		return AdminBook{}, err
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return AdminBook{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		bookID    string
		createdAt time.Time
	)

	// Generate slug from title if code is empty, otherwise use code as slug
	slug := dto.Code
	if slug == "" {
		slug = generateSlug(dto.Title)
	}

	err = tx.QueryRowContext(ctx, `
        INSERT INTO public.books (code, title, slug, short, summary)
        VALUES ($1,$2,$3,$4,$5)
        RETURNING id::text, created_at
    `, nullIfEmpty(dto.Code), dto.Title, slug, nullIfEmpty(dto.Short), nullIfEmpty(dto.Summary)).Scan(&bookID, &createdAt)
	if err != nil {
		if isUniqueViolation(err) {
			return AdminBook{}, fmt.Errorf("code_exists: %w", err)
		}
		return AdminBook{}, err
	}

	// Authors
	authNames := dedup(dto.Authors)
	for _, name := range authNames {
		var aid string
		authorSlug := generateSlug(name)

		// First try to get existing author
		err := tx.QueryRowContext(ctx, `
            SELECT id::text FROM public.authors WHERE name = $1
        `, name).Scan(&aid)

		if err == sql.ErrNoRows {
			// Author doesn't exist, create it
			if err := tx.QueryRowContext(ctx, `
                INSERT INTO public.authors (name, slug) VALUES ($1, $2)
                RETURNING id::text
            `, name, authorSlug).Scan(&aid); err != nil {
				return AdminBook{}, fmt.Errorf("failed to create author '%s': %w", name, err)
			}
		} else if err != nil {
			return AdminBook{}, fmt.Errorf("failed to lookup author '%s': %w", name, err)
		}

		if _, err := tx.ExecContext(ctx, `
            INSERT INTO public.book_authors (book_id, author_id)
            VALUES ($1,$2)
        `, bookID, aid); err != nil {
			// Check if it's a duplicate key error (which is OK)
			if !strings.Contains(strings.ToLower(err.Error()), "duplicate") &&
				!strings.Contains(strings.ToLower(err.Error()), "unique") {
				return AdminBook{}, fmt.Errorf("failed to link book to author '%s': %w", name, err)
			}
		}
	}

	// Categories
	catNames := dedup(dto.Categories)
	for _, name := range catNames {
		var cid string
		categorySlug := generateSlug(name)

		// First try to get existing category
		err := tx.QueryRowContext(ctx, `
            SELECT id::text FROM public.categories WHERE name = $1
        `, name).Scan(&cid)

		if err == sql.ErrNoRows {
			// Category doesn't exist, create it
			if err := tx.QueryRowContext(ctx, `
                INSERT INTO public.categories (name, slug) VALUES ($1, $2)
                RETURNING id::text
            `, name, categorySlug).Scan(&cid); err != nil {
				return AdminBook{}, fmt.Errorf("failed to create category '%s': %w", name, err)
			}
		} else if err != nil {
			return AdminBook{}, fmt.Errorf("failed to lookup category '%s': %w", name, err)
		}

		if _, err := tx.ExecContext(ctx, `
            INSERT INTO public.book_categories (book_id, category_id)
            VALUES ($1,$2)
        `, bookID, cid); err != nil {
			// Check if it's a duplicate key error (which is OK)
			if !strings.Contains(strings.ToLower(err.Error()), "duplicate") &&
				!strings.Contains(strings.ToLower(err.Error()), "unique") {
				return AdminBook{}, fmt.Errorf("failed to link book to category '%s': %w", name, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return AdminBook{}, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return AdminBook{
		ID:         bookID,
		Code:       dto.Code,
		Title:      dto.Title,
		Authors:    authNames,
		Categories: catNames,
		Short:      dto.Short,
		Summary:    dto.Summary,
		CreatedAt:  createdAt,
	}, nil
}

// GetAdminBookByKey retrieves a book by ID or code for admin use
func GetAdminBookByKey(ctx context.Context, db *sql.DB, key string) (AdminBook, error) {
	var book AdminBook
	var query string
	var arg interface{}

	// Check if key is UUID (ID) or code
	if len(key) == 36 && strings.Count(key, "-") == 4 {
		// Looks like UUID
		query = `
            SELECT id, COALESCE(code, ''), title, COALESCE(short, ''), COALESCE(summary, ''), created_at
            FROM public.books WHERE id = $1
        `
		arg = key
	} else {
		// Treat as code
		query = `
            SELECT id, COALESCE(code, ''), title, COALESCE(short, ''), COALESCE(summary, ''), created_at
            FROM public.books WHERE code = $1
        `
		arg = key
	}

	err := db.QueryRowContext(ctx, query, arg).Scan(
		&book.ID, &book.Code, &book.Title, &book.Short, &book.Summary, &book.CreatedAt,
	)
	if err != nil {
		return AdminBook{}, err
	}

	// Get authors
	rows, err := db.QueryContext(ctx, `
        SELECT a.name FROM public.authors a
        JOIN public.book_authors ba ON a.id = ba.author_id
        WHERE ba.book_id = $1
        ORDER BY a.name
    `, book.ID)
	if err != nil {
		return AdminBook{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var author string
		if err := rows.Scan(&author); err != nil {
			return AdminBook{}, err
		}
		book.Authors = append(book.Authors, author)
	}

	// Get categories
	rows, err = db.QueryContext(ctx, `
        SELECT c.name FROM public.categories c
        JOIN public.book_categories bc ON c.id = bc.category_id
        WHERE bc.book_id = $1
        ORDER BY c.name
    `, book.ID)
	if err != nil {
		return AdminBook{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var category string
		if err := rows.Scan(&category); err != nil {
			return AdminBook{}, err
		}
		book.Categories = append(book.Categories, category)
	}

	return book, nil
}

// ReplaceV2 replaces all fields of an existing book
func ReplaceV2(ctx context.Context, db *sql.DB, key string, dto CreateBookV2DTO) (AdminBook, error) {
	trimAll(&dto)
	if err := validateV2(dto); err != nil {
		return AdminBook{}, err
	}

	// First get the book ID
	existing, err := GetAdminBookByKey(ctx, db, key)
	if err != nil {
		return AdminBook{}, err
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return AdminBook{}, err
	}
	defer func() { _ = tx.Rollback() }()

	// Generate new slug
	slug := dto.Code
	if slug == "" {
		slug = generateSlug(dto.Title)
	}

	// Update book
	var createdAt time.Time
	err = tx.QueryRowContext(ctx, `
        UPDATE public.books 
        SET code = $1, title = $2, slug = $3, short = $4, summary = $5
        WHERE id = $6
        RETURNING created_at
    `, nullIfEmpty(dto.Code), dto.Title, slug, nullIfEmpty(dto.Short), nullIfEmpty(dto.Summary), existing.ID).Scan(&createdAt)
	if err != nil {
		return AdminBook{}, err
	}

	// Clear existing relationships
	if _, err := tx.ExecContext(ctx, `DELETE FROM public.book_authors WHERE book_id = $1`, existing.ID); err != nil {
		return AdminBook{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM public.book_categories WHERE book_id = $1`, existing.ID); err != nil {
		return AdminBook{}, err
	}

	// Re-add authors
	authNames := dedup(dto.Authors)
	for _, name := range authNames {
		var aid string
		authorSlug := generateSlug(name)

		err := tx.QueryRowContext(ctx, `SELECT id::text FROM public.authors WHERE name = $1`, name).Scan(&aid)
		if err == sql.ErrNoRows {
			if err := tx.QueryRowContext(ctx, `
                INSERT INTO public.authors (name, slug) VALUES ($1, $2)
                RETURNING id::text
            `, name, authorSlug).Scan(&aid); err != nil {
				return AdminBook{}, err
			}
		} else if err != nil {
			return AdminBook{}, err
		}

		if _, err := tx.ExecContext(ctx, `
            INSERT INTO public.book_authors (book_id, author_id) VALUES ($1,$2)
        `, existing.ID, aid); err != nil {
			return AdminBook{}, err
		}
	}

	// Re-add categories
	catNames := dedup(dto.Categories)
	for _, name := range catNames {
		var cid string
		categorySlug := generateSlug(name)

		err := tx.QueryRowContext(ctx, `SELECT id::text FROM public.categories WHERE name = $1`, name).Scan(&cid)
		if err == sql.ErrNoRows {
			if err := tx.QueryRowContext(ctx, `
                INSERT INTO public.categories (name, slug) VALUES ($1, $2)
                RETURNING id::text
            `, name, categorySlug).Scan(&cid); err != nil {
				return AdminBook{}, err
			}
		} else if err != nil {
			return AdminBook{}, err
		}

		if _, err := tx.ExecContext(ctx, `
            INSERT INTO public.book_categories (book_id, category_id) VALUES ($1,$2)
        `, existing.ID, cid); err != nil {
			return AdminBook{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return AdminBook{}, err
	}

	return AdminBook{
		ID:         existing.ID,
		Code:       dto.Code,
		Title:      dto.Title,
		Authors:    authNames,
		Categories: catNames,
		Short:      dto.Short,
		Summary:    dto.Summary,
		CreatedAt:  createdAt,
	}, nil
}

// PatchV2 partially updates a book
func PatchV2(ctx context.Context, db *sql.DB, key string, dto UpdateBookV2DTO) (AdminBook, error) {
	// First get current book
	current, err := GetAdminBookByKey(ctx, db, key)
	if err != nil {
		return AdminBook{}, err
	}

	// Build full DTO with current values + patches
	fullDTO := CreateBookV2DTO{
		Code:       current.Code,
		Title:      current.Title,
		Authors:    current.Authors,
		Categories: current.Categories,
		Short:      current.Short,
		Summary:    current.Summary,
	}

	// Apply patches
	if dto.Code != nil {
		fullDTO.Code = *dto.Code
	}
	if dto.Title != nil {
		fullDTO.Title = *dto.Title
	}
	if dto.Authors != nil {
		fullDTO.Authors = *dto.Authors
	}
	if dto.Categories != nil {
		fullDTO.Categories = *dto.Categories
	}
	if dto.Short != nil {
		fullDTO.Short = *dto.Short
	}
	if dto.Summary != nil {
		fullDTO.Summary = *dto.Summary
	}

	// Use ReplaceV2 with the patched data
	return ReplaceV2(ctx, db, key, fullDTO)
}

// DeleteV2 deletes a book and its relationships
func DeleteV2(ctx context.Context, db *sql.DB, key string) error {
	// First get the book ID
	existing, err := GetAdminBookByKey(ctx, db, key)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Delete relationships first
	if _, err := tx.ExecContext(ctx, `DELETE FROM public.book_authors WHERE book_id = $1`, existing.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM public.book_categories WHERE book_id = $1`, existing.ID); err != nil {
		return err
	}

	// Delete the book
	result, err := tx.ExecContext(ctx, `DELETE FROM public.books WHERE id = $1`, existing.ID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return tx.Commit()
}

// -------- helpers --------

func trimAll(dto *CreateBookV2DTO) {
	dto.Code = sanitizeString(dto.Code)
	dto.Title = sanitizeString(dto.Title)
	dto.Short = sanitizeString(dto.Short)
	dto.Summary = sanitizeString(dto.Summary)
	for i := range dto.Authors {
		dto.Authors[i] = sanitizeString(dto.Authors[i])
	}
	for i := range dto.Categories {
		dto.Categories[i] = sanitizeString(dto.Categories[i])
	}
}

func sanitizeString(s string) string {
	// Trim whitespace
	s = strings.TrimSpace(s)
	// Remove null bytes and control characters
	s = strings.ReplaceAll(s, "\x00", "")
	// Replace multiple spaces with single space
	reg := regexp.MustCompile(`\s+`)
	s = reg.ReplaceAllString(s, " ")
	return s
}

func validateV2(in CreateBookV2DTO) error {
	if len(in.Title) == 0 || len(in.Title) > 200 {
		return errors.New("title must be 1..200 chars")
	}
	if in.Code != "" && !codeRE.MatchString(in.Code) {
		return errors.New("code must match ^[a-z0-9-]{3,64}$")
	}
	if len(in.Authors) < 1 || len(in.Authors) > 20 {
		return errors.New("authors must have 1..20 items")
	}
	if len(in.Categories) < 1 || len(in.Categories) > 10 {
		return errors.New("categories must have 1..10 items")
	}
	if len(in.Short) > 280 {
		return errors.New("short must be <= 280 chars")
	}
	if len(in.Summary) > 10000 {
		return errors.New("summary too long")
	}
	return nil
}

func dedup(xs []string) []string {
	seen := make(map[string]struct{}, len(xs))
	out := make([]string, 0, len(xs))
	for _, s := range xs {
		s = sanitizeString(s)
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

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique")
}

// generateSlug creates a URL-friendly slug from a title
func generateSlug(title string) string {
	// Convert to lowercase and replace spaces/special chars with hyphens
	slug := strings.ToLower(title)
	// Replace non-alphanumeric chars with hyphens
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	slug = reg.ReplaceAllString(slug, "-")
	// Remove leading/trailing hyphens
	slug = strings.Trim(slug, "-")
	// Limit length
	if len(slug) > 64 {
		slug = slug[:64]
	}
	return slug
}
