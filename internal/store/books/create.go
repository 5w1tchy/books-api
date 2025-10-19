package books

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CreateV2 inserts a book with rich fields, upserts authors & categories, and returns the full record
func CreateV2(ctx context.Context, db *sql.DB, dto CreateBookV2DTO) (AdminBook, error) {
	if err := ValidateAndSanitize(&dto); err != nil {
		return AdminBook{}, err
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return AdminBook{}, err
	}
	defer tx.Rollback()

	book, err := insertBook(ctx, tx, dto)
	if err != nil {
		return AdminBook{}, err
	}

	if err := LinkAuthors(ctx, tx, book.ID, dto.Authors); err != nil {
		return AdminBook{}, err
	}

	if err := LinkCategories(ctx, tx, book.ID, dto.Categories); err != nil {
		return AdminBook{}, err
	}

	if err := tx.Commit(); err != nil {
		return AdminBook{}, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return book, nil
}

// insertBook inserts the main book record and returns basic AdminBook
func insertBook(ctx context.Context, tx *sql.Tx, dto CreateBookV2DTO) (AdminBook, error) {
	slug := generateSlugFromDTO(dto)

	var bookID string
	var createdAt time.Time

	err := tx.QueryRowContext(ctx, `
        INSERT INTO books (coda, title, slug, short, summary, cover_url)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING id::text, created_at
    `,
		NullIfEmpty(dto.Coda),
		dto.Title,
		slug,
		NullIfEmpty(dto.Short),
		NullIfEmpty(dto.Summary),
		dto.CoverURL, // ✅ added
	).Scan(&bookID, &createdAt)

	if err != nil {
		if IsUniqueViolation(err) {
			return AdminBook{}, fmt.Errorf("coda_exists: %w", err)
		}
		return AdminBook{}, err
	}

	return AdminBook{
		ID:         bookID,
		Coda:       dto.Coda,
		Title:      dto.Title,
		Authors:    Dedup(dto.Authors),
		Categories: Dedup(dto.Categories),
		Short:      dto.Short,
		Summary:    dto.Summary,
		CoverURL:   dto.CoverURL,
		CreatedAt:  createdAt,
	}, nil
}
