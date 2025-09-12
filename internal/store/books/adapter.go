package books

import (
	"context"
	"database/sql"
)

func FetchByKey(ctx context.Context, db *sql.DB, key string) (PublicBook, error) {
	return fetchByKey(ctx, db, key)
}
func ExistsByKey(ctx context.Context, db *sql.DB, key string) (bool, error) {
	return existsByKey(ctx, db, key)
}
