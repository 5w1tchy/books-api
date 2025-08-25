package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/5w1tchy/books-api/internal/models"
)

var (
	books  = make(map[int]models.Book)
	mutex  = &sync.Mutex{}
	nextID = 1
)

func init() {
	books[nextID] = models.Book{
		ID:         nextID,
		Title:      "Book One",
		Author:     "Author A",
		CategoryID: 1,
	}
	nextID++
	books[nextID] = models.Book{
		ID:         nextID,
		Title:      "Book Two",
		Author:     "Author B",
		CategoryID: 2,
	}
	nextID++
	books[nextID] = models.Book{
		ID:         nextID,
		Title:      "Book One",
		Author:     "Author B",
		CategoryID: 1,
	}
	nextID++
}

func BooksHandler(w http.ResponseWriter, r *http.Request) {
	// fmt.Fprintf(w, "Hello Books Route")
	switch r.Method {
	case http.MethodGet:
		getBooksHandler(w, r)
	case http.MethodPost:
		postBooksHandler(w, r)
	case http.MethodPatch:
		w.Write([]byte("Hello PATCH Method on Books route"))
	case http.MethodPut:
		w.Write([]byte("Hello PUT Method on Books route"))
	case http.MethodDelete:
		w.Write([]byte("Hello DELETE Method on Books route"))
	}
}

func getBooksHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/books/")
	path = strings.TrimSuffix(path, "/")

	if path == "" {
		// LIST /books?title=&author=
		title := strings.TrimSpace(r.URL.Query().Get("title"))
		author := strings.TrimSpace(r.URL.Query().Get("author"))

		out := make([]models.Book, 0, len(books))
		for _, b := range books {
			if (title == "" || strings.EqualFold(b.Title, title)) &&
				(author == "" || strings.EqualFold(b.Author, author)) {
				out = append(out, b)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Status string        `json:"status"`
			Count  int           `json:"count"`
			Data   []models.Book `json:"data"`
		}{
			Status: "success",
			Count:  len(out),
			Data:   out,
		})
		return // <-- important
	}

	// GET /books/{id}
	id, err := strconv.Atoi(path)
	if err != nil {
		http.Error(w, "Invalid book ID", http.StatusBadRequest)
		return
	}
	b, ok := books[id]
	if !ok {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(b)
}

func postBooksHandler(w http.ResponseWriter, r *http.Request) {
	// Decode request
	var payload []models.Book
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	mutex.Lock()
	defer mutex.Unlock()

	added := make([]models.Book, 0, len(payload))
	for _, b := range payload {
		b.ID = nextID
		books[nextID] = b // ← now using the global map
		nextID++
		added = append(added, b)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(struct {
		Status string        `json:"status"`
		Count  int           `json:"count"`
		Data   []models.Book `json:"data"`
	}{
		Status: "success",
		Count:  len(added),
		Data:   added,
	})
}
