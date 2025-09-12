package search

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/5w1tchy/books-api/internal/api/apperr"
)

type SuggestItem struct {
	Type  string  `json:"type"` // "book" | "author"
	Score float64 `json:"-"`    // internal ranking only

	// Common
	Slug  *string `json:"slug,omitempty"`  // book.slug OR author.slug
	Label *string `json:"label,omitempty"` // prebuilt display text for UI
	URL   *string `json:"url,omitempty"`   // /books/{slug} or /authors/{slug}

	// Book fields
	ID         *string `json:"id,omitempty"`
	ShortID    *int64  `json:"short_id,omitempty"`
	Title      *string `json:"title,omitempty"`
	AuthorName *string `json:"authorName,omitempty"`
	AuthorSlug *string `json:"authorSlug,omitempty"`

	// Author fields
	Name       *string `json:"name,omitempty"`
	BooksCount *int    `json:"books_count,omitempty"`
}

func displayKey(it SuggestItem) string {
	if it.Label != nil {
		return *it.Label
	}
	if it.Type == "book" && it.Title != nil && it.AuthorName != nil {
		return *it.Title + " — " + *it.AuthorName
	}
	if it.Name != nil {
		return *it.Name
	}
	if it.Title != nil {
		return *it.Title
	}
	return ""
}

func Suggest(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if len([]rune(q)) < 2 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success", "count": 0, "data": []any{},
			})
			return
		}

		limit := 10
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 20 {
				limit = n
			}
		}

		// default thresholds (short queries are noisier)
		defMin := 0.12
		if len([]rune(q)) <= 3 {
			defMin = 0.08
		}
		minSim := defMin
		if raw := strings.TrimSpace(r.URL.Query().Get("min_sim")); raw != "" {
			if f, err := strconv.ParseFloat(raw, 64); err == nil && f >= 0 && f <= 1 {
				minSim = f
			}
		}

		// optional filters for BOOK suggestions
		var cats []string
		match := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("match")))
		if match != "all" {
			match = "any"
		}
		if csv := strings.TrimSpace(r.URL.Query().Get("categories")); csv != "" {
			for _, s := range strings.Split(csv, ",") {
				if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
					cats = append(cats, s)
				}
			}
		}
		authorFilter := strings.TrimSpace(r.URL.Query().Get("author")) // author slug

		// --- AUTHORS (word-aware both directions) ---
		authRows, err := db.QueryContext(r.Context(), `
WITH iq AS (SELECT public.immutable_unaccent(lower($1)) AS q)
SELECT
  a.slug,
  a.name,
  COUNT(b.id) AS books_count,
  GREATEST(
    similarity(public.immutable_unaccent(lower(a.name)), (SELECT q FROM iq)),
    similarity((SELECT q FROM iq), public.immutable_unaccent(lower(a.name))),
    word_similarity(public.immutable_unaccent(lower(a.name)), (SELECT q FROM iq)),
    word_similarity((SELECT q FROM iq), public.immutable_unaccent(lower(a.name)))
  ) AS score
FROM authors a
LEFT JOIN books b ON b.author_id = a.id
WHERE GREATEST(
    similarity(public.immutable_unaccent(lower(a.name)), (SELECT q FROM iq)),
    similarity((SELECT q FROM iq), public.immutable_unaccent(lower(a.name))),
    word_similarity(public.immutable_unaccent(lower(a.name)), (SELECT q FROM iq)),
    word_similarity((SELECT q FROM iq), public.immutable_unaccent(lower(a.name)))
) >= $2
GROUP BY a.id
ORDER BY score DESC, a.name ASC
LIMIT $3
`, q, minSim, limit)
		if err != nil {
			apperr.WriteStatus(w, r, http.StatusInternalServerError, "DB error", "Failed to search authors")
			return
		}
		defer authRows.Close()

		var authors []SuggestItem
		for authRows.Next() {
			var slug, name string
			var booksCount int
			var score float64
			if err := authRows.Scan(&slug, &name, &booksCount, &score); err != nil {
				apperr.WriteStatus(w, r, http.StatusInternalServerError, "DB scan error", "Failed to read authors")
				return
			}
			lbl := name
			url := "/authors/" + slug
			authors = append(authors, SuggestItem{
				Type:       "author",
				Score:      score,
				Name:       &name,
				Slug:       &slug,
				Label:      &lbl,
				URL:        &url,
				BooksCount: &booksCount,
			})
		}

		// --- BOOKS (title OR author; word-aware both directions) ---
		where := []string{
			`GREATEST(
       similarity(public.immutable_unaccent(lower(b.title)), (SELECT q FROM iq)),
       similarity((SELECT q FROM iq), public.immutable_unaccent(lower(b.title))),
       word_similarity(public.immutable_unaccent(lower(b.title)), (SELECT q FROM iq)),
       word_similarity((SELECT q FROM iq), public.immutable_unaccent(lower(b.title))),
       similarity(public.immutable_unaccent(lower(a.name)), (SELECT q FROM iq)),
       similarity((SELECT q FROM iq), public.immutable_unaccent(lower(a.name))),
       word_similarity(public.immutable_unaccent(lower(a.name)), (SELECT q FROM iq)),
       word_similarity((SELECT q FROM iq), public.immutable_unaccent(lower(a.name)))
     ) >= $2`,
		}
		args := []any{q, minSim}
		i := 3

		if authorFilter != "" {
			where = append(where, "a.slug = $"+strconv.Itoa(i))
			args = append(args, authorFilter)
			i++
		}
		if len(cats) > 0 {
			if match == "any" {
				where = append(where, `
EXISTS (
  SELECT 1
  FROM book_categories bc2
  JOIN categories c2 ON c2.id = bc2.category_id
  WHERE bc2.book_id = b.id AND c2.slug = ANY($`+strconv.Itoa(i)+`::text[])
)`)
			} else {
				where = append(where, `
(
  SELECT COUNT(DISTINCT c2.slug)
  FROM book_categories bc2
  JOIN categories c2 ON c2.id = bc2.category_id
  WHERE bc2.book_id = b.id AND c2.slug = ANY($`+strconv.Itoa(i)+`::text[])
) = `+strconv.Itoa(len(cats)))
			}
			args = append(args, cats)
			i++
		}

		qBooks := `
WITH iq AS (SELECT public.immutable_unaccent(lower($1)) AS q)
SELECT
  b.id,
  b.short_id,
  b.title,
  b.slug,
  a.name AS author_name,
  a.slug AS author_slug,
  GREATEST(
    similarity(public.immutable_unaccent(lower(b.title)), (SELECT q FROM iq)),
    similarity((SELECT q FROM iq), public.immutable_unaccent(lower(b.title))),
    word_similarity(public.immutable_unaccent(lower(b.title)), (SELECT q FROM iq)),
    word_similarity((SELECT q FROM iq), public.immutable_unaccent(lower(b.title))),
    similarity(public.immutable_unaccent(lower(a.name)), (SELECT q FROM iq)),
    similarity((SELECT q FROM iq), public.immutable_unaccent(lower(a.name))),
    word_similarity(public.immutable_unaccent(lower(a.name)), (SELECT q FROM iq)),
    word_similarity((SELECT q FROM iq), public.immutable_unaccent(lower(a.name)))
  ) AS score
FROM books b
JOIN authors a ON a.id = b.author_id
`
		if len(where) > 0 {
			qBooks += "WHERE " + strings.Join(where, " AND ") + "\n"
		}
		qBooks += "ORDER BY score DESC, b.title ASC LIMIT $" + strconv.Itoa(i)
		args = append(args, limit)

		bookRows, err := db.QueryContext(r.Context(), qBooks, args...)
		if err != nil {
			apperr.WriteStatus(w, r, http.StatusInternalServerError, "DB error", "Failed to search books")
			return
		}
		defer bookRows.Close()

		var books []SuggestItem
		for bookRows.Next() {
			var id, bslug, title, aname, aslug string
			var shortID int64
			var score float64
			if err := bookRows.Scan(&id, &shortID, &title, &bslug, &aname, &aslug, &score); err != nil {
				apperr.WriteStatus(w, r, http.StatusInternalServerError, "DB scan error", "Failed to read books")
				return
			}
			lbl := title + " — " + aname
			url := "/books/" + bslug
			books = append(books, SuggestItem{
				Type:       "book",
				Score:      score,
				ID:         &id,
				ShortID:    &shortID,
				Title:      &title,
				Slug:       &bslug,
				Label:      &lbl,
				URL:        &url,
				AuthorName: &aname,
				AuthorSlug: &aslug,
			})
		}

		// merge + take top N
		mixed := append(authors, books...)
		sort.Slice(mixed, func(i, j int) bool {
			if mixed[i].Score == mixed[j].Score {
				if mixed[i].Type != mixed[j].Type {
					return mixed[i].Type == "book"
				}
				return strings.ToLower(displayKey(mixed[i])) < strings.ToLower(displayKey(mixed[j]))
			}
			return mixed[i].Score > mixed[j].Score
		})
		if len(mixed) > limit {
			mixed = mixed[:limit]
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"count":  len(mixed),
			"data":   mixed,
		})
	}
}
