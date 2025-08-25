package router

import (
	"net/http"

	"github.com/5w1tchy/books-api/internal/api/handlers"
)

func Router() http.Handler {

	mux := http.NewServeMux()

	mux.HandleFunc("/", handlers.RootHandler)

	mux.HandleFunc("/books/", handlers.BooksHandler)

	mux.HandleFunc("/users/", handlers.UsersHandler)

	mux.HandleFunc("/categories/", handlers.CategoriesHandler)

	return mux
}
