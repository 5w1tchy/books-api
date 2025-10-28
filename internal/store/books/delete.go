package books

import (
	"context"
	"database/sql"
	"fmt"

	storage "github.com/5w1tchy/books-api/internal/storage/s3"
)

// DeleteV2 deletes a book and its relationships, and cleans up R2 files
func DeleteV2(ctx context.Context, db *sql.DB, key string) error {
	// First get the book ID and file keys to ensure it exists
	existing, err := GetAdminBookByID(ctx, db, key)
	if err != nil {
		return err
	}

	// Get cover_url and audio_key before deletion
	var coverURL, audioKey sql.NullString
	err = db.QueryRowContext(ctx, `
		SELECT cover_url, audio_key FROM books WHERE id = $1
	`, existing.ID).Scan(&coverURL, &audioKey)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear all relationships first
	if err := ClearBookRelationships(ctx, tx, existing.ID); err != nil {
		return err
	}

	// Delete the book itself
	result, err := tx.ExecContext(ctx, `DELETE FROM books WHERE id = $1`, existing.ID)
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

	if err := tx.Commit(); err != nil {
		return err
	}

	// Clean up R2 files (best effort, don't fail if this fails)
	r2, err := storage.NewR2Client(ctx)
	if err == nil {
		if coverURL.Valid && coverURL.String != "" {
			if err := r2.DeleteObject(ctx, coverURL.String); err != nil {
				fmt.Printf("Warning: failed to delete cover %s: %v\n", coverURL.String, err)
			}
		}
		if audioKey.Valid && audioKey.String != "" {
			if err := r2.DeleteObject(ctx, audioKey.String); err != nil {
				fmt.Printf("Warning: failed to delete audio %s: %v\n", audioKey.String, err)
			}
		}
	}

	return nil
}
