package foryou

type Limits struct {
	Shorts, Recs, Trending, New int
}

type Fields struct {
	Lite           bool // omit category_slugs
	IncludeSummary bool // include summary (if available) for recs/trending/new
}

type BookLite struct {
	ID            string   `json:"id"`
	Slug          string   `json:"slug"`
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	CategorySlugs []string `json:"category_slugs,omitempty"`
	Summary       string   `json:"summary,omitempty"`
	URL           string   `json:"url"`
}

type ShortItem struct {
	Content string   `json:"content"`
	Book    BookLite `json:"book"`
}

type Sections struct {
	Shorts          []ShortItem `json:"shorts"`
	Recs            []BookLite  `json:"recs"`
	Trending        []BookLite  `json:"trending"`
	New             []BookLite  `json:"new"`
	ContinueReading []BookLite  `json:"continue_reading"`
}
