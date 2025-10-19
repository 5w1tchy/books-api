package books

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	storage "github.com/5w1tchy/books-api/internal/storage/s3"
)

type audioUploadResponse struct {
	UploadURL string `json:"upload_url"`
	ObjectKey string `json:"object_key"`
}

// POST /admin/books/{key}/audio
func GenerateBookAudioURLHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Extract book key from path parameter
		bookKey := r.PathValue("key")

		if bookKey == "" {
			http.Error(w, `{"error":"missing book key"}`, http.StatusBadRequest)
			return
		}

		// Initialize R2 client
		r2, err := storage.NewR2Client(ctx)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}

		// Object path (unique filename)
		objectKey := fmt.Sprintf("books/%s-summary-%d.mp3", bookKey, time.Now().Unix())

		uploadURL, err := r2.GeneratePresignedUploadURL(ctx, objectKey, "audio/mpeg")
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}

		result, err := db.ExecContext(ctx, `
			UPDATE books
			SET audio_key = $1, audio_lang = 'en'
			WHERE id::text = $2 OR slug = $2
		`, objectKey, bookKey)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to save audio key: %v"}`, err), http.StatusInternalServerError)
			return
		}

		rowsAffected, _ := result.RowsAffected()

		if rowsAffected == 0 {
			http.Error(w, `{"error":"book not found"}`, http.StatusNotFound)
			return
		}

		resp := audioUploadResponse{
			UploadURL: uploadURL,
			ObjectKey: objectKey,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
