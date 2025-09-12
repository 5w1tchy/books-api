package booksrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

func FetchByKey(ctx context.Context, db *sql.DB, key string) (PublicBook, error) {
	cond, arg := resolveBookKeyCondArg(key)
	q := `
	SELECT
		b.id, b.short_id, b.slug, b.title, a.name,
		COALESCE(json_agg(c.slug) FILTER (WHERE c.slug IS NOT NULL), '[]'),
		COALESCE(bo.summary, ''), COALESCE(bo.short, ''), COALESCE(bo.coda, '')
	FROM books b
	JOIN authors a               ON a.id = b.author_id
	LEFT JOIN book_categories bc ON bc.book_id = b.id
	LEFT JOIN categories c        ON c.id = bc.category_id
	LEFT JOIN book_outputs bo     ON bo.book_id = b.id
	WHERE ` + cond + `
	GROUP BY b.id, b.short_id, b.slug, b.title, a.name, bo.summary, bo.short, bo.coda`

	var pb PublicBook
	var slugsJSON []byte
	if err := db.QueryRowContext(ctx, q, arg).
		Scan(&pb.ID, &pb.ShortID, &pb.Slug, &pb.Title, &pb.Author, &slugsJSON, &pb.Summary, &pb.Short, &pb.Coda); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PublicBook{}, ErrNotFound
		}
		return PublicBook{}, err
	}
	_ = json.Unmarshal(slugsJSON, &pb.CategorySlugs)
	pb.URL = "/books/" + pb.Slug
	return pb, nil
}

func ExistsByKey(ctx context.Context, db *sql.DB, key string) (bool, error) {
	cond, arg := resolveBookKeyCondArg(key)
	var exists bool
	err := db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM books b WHERE `+cond+`)`, arg).
		Scan(&exists)
	return exists, err
}
