package router

import (
	"database/sql"
	"net/http"

	"github.com/5w1tchy/books-api/internal/api/handlers"
)

func Router(db *sql.DB) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", handlers.RootHandler)
	mux.HandleFunc("/users/", handlers.UsersHandler)

	// pass db to handlers that need it
	mux.HandleFunc("/books/", handlers.BooksHandler(db))
	mux.HandleFunc("/categories/", handlers.CategoriesHandler(db))

	mux.HandleFunc("GET /healthz", handlers.Healthz)
	return mux
}
