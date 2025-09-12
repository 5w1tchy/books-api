package dbx

import (
	"context"
	"database/sql"
)

// Queryer/Execer/Getter let these helpers work with *sql.DB and *sql.Tx.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}
type Getter interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func Query(ctx context.Context, q Queryer, query string, args ...any) (*sql.Rows, error) {
	return q.QueryContext(ctx, query, args...)
}
func Exec(ctx context.Context, e Execer, query string, args ...any) (sql.Result, error) {
	return e.ExecContext(ctx, query, args...)
}
func Get(ctx context.Context, g Getter, query string, args ...any) *sql.Row {
	return g.QueryRowContext(ctx, query, args...)
}

// WithinTx runs fn in a transaction (commit on nil, rollback on error).
func WithinTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// MapPGError is a placeholder for future pg error normalization.
func MapPGError(err error) error { return err }
