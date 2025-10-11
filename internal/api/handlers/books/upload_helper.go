package books

import (
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	storage "github.com/5w1tchy/books-api/internal/storage/s3"
)

// uploadFileToR2 creates a presigned PUT url and streams the file to it.
// - file is the multipart.File opened by r.FormFile
// - contentType should be validated before calling
// - contentLength MUST be set (R2 rejects chunked uploads without it)
func uploadFileToR2(ctx context.Context, r2 *storage.S3Client, objectKey string, file multipart.File, contentType string, contentLength int64) error {
	uploadURL, err := r2.GeneratePresignedUploadURL(ctx, objectKey, contentType)
	if err != nil {
		return fmt.Errorf("generate presigned upload url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, file)
	if err != nil {
		return fmt.Errorf("create put request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Content-Length", strconv.FormatInt(contentLength, 10))
	req.ContentLength = contentLength // ensure no chunked encoding

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("put to r2 failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("r2 upload failed status=%d", resp.StatusCode)
	}
	return nil
}
