package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"

	mw "github.com/5w1tchy/books-api/internal/api/middlewares"
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
	fmt.Println("Hello Root Route")
}

func booksHandler(w http.ResponseWriter, r *http.Request) {
	// fmt.Fprintf(w, "Hello Books Route")
	switch r.Method {
	case http.MethodGet:
		// Handle GET request
		w.Write([]byte("Hello GET Method on Books route"))
		fmt.Println("Hello GET Method on Books route")
	case http.MethodPost:
		// Handle POST request
		w.Write([]byte("Hello POST Method on Books route"))
		fmt.Println("Hello POST Method on Books route")
	case http.MethodPatch:
		// Handle PATCH request
		w.Write([]byte("Hello PATCH Method on Books route"))
		fmt.Println("Hello PATCH Method on Books route")
	case http.MethodPut:
		// Handle PUT request
		w.Write([]byte("Hello PUT Method on Books route"))
		fmt.Println("Hello PUT Method on Books route")
	case http.MethodDelete:
		// Handle DELETE request
		w.Write([]byte("Hello DELETE Method on Books route"))
		fmt.Println("Hello DELETE Method on Books route")
	}
}

func usersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Handle GET request
		w.Write([]byte("Hello GET Method on Users route"))
		fmt.Println("Hello GET Method on Users route")
	case http.MethodPost:
		// Handle POST request
		w.Write([]byte("Hello POST Method on Users route"))
		fmt.Println("Hello POST Method on Users route")
	case http.MethodPatch:
		// Handle PATCH request
		w.Write([]byte("Hello PATCH Method on Users route"))
		fmt.Println("Hello PATCH Method on Users route")
	case http.MethodPut:
		// Handle PUT request
		w.Write([]byte("Hello PUT Method on Users route"))
		fmt.Println("Hello PUT Method on Users route")
	case http.MethodDelete:
		// Handle DELETE request
		w.Write([]byte("Hello DELETE Method on Users route"))
	}
}

func categoryHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Handle GET request
		w.Write([]byte("Hello GET Method on Categories route"))
		fmt.Println("Hello GET Method on Categories route")
	case http.MethodPost:
		// Handle POST request
		w.Write([]byte("Hello POST Method on Categories route"))
		fmt.Println("Hello POST Method on Categories route")
	case http.MethodPatch:
		// Handle PATCH request
		w.Write([]byte("Hello PATCH Method on Categories route"))
		fmt.Println("Hello PATCH Method on Categories route")
	case http.MethodPut:
		// Handle PUT request
		w.Write([]byte("Hello PUT Method on Categories route"))
		fmt.Println("Hello PUT Method on Categories route")
	case http.MethodDelete:
		// Handle DELETE request
		w.Write([]byte("Hello DELETE Method on Categories route"))
		fmt.Println("Hello DELETE Method on Categories route")
	}
}

func main() {

	port := ":3000"

	cert := "cert.pem"
	key := "key.pem"

	mux := http.NewServeMux()

	mux.HandleFunc("/", rootHandler)

	mux.HandleFunc("/books/", booksHandler)

	mux.HandleFunc("/users/", usersHandler)

	mux.HandleFunc("/categories/", categoryHandler)

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12, // Change later to TLS 1.3
	}

	// Create custom server
	server := &http.Server{
		Addr:    port,
		Handler: mw.SecurityHeaders(mw.Cors(mux)),
		// Handler:   mw.CORS(mux),
		TLSConfig: tlsConfig,
	}

	fmt.Println("Server is running on port:", port)
	err := server.ListenAndServeTLS(cert, key)
	if err != nil {
		log.Fatalln("Error starting server:", err)
	}
}
