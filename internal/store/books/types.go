package books

import (
	"time"
)

type PublicBook struct {
	ID            string   `json:"id"`
	ShortID       int      `json:"short_id"`
	Slug          string   `json:"slug"`
	Title         string   `json:"title"`
	Authors       []string `json:"author"`
	CategorySlugs []string `json:"category_slugs"`
	Summary       string   `json:"summary,omitempty"`
	Short         string   `json:"short,omitempty"`
	Coda          string   `json:"coda,omitempty"`
	URL           string   `json:"url"`
	CoverURL      *string  `json:"cover_url,omitempty"`
}

type ListFilters struct {
	Q          string
	MinSim     float64
	Authors    []string
	Categories []string
	Match      string // "any" | "all"
	Limit      int
	Offset     int
}

// AdminBook is the rich shape returned by CreateV2.
type AdminBook struct {
	ID         string    `json:"id"`
	Coda       string    `json:"coda,omitempty"`
	Title      string    `json:"title"`
	Authors    []string  `json:"authors"`
	Categories []string  `json:"categories"`
	Short      string    `json:"short,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	CoverURL   *string   `json:"cover_url,omitempty"`
}

type CreateBookV2DTO struct {
	Coda       string
	Title      string
	Authors    []string
	Categories []string
	Short      string
	Summary    string
	CoverURL   *string
}

type UpdateBookV2DTO struct {
	Coda       *string   `json:"coda,omitempty"`
	Title      *string   `json:"title,omitempty"`
	Authors    *[]string `json:"authors,omitempty"`
	Categories *[]string `json:"categories,omitempty"`
	Short      *string   `json:"short,omitempty"`
	Summary    *string   `json:"summary,omitempty"`
	CoverURL   *string   `json:"cover_url,omitempty"`
}

type ListBooksFilter struct {
	Query      string // search in title, author names
	Category   string // filter by category name
	AuthorName string // filter by author name
	Page       int
	Size       int
}
