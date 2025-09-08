package router

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/5w1tchy/books-api/internal/api/handlers"
	"github.com/5w1tchy/books-api/internal/api/handlers/books"
	"github.com/5w1tchy/books-api/internal/api/handlers/search"
	"github.com/redis/go-redis/v9"
)

func Router(db *sql.DB, rdb *redis.Client) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", handlers.RootHandler)

	mux.HandleFunc("/users/", handlers.UsersHandler)

	// keep legacy /books (no slash) -> /books/
	mux.HandleFunc("/books", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/books/", http.StatusMovedPermanently)
	})

	// Redirect /books?author=slug (and no other filters) -> /authors/slug
	booksHandler := books.Handler(db)
	mux.HandleFunc("/books/", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		author := strings.TrimSpace(q.Get("author"))

		if author != "" &&
			q.Get("q") == "" &&
			q.Get("categories") == "" &&
			q.Get("match") == "" &&
			q.Get("min_sim") == "" &&
			q.Get("limit") == "" &&
			q.Get("offset") == "" {
			http.Redirect(w, r, "/authors/"+author, http.StatusMovedPermanently)
			return
		}

		booksHandler.ServeHTTP(w, r)
	})

	mux.HandleFunc("/categories", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/categories/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/categories/", handlers.CategoriesHandler(db))

	mux.HandleFunc("/authors", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/authors/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/authors/", handlers.AuthorsHandler(db))

	// new mixed autocomplete
	mux.HandleFunc("GET /search/suggest", search.Suggest(db))

	// redirect
	mux.HandleFunc("/for-you", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/for-you/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/for-you/", handlers.ForYouHandler(db, rdb))

	mux.HandleFunc("GET /healthz", handlers.Healthz)
	return mux
}
