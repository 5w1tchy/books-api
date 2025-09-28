package books

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// AdminBook is the rich shape returned by CreateV2.
type AdminBook struct {
	ID         string    `json:"id"`
	Code       string    `json:"code,omitempty"`
	Title      string    `json:"title"`
	Authors    []string  `json:"authors"`
	Categories []string  `json:"categories"`
	Short      string    `json:"short,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type CreateBookV2DTO struct {
	Code       string
	Title      string
	Authors    []string // names
	Categories []string // names
	Short      string
	Summary    string
}

var codeRE = regexp.MustCompile(`^[a-z0-9-]{3,64}$`)

// CreateV2 inserts a book with rich fields, upserts authors & categories, and returns the full record.
func CreateV2(ctx context.Context, db *sql.DB, dto CreateBookV2DTO) (AdminBook, error) {
	trimAll(&dto)
	if err := validateV2(dto); err != nil {
		return AdminBook{}, err
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return AdminBook{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		bookID    string
		createdAt time.Time
	)
	err = tx.QueryRowContext(ctx, `
		INSERT INTO public.books (code, title, short, summary)
		VALUES ($1,$2,$3,$4)
		RETURNING id::text, created_at
	`, nullIfEmpty(dto.Code), dto.Title, nullIfEmpty(dto.Short), nullIfEmpty(dto.Summary)).Scan(&bookID, &createdAt)
	if err != nil {
		if isUniqueViolation(err) {
			return AdminBook{}, fmt.Errorf("code_exists: %w", err)
		}
		return AdminBook{}, err
	}

	// Authors
	authNames := dedup(dto.Authors)
	for _, name := range authNames {
		var aid string
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO public.authors (name) VALUES ($1)
			ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id::text
		`, name).Scan(&aid); err != nil {
			return AdminBook{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO public.book_authors (book_id, author_id)
			VALUES ($1,$2) ON CONFLICT DO NOTHING
		`, bookID, aid); err != nil {
			return AdminBook{}, err
		}
	}

	// Categories
	catNames := dedup(dto.Categories)
	for _, name := range catNames {
		var cid string
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO public.categories (name) VALUES ($1)
			ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id::text
		`, name).Scan(&cid); err != nil {
			return AdminBook{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO public.book_categories (book_id, category_id)
			VALUES ($1,$2) ON CONFLICT DO NOTHING
		`, bookID, cid); err != nil {
			return AdminBook{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return AdminBook{}, err
	}

	return AdminBook{
		ID:         bookID,
		Code:       dto.Code,
		Title:      dto.Title,
		Authors:    authNames,
		Categories: catNames,
		Short:      dto.Short,
		Summary:    dto.Summary,
		CreatedAt:  createdAt,
	}, nil
}

// -------- helpers --------

func trimAll(dto *CreateBookV2DTO) {
	dto.Code = strings.TrimSpace(dto.Code)
	dto.Title = strings.TrimSpace(dto.Title)
	dto.Short = strings.TrimSpace(dto.Short)
	dto.Summary = strings.TrimSpace(dto.Summary)
	for i := range dto.Authors {
		dto.Authors[i] = strings.TrimSpace(dto.Authors[i])
	}
	for i := range dto.Categories {
		dto.Categories[i] = strings.TrimSpace(dto.Categories[i])
	}
}

func validateV2(in CreateBookV2DTO) error {
	if len(in.Title) == 0 || len(in.Title) > 200 {
		return errors.New("title must be 1..200 chars")
	}
	if in.Code != "" && !codeRE.MatchString(in.Code) {
		return errors.New("code must match ^[a-z0-9-]{3,64}$")
	}
	if len(in.Authors) < 1 || len(in.Authors) > 20 {
		return errors.New("authors must have 1..20 items")
	}
	if len(in.Categories) < 1 || len(in.Categories) > 10 {
		return errors.New("categories must have 1..10 items")
	}
	if len(in.Short) > 280 {
		return errors.New("short must be <= 280 chars")
	}
	if len(in.Summary) > 10000 {
		return errors.New("summary too long")
	}
	return nil
}

func dedup(xs []string) []string {
	seen := make(map[string]struct{}, len(xs))
	out := make([]string, 0, len(xs))
	for _, s := range xs {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique")
}
