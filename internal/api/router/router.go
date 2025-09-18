package router

import (
	"database/sql"
	"net/http"

	"github.com/5w1tchy/books-api/internal/api/handlers"
	"github.com/5w1tchy/books-api/internal/api/handlers/books"
	"github.com/5w1tchy/books-api/internal/api/handlers/foryou"
	"github.com/5w1tchy/books-api/internal/api/handlers/search"
	"github.com/redis/go-redis/v9"
)

func Router(db *sql.DB, rdb *redis.Client) http.Handler {
	mux := http.NewServeMux()

	// Root & health
	mux.HandleFunc("GET /", handlers.RootHandler)
	mux.HandleFunc("GET /healthz", handlers.Healthz)
	mux.HandleFunc("HEAD /healthz", handlers.Healthz)

	// Books (no trailing slash; consistent)
	mux.Handle("GET /books", books.Handler(db, rdb))           // list
	mux.Handle("POST /books", books.Handler(db, rdb))          // create
	mux.Handle("GET /books/{key}", books.Handler(db, rdb))     // get
	mux.Handle("HEAD /books/{key}", books.Handler(db, rdb))    // head
	mux.Handle("PATCH /books/{key}", books.Handler(db, rdb))   // patch
	mux.Handle("PUT /books/{key}", books.Handler(db, rdb))     // put
	mux.Handle("DELETE /books/{key}", books.Handler(db, rdb))  // delete
	mux.Handle("OPTIONS /books", books.Handler(db, rdb))       // preflight
	mux.Handle("OPTIONS /books/{key}", books.Handler(db, rdb)) // preflight

	// Search
	mux.Handle("GET /search/suggest", search.Suggest(db))

	// For-You feed
	feed := foryou.Handler(db, rdb)
	mux.Handle("GET /for-you", feed)
	mux.Handle("GET /for-you/", feed)

	return mux
}
