package books

import (
	"context"
	"database/sql"
)

// GetAllCategories returns all existing category names for autocomplete
func GetAllCategories(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
        SELECT DISTINCT name FROM categories 
        ORDER BY name
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		categories = append(categories, name)
	}
	return categories, rows.Err()
}

// GetAllAuthors returns all existing author names for autocomplete
func GetAllAuthors(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
        SELECT DISTINCT name FROM authors 
        ORDER BY name
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var authors []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		authors = append(authors, name)
	}
	return authors, rows.Err()
}

// GetCategoriesByPrefix returns categories matching a prefix (for real-time search)
func GetCategoriesByPrefix(ctx context.Context, db *sql.DB, prefix string, limit int) ([]string, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	rows, err := db.QueryContext(ctx, `
        SELECT DISTINCT name FROM categories 
        WHERE name ILIKE $1
        ORDER BY name
        LIMIT $2
    `, prefix+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		categories = append(categories, name)
	}
	return categories, rows.Err()
}

// GetAuthorsByPrefix returns authors matching a prefix (for real-time search)
func GetAuthorsByPrefix(ctx context.Context, db *sql.DB, prefix string, limit int) ([]string, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	rows, err := db.QueryContext(ctx, `
        SELECT DISTINCT name FROM authors 
        WHERE name ILIKE $1
        ORDER BY name
        LIMIT $2
    `, prefix+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var authors []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		authors = append(authors, name)
	}
	return authors, rows.Err()
}
