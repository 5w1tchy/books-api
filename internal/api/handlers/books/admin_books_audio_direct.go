package books

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"time"

	storage "github.com/5w1tchy/books-api/internal/storage/s3"
)

// PUT /admin/books/{key}/audio/upload - Direct upload through backend (CORS workaround)
func DirectAudioUploadHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		bookKey := r.PathValue("key")

		if bookKey == "" {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"missing book key"}`, http.StatusBadRequest)
			return
		}

		// Get existing audio_key to delete old one
		var oldAudioKey sql.NullString
		err := db.QueryRowContext(ctx, `
			SELECT audio_key FROM books WHERE id::text = $1 OR slug = $1
		`, bookKey).Scan(&oldAudioKey)
		
		if err == sql.ErrNoRows {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"book not found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, fmt.Sprintf(`{"error":"database error: %v"}`, err), http.StatusInternalServerError)
			return
		}

		// Parse the multipart form (max 200MB for audio)
		if err := r.ParseMultipartForm(200 << 20); err != nil {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"failed to parse form"}`, http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("audio")
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"missing audio file"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Validate content type
		contentType := header.Header.Get("Content-Type")
		if contentType != "audio/mpeg" && contentType != "audio/mp3" && 
		   contentType != "audio/ogg" && contentType != "audio/wav" {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"invalid audio type"}`, http.StatusBadRequest)
			return
		}

		// Initialize R2 client
		r2, err := storage.NewR2Client(ctx)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, fmt.Sprintf(`{"error":"storage init failed: %v"}`, err), http.StatusInternalServerError)
			return
		}

		// Generate object key
		objectKey := fmt.Sprintf("books/%s-summary-%d.mp3", bookKey, time.Now().Unix())

		// Upload to R2
		if err := uploadFileToR2(ctx, r2, objectKey, file, contentType, header.Size); err != nil {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, fmt.Sprintf(`{"error":"upload failed: %v"}`, err), http.StatusInternalServerError)
			return
		}

		// Update DB
		result, err := db.ExecContext(ctx, `
			UPDATE books
			SET audio_key = $1
			WHERE id::text = $2 OR slug = $2
		`, objectKey, bookKey)
		if err != nil {
			// Cleanup uploaded file
			_ = r2.DeleteObject(ctx, objectKey)
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, fmt.Sprintf(`{"error":"failed to save audio key: %v"}`, err), http.StatusInternalServerError)
			return
		}

		rows, _ := result.RowsAffected()
		if rows == 0 {
			_ = r2.DeleteObject(ctx, objectKey)
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"book not found"}`, http.StatusNotFound)
			return
		}

		// Delete old audio from R2 if it exists
		if oldAudioKey.Valid && oldAudioKey.String != "" {
			if err := r2.DeleteObject(ctx, oldAudioKey.String); err != nil {
				fmt.Printf("⚠️ Warning: failed to delete old audio %s: %v\n", oldAudioKey.String, err)
			} else {
				fmt.Printf("✅ Deleted old audio: %s\n", oldAudioKey.String)
			}
		}

		fmt.Printf("✅ Audio uploaded directly for book %s: %s\n", bookKey, objectKey)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"status":"success","message":"audio uploaded successfully"}`)
	}
}
