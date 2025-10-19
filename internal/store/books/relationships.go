package books

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// LinkAuthors creates or finds authors and links them to a book
func LinkAuthors(ctx context.Context, tx *sql.Tx, bookID string, authorNames []string) error {
	for _, name := range Dedup(authorNames) {
		authorID, err := upsertAuthor(ctx, tx, name)
		if err != nil {
			return fmt.Errorf("failed linking author '%s': %w", name, err)
		}

		if err := linkBookAuthor(ctx, tx, bookID, authorID); err != nil {
			return fmt.Errorf("failed to link book to author '%s': %w", name, err)
		}
	}
	return nil
}

// LinkCategories creates or finds categories and links them to a book
func LinkCategories(ctx context.Context, tx *sql.Tx, bookID string, categoryNames []string) error {
	for _, name := range Dedup(categoryNames) {
		categoryID, err := upsertCategory(ctx, tx, name)
		if err != nil {
			return fmt.Errorf("failed linking category '%s': %w", name, err)
		}

		if err := linkBookCategory(ctx, tx, bookID, categoryID); err != nil {
			return fmt.Errorf("failed to link book to category '%s': %w", name, err)
		}
	}
	return nil
}

// ClearBookRelationships removes all author and category links for a book
func ClearBookRelationships(ctx context.Context, tx *sql.Tx, bookID string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM book_authors WHERE book_id = $1`, bookID); err != nil {
		return fmt.Errorf("failed to clear book authors: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM book_categories WHERE book_id = $1`, bookID); err != nil {
		return fmt.Errorf("failed to clear book categories: %w", err)
	}
	return nil
}

// upsertAuthor creates an author if it doesn't exist, returns ID
func upsertAuthor(ctx context.Context, tx *sql.Tx, name string) (string, error) {
	slug := strings.ToLower(GenerateSlug(name))

	var id string
	// Step 1: try to find existing author by name
	err := tx.QueryRowContext(ctx, `
        SELECT id::text FROM authors WHERE name = $1 LIMIT 1
    `, name).Scan(&id)

	if err == nil {
		return id, nil // author exists — return its ID
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("failed to lookup author '%s': %w", name, err)
	}

	// Step 2: insert new one if not found
	err = tx.QueryRowContext(ctx, `
        INSERT INTO authors (name, slug)
        VALUES ($1, $2)
        RETURNING id::text
    `, name, slug).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("failed to create author '%s': %w", name, err)
	}

	return id, nil
}

// upsertCategory creates a category if it doesn't exist, returns ID
func upsertCategory(ctx context.Context, tx *sql.Tx, name string) (string, error) {
	slug := strings.ToLower(GenerateSlug(name))

	var id string
	// Step 1: try to find existing category by name
	err := tx.QueryRowContext(ctx, `
        SELECT id::text FROM categories WHERE name = $1 LIMIT 1
    `, name).Scan(&id)

	if err == nil {
		return id, nil // category already exists
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("failed to lookup category '%s': %w", name, err)
	}

	// Step 2: insert new one if not found
	err = tx.QueryRowContext(ctx, `
        INSERT INTO categories (name, slug)
        VALUES ($1, $2)
        RETURNING id::text
    `, name, slug).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("failed to create category '%s': %w", name, err)
	}

	return id, nil
}

// linkBookAuthor creates the many-to-many relationship
func linkBookAuthor(ctx context.Context, tx *sql.Tx, bookID, authorID string) error {
	_, err := tx.ExecContext(ctx, `
        INSERT INTO book_authors (book_id, author_id)
        VALUES ($1, $2)
        ON CONFLICT DO NOTHING
    `, bookID, authorID)
	return err
}

// linkBookCategory creates the many-to-many relationship
func linkBookCategory(ctx context.Context, tx *sql.Tx, bookID, categoryID string) error {
	_, err := tx.ExecContext(ctx, `
        INSERT INTO book_categories (book_id, category_id)
        VALUES ($1, $2)
        ON CONFLICT DO NOTHING
    `, bookID, categoryID)
	return err
}

// LoadAuthorsForBook loads all authors for a given book ID
func LoadAuthorsForBook(ctx context.Context, db *sql.DB, bookID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
        SELECT a.name FROM authors a
        JOIN book_authors ba ON a.id = ba.author_id
        WHERE ba.book_id = $1
        ORDER BY a.name
    `, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var authors []string
	for rows.Next() {
		var author string
		if err := rows.Scan(&author); err != nil {
			return nil, err
		}
		authors = append(authors, author)
	}
	return authors, rows.Err()
}

// LoadCategoriesForBook loads all categories for a given book ID
func LoadCategoriesForBook(ctx context.Context, db *sql.DB, bookID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
        SELECT c.name FROM categories c
        JOIN book_categories bc ON c.id = bc.category_id
        WHERE bc.book_id = $1
        ORDER BY c.name
    `, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []string
	for rows.Next() {
		var category string
		if err := rows.Scan(&category); err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	return categories, rows.Err()
}
