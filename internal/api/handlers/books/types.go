package books

type PublicBook struct {
	ID            string   `json:"id"`
	ShortID       int64    `json:"short_id"`
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	CategorySlugs []string `json:"category_slugs"`
	// heavy fields (single-book)
	Summary string `json:"summary,omitempty"`
	Short   string `json:"short,omitempty"`
	Coda    string `json:"coda,omitempty"`
}

type CreateBookDTO struct {
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	CategorySlugs []string `json:"category_slugs"`
}

type UpdateBookDTO struct {
	Title         *string   `json:"title,omitempty"`
	Author        *string   `json:"author,omitempty"`
	CategorySlugs *[]string `json:"category_slugs,omitempty"`
}
