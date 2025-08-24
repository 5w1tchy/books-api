package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"time"

	mw "github.com/5w1tchy/books-api/internal/api/middlewares"
	"github.com/redis/go-redis/v9"
)

type User struct {
	Email    string `json:"email"`
	Username string `json:"username"`
}

type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type Book struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Authors    []string `json:"authors"`
	CategoryID string   `json:"category_id"` // <- link
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello Root Route"))
}

func booksHandler(w http.ResponseWriter, r *http.Request) {
	// fmt.Fprintf(w, "Hello Books Route")
	switch r.Method {
	case http.MethodGet:
		w.Write([]byte("Hello GET Method on Books route"))
	case http.MethodPost:
		w.Write([]byte("Hello POST Method on Books route"))
	case http.MethodPatch:
		w.Write([]byte("Hello PATCH Method on Books route"))
	case http.MethodPut:
		w.Write([]byte("Hello PUT Method on Books route"))
	case http.MethodDelete:
		w.Write([]byte("Hello DELETE Method on Books route"))
	}
}

func usersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Write([]byte("Hello GET Method on Users route"))
	case http.MethodPost:
		w.Write([]byte("Hello POST Method on Users route"))
	case http.MethodPatch:
		w.Write([]byte("Hello PATCH Method on Users route"))
	case http.MethodPut:
		w.Write([]byte("Hello PUT Method on Users route"))
	case http.MethodDelete:
		// Handle DELETE request
		w.Write([]byte("Hello DELETE Method on Users route"))
	}
}

func categoryHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Write([]byte("Hello GET Method on Categories route"))
	case http.MethodPost:
		w.Write([]byte("Hello POST Method on Categories route"))
	case http.MethodPatch:
		w.Write([]byte("Hello PATCH Method on Categories route"))
	case http.MethodPut:
		w.Write([]byte("Hello PUT Method on Categories route"))
	case http.MethodDelete:
		w.Write([]byte("Hello DELETE Method on Categories route"))
	}
}

func main() {

	port := ":3000"

	cert := "cert.pem"
	key := "key.pem"

	rdb := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	mux := http.NewServeMux()

	mux.HandleFunc("/", rootHandler)

	mux.HandleFunc("/books/", booksHandler)

	mux.HandleFunc("/users/", usersHandler)

	mux.HandleFunc("/categories/", categoryHandler)

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12, // Change later to TLS 1.3
	}

	tb := mw.NewRedisTokenBucket(rdb, 5, 20, mw.PerIPKey("tb"))

	sw := mw.NewRedisSlidingWindow(rdb, 3000, 60*time.Minute, mw.PerIPKey("sw"))

	hppOptions := mw.HPPOptions{
		CheckQuery:                  true,
		CheckBody:                   true,
		CheckBodyOnlyForContentType: "application/x-www-form-urlencoded",
		Whitelist: []string{
			// General / shared
			"id", "user_id", "book_id", "chapter", "page", "limit", "offset",
			"lang", "search", "category", "tags",

			// Books
			"title", "author", "sort", "order",

			// Users
			"username", "email", "password", "token", "session_id",

			// Notes
			"note_id", "content", "created_at", "updated_at",

			// Highlights
			"highlight_id", "text", "color", "created_at",

			// Progress
			"progress_id", "percentage", "last_read_at",
		},
	}

	handler := sw.Middleware(
		tb.Middleware(
			mw.ResponseTimeMiddleware(
				mw.Cors(
					mw.HPP(hppOptions)(
						mw.SecurityHeaders(
							mw.Compression(
								mux,
							),
						),
					),
				),
			),
		),
	)
	// Create custom server
	server := &http.Server{
		Addr:    port,
		Handler: handler,
		// Handler:   mw.CORS(mux),
		TLSConfig: tlsConfig,
	}

	fmt.Println("Server is running on port:", port)
	err := server.ListenAndServeTLS(cert, key)
	if err != nil {
		log.Fatalln("Error starting server:", err)
	}
}
