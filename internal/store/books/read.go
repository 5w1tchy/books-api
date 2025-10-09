package books

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/5w1tchy/books-api/internal/store/shared"
)

// GetAdminBookByKey retrieves a book by ID
func GetAdminBookByKey(ctx context.Context, db *sql.DB, key string) (AdminBook, error) {
	var book AdminBook
	var query string
	var arg interface{}

	// Check if key is UUID
	if len(key) == 36 && strings.Count(key, "-") == 4 {
		query = `
            SELECT id, COALESCE(coda, ''), title, COALESCE(short, ''), COALESCE(summary, ''), created_at
            FROM books WHERE id = $1
        `
		arg = key
	} else {
		query = `
            SELECT id, COALESCE(coda, ''), title, COALESCE(short, ''), COALESCE(summary, ''), created_at
            FROM books WHERE coda = $1
        `
		arg = key
	}

	err := db.QueryRowContext(ctx, query, arg).Scan(
		&book.ID, &book.Coda, &book.Title, &book.Short, &book.Summary, &book.CreatedAt,
	)
	if err != nil {
		return AdminBook{}, err
	}

	// Load relationships using helper functions
	book.Authors, err = LoadAuthorsForBook(ctx, db, book.ID)
	if err != nil {
		return AdminBook{}, err
	}

	book.Categories, err = LoadCategoriesForBook(ctx, db, book.ID)
	if err != nil {
		return AdminBook{}, err
	}

	return book, nil
}

// ListAdminBooks returns paginated books for admin panel
func ListAdminBooks(ctx context.Context, db *sql.DB, filter ListBooksFilter) ([]AdminBook, int, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Size < 1 || filter.Size > 100 {
		filter.Size = 25
	}

	// Build query conditions
	conditions, args := buildAdminListConditions(filter)

	baseQuery := `
        FROM books b
        LEFT JOIN book_authors ba ON b.id = ba.book_id
        LEFT JOIN authors a ON ba.author_id = a.id
        LEFT JOIN book_categories bc ON b.id = bc.book_id
        LEFT JOIN categories c ON bc.category_id = c.id
    `

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total
	total, err := countAdminBooks(ctx, db, baseQuery, whereClause, args)
	if err != nil {
		return nil, 0, err
	}

	// Get books
	books, err := fetchAdminBooks(ctx, db, baseQuery, whereClause, args, filter)
	if err != nil {
		return nil, 0, err
	}

	return books, total, nil
}

// fetchByKey gets a single public book (for reading) - private helper
func fetchByKey(ctx context.Context, db *sql.DB, key string) (PublicBook, error) {
	cond, arg := shared.ResolveBookKeyCondArg(ctx, key)

	q := `
SELECT
    b.id,
    b.short_id,
    b.slug,
    b.title,
    COALESCE(jsonb_agg(DISTINCT a.name) FILTER (WHERE a.name IS NOT NULL), '[]'::jsonb) AS authors,
    COALESCE(jsonb_agg(DISTINCT c.slug) FILTER (WHERE c.slug IS NOT NULL), '[]'::jsonb) AS cat_slugs,
    COALESCE(bo.summary, '') AS summary,
    COALESCE(bo.coda, '')    AS coda
FROM books b
LEFT JOIN book_authors ba ON ba.book_id = b.id
LEFT JOIN authors a       ON a.id = ba.author_id
LEFT JOIN book_categories bc ON bc.book_id = b.id
LEFT JOIN categories c       ON c.id = bc.category_id
LEFT JOIN book_outputs bo    ON bo.book_id = b.id
WHERE ` + cond + `
GROUP BY b.id, b.short_id, b.slug, b.title, bo.summary, bo.coda
`

	var pb PublicBook
	var authorsJSON, catsJSON []byte

	if err := db.QueryRowContext(ctx, q, arg).
		Scan(&pb.ID, &pb.ShortID, &pb.Slug, &pb.Title, &authorsJSON, &catsJSON, &pb.Summary, &pb.Coda); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PublicBook{}, sql.ErrNoRows
		}
		return PublicBook{}, err
	}

	// Decode authors and categories arrays
	_ = json.Unmarshal(authorsJSON, &pb.Authors)
	_ = json.Unmarshal(catsJSON, &pb.CategorySlugs)

	pb.URL = "/books/" + pb.Slug
	pb.Short = "" // ensure short is empty on the book page

	return pb, nil
}

// existsByKey checks if a book exists - private helper
func existsByKey(ctx context.Context, db *sql.DB, key string) (bool, error) {
	cond, arg := shared.ResolveBookKeyCondArg(ctx, key)
	var exists bool
	err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM books b WHERE `+cond+`)`, arg).Scan(&exists)
	return exists, err
}

// Helper functions

func buildAdminListConditions(filter ListBooksFilter) ([]string, []interface{}) {
	var conditions []string
	var args []interface{}
	argIndex := 1

	if filter.Query != "" {
		conditions = append(conditions, fmt.Sprintf("(b.title ILIKE $%d OR a.name ILIKE $%d)", argIndex, argIndex))
		args = append(args, "%"+filter.Query+"%")
		argIndex++
	}

	if filter.Category != "" {
		conditions = append(conditions, fmt.Sprintf("c.name ILIKE $%d", argIndex))
		args = append(args, "%"+filter.Category+"%")
		argIndex++
	}

	if filter.AuthorName != "" {
		conditions = append(conditions, fmt.Sprintf("a.name ILIKE $%d", argIndex))
		args = append(args, "%"+filter.AuthorName+"%")
		argIndex++
	}

	return conditions, args
}

func countAdminBooks(ctx context.Context, db *sql.DB, baseQuery, whereClause string, args []interface{}) (int, error) {
	countQuery := "SELECT COUNT(DISTINCT b.id) " + baseQuery + " " + whereClause
	var total int
	err := db.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	return total, err
}

func fetchAdminBooks(ctx context.Context, db *sql.DB, baseQuery, whereClause string, args []interface{}, filter ListBooksFilter) ([]AdminBook, error) {
	offset := (filter.Page - 1) * filter.Size
	argIndex := len(args) + 1

	listQuery := fmt.Sprintf(`
        SELECT DISTINCT b.id, COALESCE(b.coda, ''), b.title, COALESCE(b.short, ''), COALESCE(b.summary, ''), b.created_at
        %s %s
        ORDER BY b.created_at DESC
        LIMIT $%d OFFSET $%d
    `, baseQuery, whereClause, argIndex, argIndex+1)

	args = append(args, filter.Size, offset)

	rows, err := db.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []AdminBook
	for rows.Next() {
		var book AdminBook
		if err := rows.Scan(&book.ID, &book.Coda, &book.Title, &book.Short, &book.Summary, &book.CreatedAt); err != nil {
			return nil, err
		}

		// Load relationships for each book
		book.Authors, err = LoadAuthorsForBook(ctx, db, book.ID)
		if err != nil {
			return nil, err
		}

		book.Categories, err = LoadCategoriesForBook(ctx, db, book.ID)
		if err != nil {
			return nil, err
		}

		books = append(books, book)
	}

	return books, rows.Err()
}
