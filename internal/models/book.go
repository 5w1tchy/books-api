package models

import "time"

type Book struct {
	ID            string    `json:"id"`
	ShortID       int       `json:"short_id"`
	Slug          string    `json:"slug"`
	Title         string    `json:"title"`
	Author        string    `json:"author"`
	CategorySlugs []string  `json:"category_slugs"`
	URL           string    `json:"url,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
