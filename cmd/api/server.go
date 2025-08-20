package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
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
	// fmt.Fprintf(w, "Hello Root Route")
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
		path := strings.TrimPrefix(r.URL.Path, "/users/")
		userID := strings.TrimSpace(path)
		fmt.Println("User ID:", userID)

		queryParams := r.URL.Query()
		sortby := queryParams.Get("sortby")
		key := queryParams.Get("key")
		sortorder := queryParams.Get("sortorder")

		if sortorder == "" {
			sortorder = "asc"
		}

		fmt.Printf("Sort By: %s, Key: %s, Sort Order: %s\n", sortby, key, sortorder)

		w.Write([]byte("Hello GET Method on Users route"))
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

	http.HandleFunc("/", rootHandler)

	http.HandleFunc("/books/", booksHandler)

	http.HandleFunc("/users/", usersHandler)

	http.HandleFunc("/categories/", categoryHandler)

	fmt.Println("Server is running on port:", port)
	err := http.ListenAndServe(port, nil)
	if err != nil {
		log.Fatalln("Error starting server:", err)
	}
}
