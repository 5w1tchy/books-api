package booksrepo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func Create(ctx context.Context, db *sql.DB, dto CreateBookDTO) (PublicBook, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return PublicBook{}, err
	}
	defer tx.Rollback()

	aid, _, err := getOrCreateAuthor(tx, dto.Author)
	if err != nil {
		return PublicBook{}, err
	}
	slug, err := ensureUniqueSlug(tx, "books", "slug", slugify(dto.Title), 20)
	if err != nil {
		return PublicBook{}, err
	}

	var id string
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO books (title, slug, author, author_id) VALUES ($1,$2,$3,$4) RETURNING id`,
		dto.Title, slug, dto.Author, aid,
	).Scan(&id); err != nil {
		return PublicBook{}, err
	}

	if len(dto.CategorySlugs) > 0 {
		for _, s := range dedupSlugs(dto.CategorySlugs) {
			res, err := tx.ExecContext(ctx, `
				INSERT INTO book_categories (book_id, category_id)
				SELECT $1, c.id FROM categories c WHERE c.slug = $2`, id, s)
			if err != nil {
				return PublicBook{}, err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return PublicBook{}, fmt.Errorf("%w: unknown category slug: %s", ErrInvalid, s)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return PublicBook{}, err
	}
	return FetchByKey(ctx, db, id)
}

func Patch(ctx context.Context, db *sql.DB, id string, dto UpdateBookDTO) (PublicBook, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return PublicBook{}, err
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM books WHERE id=$1)`, id).Scan(&exists); err != nil {
		return PublicBook{}, err
	}
	if !exists {
		return PublicBook{}, ErrNotFound
	}

	set := []string{}
	args := []any{}
	add := func(s string, v any) { set = append(set, s); args = append(args, v) }

	if dto.Title != nil {
		add("title = $"+strconvItoa(len(args)+1), *dto.Title)
	}
	if dto.Author != nil {
		aid, _, err := getOrCreateAuthor(tx, *dto.Author)
		if err != nil {
			return PublicBook{}, err
		}
		add("author = $"+strconvItoa(len(args)+1), *dto.Author)
		add("author_id = $"+strconvItoa(len(args)+1), aid)
	}
	if len(set) > 0 {
		args = append(args, id)
		q := "UPDATE books SET " + strings.Join(set, ", ") + " WHERE id = $" + strconvItoa(len(args))
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return PublicBook{}, err
		}
	}

	if dto.CategorySlugs != nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM book_categories WHERE book_id = $1`, id); err != nil {
			return PublicBook{}, err
		}
		for _, s := range dedupSlugs(*dto.CategorySlugs) {
			res, err := tx.ExecContext(ctx, `
				INSERT INTO book_categories (book_id, category_id)
				SELECT $1, c.id FROM categories c WHERE c.slug = $2`, id, s)
			if err != nil {
				return PublicBook{}, err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return PublicBook{}, fmt.Errorf("%w: unknown category slug: %s", ErrInvalid, s)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return PublicBook{}, err
	}
	return FetchByKey(ctx, db, id)
}

func Replace(ctx context.Context, db *sql.DB, id string, dto CreateBookDTO) (PublicBook, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return PublicBook{}, err
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM books WHERE id=$1)`, id).Scan(&exists); err != nil {
		return PublicBook{}, err
	}
	if !exists {
		return PublicBook{}, ErrNotFound
	}

	aid, _, err := getOrCreateAuthor(tx, dto.Author)
	if err != nil {
		return PublicBook{}, err
	}
	slug, err := ensureUniqueSlug(tx, "books", "slug", slugify(dto.Title), 20)
	if err != nil {
		return PublicBook{}, err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE books SET title=$1, slug=$2, author=$3, author_id=$4 WHERE id=$5`,
		dto.Title, slug, dto.Author, aid, id); err != nil {
		return PublicBook{}, err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM book_categories WHERE book_id=$1`, id); err != nil {
		return PublicBook{}, err
	}
	for _, s := range dedupSlugs(dto.CategorySlugs) {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO book_categories (book_id, category_id)
			SELECT $1, c.id FROM categories c WHERE c.slug = $2`, id, s)
		if err != nil {
			return PublicBook{}, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return PublicBook{}, fmt.Errorf("%w: unknown category slug: %s", ErrInvalid, s)
		}
	}

	if err := tx.Commit(); err != nil {
		return PublicBook{}, err
	}
	return FetchByKey(ctx, db, id)
}

func Delete(ctx context.Context, db *sql.DB, id string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM book_categories WHERE book_id=$1`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM books WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}

	return tx.Commit()
}
