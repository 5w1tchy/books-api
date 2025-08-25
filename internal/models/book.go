package models

type Book struct {
	ID         int    `json:"id"`
	Title      string `json:"title"`
	Author     string `json:"author"`
	CategoryID int    `json:"category_id"` // <- link
}
