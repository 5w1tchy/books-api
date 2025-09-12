package books

type PublicBook struct {
	ID            string   `json:"id"`
	ShortID       int      `json:"short_id"`
	Slug          string   `json:"slug"`
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	CategorySlugs []string `json:"category_slugs"`
	Summary       string   `json:"summary,omitempty"`
	Short         string   `json:"short,omitempty"`
	Coda          string   `json:"coda,omitempty"`
	URL           string   `json:"url"`
}

type ListFilters struct {
	Q          string
	MinSim     float64
	Author     string
	Categories []string
	Match      string // "any" | "all"
	Limit      int
	Offset     int
}

type CreateBookDTO struct {
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	CategorySlugs []string `json:"categories,omitempty"`
}

type UpdateBookDTO struct {
	Title         *string   `json:"title,omitempty"`
	Author        *string   `json:"author,omitempty"`
	CategorySlugs *[]string `json:"categories,omitempty"`
}
