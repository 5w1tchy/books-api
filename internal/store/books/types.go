package books

import (
	"regexp"
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

type CreateBookDTO struct {
	Title         string   `json:"title"`
	Authors       []string `json:"authors"`
	CategorySlugs []string `json:"categories,omitempty"`
}

type UpdateBookDTO struct {
	Title         *string   `json:"title,omitempty"`
	Authors       *[]string `json:"authors,omitempty"`
	CategorySlugs *[]string `json:"categories,omitempty"`
}

var codeRE = regexp.MustCompile(`^[a-z0-9-]{3,64}$`)

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
}

type CreateBookV2DTO struct {
	Coda       string
	Title      string
	Authors    []string // names
	Categories []string // names
	Short      string
	Summary    string
}

type UpdateBookV2DTO struct {
	Coda       *string   `json:"coda,omitempty"`
	Title      *string   `json:"title,omitempty"`
	Authors    *[]string `json:"authors,omitempty"`
	Categories *[]string `json:"categories,omitempty"`
	Short      *string   `json:"short,omitempty"`
	Summary    *string   `json:"summary,omitempty"`
}

type ListBooksFilter struct {
	Query      string // search in title, author names
	Category   string // filter by category name
	AuthorName string // filter by author name
	Page       int
	Size       int
}
