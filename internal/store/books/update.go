package books

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ReplaceV2 replaces all fields of an existing book
func ReplaceV2(ctx context.Context, db *sql.DB, key string, dto CreateBookV2DTO) (AdminBook, error) {
	if err := ValidateAndSanitize(&dto); err != nil {
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
	defer tx.Rollback()

	// Update the book
	createdAt, err := updateBookFields(ctx, tx, existing.ID, dto)
	if err != nil {
		return AdminBook{}, err
	}

	// Clear and rebuild relationships
	if err := ClearBookRelationships(ctx, tx, existing.ID); err != nil {
		return AdminBook{}, err
	}

	if err := LinkAuthors(ctx, tx, existing.ID, dto.Authors); err != nil {
		return AdminBook{}, err
	}

	if err := LinkCategories(ctx, tx, existing.ID, dto.Categories); err != nil {
		return AdminBook{}, err
	}

	if err := tx.Commit(); err != nil {
		return AdminBook{}, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return AdminBook{
		ID:         existing.ID,
		Coda:       dto.Coda,
		Title:      dto.Title,
		Authors:    Dedup(dto.Authors),
		Categories: Dedup(dto.Categories),
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
		Coda:       current.Coda,
		Title:      current.Title,
		Authors:    current.Authors,
		Categories: current.Categories,
		Short:      current.Short,
		Summary:    current.Summary,
	}

	// Apply patches
	if dto.Coda != nil {
		fullDTO.Coda = *dto.Coda
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

// updateBookFields updates the core book fields
func updateBookFields(ctx context.Context, tx *sql.Tx, bookID string, dto CreateBookV2DTO) (time.Time, error) {
	slug := generateSlugFromDTO(dto)

	var createdAt time.Time
	err := tx.QueryRowContext(ctx, `
        UPDATE books 
        SET coda = $1, title = $2, slug = $3, short = $4, summary = $5
        WHERE id = $6
        RETURNING created_at
    `, NullIfEmpty(dto.Coda), dto.Title, slug, NullIfEmpty(dto.Short), NullIfEmpty(dto.Summary), bookID).Scan(&createdAt)

	return createdAt, err
}
