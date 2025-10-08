package userbooks

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"time"
)

type ReadingProgress struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	BookID          string    `json:"book_id"`
	PageNumber      int       `json:"page_number"`
	ProgressPercent float64   `json:"progress_percent"`
	LastReadAt      time.Time `json:"last_read_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type UserFavorite struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	BookID    string    `json:"book_id"`
	CreatedAt time.Time `json:"created_at"`
}

type UserBookNote struct {
	ID           string                 `json:"id"`
	UserID       string                 `json:"user_id"`
	BookID       string                 `json:"book_id"`
	NoteType     string                 `json:"note_type"` // "note" or "highlight"
	Content      string                 `json:"content"`
	PageNumber   *int                   `json:"page_number,omitempty"`
	PositionInfo map[string]interface{} `json:"position_info,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

type ContinueReadingItem struct {
	BookID          string    `json:"book_id"`
	Title           string    `json:"title"`
	Authors         []string  `json:"authors"`
	Slug            string    `json:"slug"`
	URL             string    `json:"url"`
	PageNumber      int       `json:"page_number"`
	ProgressPercent float64   `json:"progress_percent"`
	LastReadAt      time.Time `json:"last_read_at"`
}

// UpdateReadingProgress creates or updates reading progress (defensive clamp + rounding)
func UpdateReadingProgress(ctx context.Context, db *sql.DB, userID, bookID string, pageNumber int, progressPercent float64) error {
	if pageNumber < 0 {
		pageNumber = 0
	}
	if progressPercent < 0 {
		progressPercent = 0
	} else if progressPercent > 100 {
		progressPercent = 100
	}
	// round to 2 decimals to match DECIMAL(5,2)
	progressPercent = math.Round(progressPercent*100) / 100

	_, err := db.ExecContext(ctx, `
        INSERT INTO public.user_reading_progress (user_id, book_id, page_number, progress_percent, last_read_at, updated_at)
        VALUES ($1, $2, $3, $4, NOW(), NOW())
        ON CONFLICT (user_id, book_id)
        DO UPDATE SET 
            page_number = EXCLUDED.page_number,
            progress_percent = EXCLUDED.progress_percent,
            last_read_at = NOW(),
            updated_at = NOW()
    `, userID, bookID, pageNumber, progressPercent)
	return err
}

// GetReadingProgress gets user's progress for a specific book
func GetReadingProgress(ctx context.Context, db *sql.DB, userID, bookID string) (*ReadingProgress, error) {
	var progress ReadingProgress
	err := db.QueryRowContext(ctx, `
        SELECT id::text, user_id::text, book_id::text, page_number, progress_percent, 
               last_read_at, created_at, updated_at
        FROM public.user_reading_progress
        WHERE user_id = $1 AND book_id = $2
    `, userID, bookID).Scan(
		&progress.ID, &progress.UserID, &progress.BookID, &progress.PageNumber,
		&progress.ProgressPercent, &progress.LastReadAt, &progress.CreatedAt, &progress.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &progress, nil
}

// GetContinueReading gets user's recently read books for "Continue Reading" section
func GetContinueReading(ctx context.Context, db *sql.DB, userID string, limit int) ([]ContinueReadingItem, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	rows, err := db.QueryContext(ctx, `
        SELECT 
            p.book_id::text,
            b.title,
            b.slug,
            p.page_number,
            p.progress_percent,
            p.last_read_at,
            COALESCE(
                jsonb_agg(DISTINCT a.name ORDER BY a.name) FILTER (WHERE a.name IS NOT NULL),
                '[]'::jsonb
            ) AS authors
        FROM public.user_reading_progress p
        JOIN public.books b ON b.id = p.book_id
        LEFT JOIN public.book_authors ba ON ba.book_id = b.id
        LEFT JOIN public.authors a ON a.id = ba.author_id
        WHERE p.user_id = $1 AND p.progress_percent < 100.00
        GROUP BY p.book_id, b.title, b.slug, p.page_number, p.progress_percent, p.last_read_at
        ORDER BY p.last_read_at DESC
        LIMIT $2
    `, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ContinueReadingItem
	for rows.Next() {
		var item ContinueReadingItem
		var authorsJSON []byte

		if err := rows.Scan(&item.BookID, &item.Title, &item.Slug, &item.PageNumber,
			&item.ProgressPercent, &item.LastReadAt, &authorsJSON); err != nil {
			return nil, err
		}

		if len(authorsJSON) > 0 {
			_ = json.Unmarshal(authorsJSON, &item.Authors)
		}
		item.URL = "/books/" + item.Slug

		items = append(items, item)
	}
	return items, rows.Err()
}

// AddToFavorites adds a book to user's favorites
func AddToFavorites(ctx context.Context, db *sql.DB, userID, bookID string) error {
	_, err := db.ExecContext(ctx, `
        INSERT INTO public.user_favorites (user_id, book_id)
        VALUES ($1, $2)
        ON CONFLICT (user_id, book_id) DO NOTHING
    `, userID, bookID)
	return err
}

// RemoveFromFavorites removes a book from user's favorites
func RemoveFromFavorites(ctx context.Context, db *sql.DB, userID, bookID string) error {
	_, err := db.ExecContext(ctx, `
        DELETE FROM public.user_favorites
        WHERE user_id = $1 AND book_id = $2
    `, userID, bookID)
	return err
}

// GetUserFavorites gets user's favorite books
func GetUserFavorites(ctx context.Context, db *sql.DB, userID string, limit int) ([]UserFavorite, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := db.QueryContext(ctx, `
        SELECT id::text, user_id::text, book_id::text, created_at
        FROM public.user_favorites
        WHERE user_id = $1
        ORDER BY created_at DESC
        LIMIT $2
    `, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var favorites []UserFavorite
	for rows.Next() {
		var f UserFavorite
		if err := rows.Scan(&f.ID, &f.UserID, &f.BookID, &f.CreatedAt); err != nil {
			return nil, err
		}
		favorites = append(favorites, f)
	}
	return favorites, rows.Err()
}

// AddBookNote adds a note or highlight for a book
func AddBookNote(ctx context.Context, db *sql.DB, userID, bookID, noteType, content string, pageNumber *int, positionInfo map[string]interface{}) (string, error) {
	var positionJSON []byte
	var err error
	if positionInfo != nil {
		positionJSON, err = json.Marshal(positionInfo)
		if err != nil {
			return "", err
		}
	}

	var noteID string
	err = db.QueryRowContext(ctx, `
        INSERT INTO public.user_book_notes (user_id, book_id, note_type, content, page_number, position_info)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING id::text
    `, userID, bookID, noteType, content, pageNumber, positionJSON).Scan(&noteID)

	return noteID, err
}

// GetBookNotes gets user's notes and highlights for a specific book
func GetBookNotes(ctx context.Context, db *sql.DB, userID, bookID string) ([]UserBookNote, error) {
	rows, err := db.QueryContext(ctx, `
        SELECT id::text, user_id::text, book_id::text, note_type, content, 
               page_number, position_info, created_at, updated_at
        FROM public.user_book_notes
        WHERE user_id = $1 AND book_id = $2
        ORDER BY created_at DESC
    `, userID, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []UserBookNote
	for rows.Next() {
		var n UserBookNote
		var positionJSON []byte
		if err := rows.Scan(&n.ID, &n.UserID, &n.BookID, &n.NoteType, &n.Content,
			&n.PageNumber, &positionJSON, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}

		if positionJSON != nil {
			_ = json.Unmarshal(positionJSON, &n.PositionInfo)
		}

		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// IsBookFavorited checks if a book is in user's favorites
func IsBookFavorited(ctx context.Context, db *sql.DB, userID, bookID string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `
        SELECT EXISTS (
            SELECT 1 FROM public.user_favorites 
            WHERE user_id = $1 AND book_id = $2
        )
    `, userID, bookID).Scan(&exists)
	return exists, err
}
