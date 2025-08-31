package router

import (
	"database/sql"
	"net/http"

	"github.com/5w1tchy/books-api/internal/api/handlers"
	"github.com/5w1tchy/books-api/internal/api/handlers/books"
)

func Router(db *sql.DB) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", handlers.RootHandler)
	mux.HandleFunc("/users/", handlers.UsersHandler)

	// Books: redirect /books -> /books/
	mux.HandleFunc("/books", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/books/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/books/", books.Handler(db))

	// Categories: redirect /categories -> /categories/
	mux.HandleFunc("/categories", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/categories/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/categories/", handlers.CategoriesHandler(db))

	// Authors: redirect /authors -> /authors/
	mux.HandleFunc("/authors", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/authors/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/authors/", handlers.AuthorsHandler(db))

	// Health
	mux.HandleFunc("GET /healthz", handlers.Healthz)

	return mux
}
