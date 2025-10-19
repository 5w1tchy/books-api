package books

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	storage "github.com/5w1tchy/books-api/internal/storage/s3"
)

// GET /books/{key}/audio
func GetBookAudioURLHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		bookKey := r.PathValue("key")

		var objectKey string
		err := db.QueryRowContext(ctx, `
			SELECT audio_key
			FROM books
			WHERE id::text = $1 OR slug = $1
			LIMIT 1
		`, bookKey).Scan(&objectKey)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, `{"error":"book not found"}`, http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}

		r2, err := storage.NewR2Client(ctx)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}

		url, err := r2.GeneratePresignedDownloadURL(ctx, objectKey)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"audio_url": url,
		})
	}
}
