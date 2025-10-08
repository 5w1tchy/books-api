package books

import (
	"context"
	"database/sql"
)

// DeleteV2 deletes a book and its relationships
func DeleteV2(ctx context.Context, db *sql.DB, key string) error {
	// First get the book ID to ensure it exists
	existing, err := GetAdminBookByKey(ctx, db, key)
	if err != nil {
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

	return tx.Commit()
}
