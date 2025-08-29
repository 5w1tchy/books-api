package models

import "time"

type Book struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`
}
