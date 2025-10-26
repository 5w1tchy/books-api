package books

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	storage "github.com/5w1tchy/books-api/internal/storage/s3"
)

// POST /admin/books/{id}/cover
func UploadBookCoverHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		bookID := r.PathValue("id")

		if bookID == "" {
			http.Error(w, `{"error":"missing book id"}`, http.StatusBadRequest)
			return
		}

		// Parse multipart form (max 10MB for images)
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to parse form: %v"}`, err), http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("cover")
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"missing cover file: %v"}`, err), http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Validate content type
		contentType := header.Header.Get("Content-Type")
		if contentType != "image/webp" && contentType != "image/jpeg" && contentType != "image/png" {
			http.Error(w, `{"error":"invalid image type, must be webp, jpeg, or png"}`, http.StatusBadRequest)
			return
		}

		r2, err := storage.NewR2Client(ctx)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}

		// Generate object key
		objectKey := fmt.Sprintf("books/covers/%s-%d.webp", bookID, time.Now().Unix())

		// Upload to R2
		if err := uploadFileToR2(ctx, r2, objectKey, file, contentType, header.Size); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to upload: %v"}`, err), http.StatusInternalServerError)
			return
		}

		// Save in DB
		result, err := db.ExecContext(ctx, `
			UPDATE books
			SET cover_url = $1
			WHERE id::text = $2 OR slug = $2
		`, objectKey, bookID)
		if err != nil {
			// Try to cleanup uploaded file
			_ = r2.DeleteObject(ctx, objectKey)
			http.Error(w, fmt.Sprintf(`{"error":"failed to save cover key: %v"}`, err), http.StatusInternalServerError)
			return
		}

		rows, _ := result.RowsAffected()
		if rows == 0 {
			// Cleanup uploaded file
			_ = r2.DeleteObject(ctx, objectKey)
			http.Error(w, `{"error":"book not found"}`, http.StatusNotFound)
			return
		}

		// Generate download URL
		downloadURL, err := r2.GeneratePresignedDownloadURL(ctx, objectKey)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"uploaded but failed to generate url: %v"}`, err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"cover_url":  downloadURL,
			"object_key": objectKey,
		})
	}
}

// GET /books/{key}/cover - Redirects to presigned cover URL (just like audio)
func GetBookCoverURLHandler(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		key := r.PathValue("key")

		if key == "" {
			http.Error(w, `{"error":"missing book key"}`, http.StatusBadRequest)
			return
		}

		// Get cover_url from database
		var coverURL sql.NullString
		err := db.QueryRowContext(ctx, `
            SELECT cover_url 
            FROM books 
            WHERE id::text = $1 OR slug = $1
        `, key).Scan(&coverURL)

		if err == sql.ErrNoRows {
			http.Error(w, `{"error":"book not found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
			return
		}

		if !coverURL.Valid || coverURL.String == "" {
			http.Error(w, `{"error":"book has no cover"}`, http.StatusNotFound)
			return
		}

		// Generate presigned download URL from R2
		r2, err := storage.NewR2Client(ctx)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}

		downloadURL, err := r2.GeneratePresignedDownloadURL(ctx, coverURL.String)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to generate url: %v"}`, err), http.StatusInternalServerError)
			return
		}

		// Redirect to the presigned URL (same as audio)
		http.Redirect(w, r, downloadURL, http.StatusTemporaryRedirect)
	})
}
