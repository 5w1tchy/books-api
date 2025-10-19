package books

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
	"time"

	httpx "github.com/5w1tchy/books-api/internal/api/httpx"
	storage "github.com/5w1tchy/books-api/internal/storage/s3"
	storebooks "github.com/5w1tchy/books-api/internal/store/books"
	"github.com/redis/go-redis/v9"
)

const (
	maxAudioSize = 200 << 20 // 200 MB
	maxCoverSize = 10 << 20  // 10 MB
)

var allowedAudioC = map[string]bool{
	"audio/mpeg":  true,
	"audio/mp3":   true,
	"audio/mp4":   true,
	"audio/ogg":   true,
	"audio/wav":   true,
	"audio/x-wav": true,
}

var allowedCoverC = map[string]bool{
	"image/jpeg": true,
	"image/jpg":  true,
	"image/png":  true,
	"image/webp": true,
}

// === Request / Response ===

type adminCreateReq struct {
	Coda       string   `json:"coda"`       // free-text (no slugifying)
	Title      string   `json:"title"`      // required
	Authors    []string `json:"authors"`    // >=1
	Categories []string `json:"categories"` // >=1
	Short      string   `json:"short,omitempty"`
	Summary    string   `json:"summary,omitempty"`
}

type adminCreateResp struct {
	Status   string               `json:"status"`
	Data     storebooks.AdminBook `json:"data"`
	AudioKey string               `json:"audio_key,omitempty"`
	AudioURL string               `json:"audio_url,omitempty"`
	CoverKey string               `json:"cover_key,omitempty"`
	CoverURL string               `json:"cover_url,omitempty"`
}

// === Handler ===

func AdminCreate(db *sql.DB, _ *redis.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpx.ErrorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var (
			in               adminCreateReq
			ctx              = r.Context()
			audioFound       bool
			coverFound       bool
			audioKey         string
			coverKey         string
			audioURL         string
			coverURL         string
			audioSize        int64
			coverSize        int64
			audioContentType string
			coverContentType string
			audioFile        multipart.File
			coverFile        multipart.File
			r2client         *storage.S3Client
		)

		ct := r.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "multipart/form-data") {
			// multipart path (supports optional audio and cover)
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				httpx.ErrorJSON(w, http.StatusBadRequest, "invalid multipart form")
				return
			}
			in.Coda = strings.TrimSpace(r.FormValue("coda"))
			in.Title = strings.TrimSpace(r.FormValue("title"))
			in.Short = strings.TrimSpace(r.FormValue("short"))
			in.Summary = strings.TrimSpace(r.FormValue("summary"))
			in.Authors = normalizeSlice(r.Form["authors"])
			in.Categories = normalizeSlice(r.Form["categories"])

			// Handle audio file
			if f, hdr, err := r.FormFile("audio"); err == nil {
				audioFile = f
				audioFound = true
				audioSize = hdr.Size
				audioContentType = hdr.Header.Get("Content-Type")
			}

			// Handle cover file
			if f, hdr, err := r.FormFile("cover"); err == nil {
				coverFile = f
				coverFound = true
				coverSize = hdr.Size
				coverContentType = hdr.Header.Get("Content-Type")
			}
		} else {
			// JSON path (no files)
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				httpx.ErrorJSON(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			in.Coda = strings.TrimSpace(in.Coda)
			in.Title = strings.TrimSpace(in.Title)
			in.Short = strings.TrimSpace(in.Short)
			in.Summary = strings.TrimSpace(in.Summary)
			in.Authors = normalizeSlice(in.Authors)
			in.Categories = normalizeSlice(in.Categories)
		}

		// validations (coda is free text now; no regex)
		if in.Title == "" {
			httpx.ErrorJSON(w, http.StatusBadRequest, "title is required")
			return
		}
		if len(in.Authors) == 0 || in.Authors[0] == "" {
			httpx.ErrorJSON(w, http.StatusBadRequest, "at least one author is required")
			return
		}
		if len(in.Categories) == 0 || in.Categories[0] == "" {
			httpx.ErrorJSON(w, http.StatusBadRequest, "at least one category is required")
			return
		}
		if len(in.Short) > 280 {
			httpx.ErrorJSON(w, http.StatusBadRequest, "short must be <= 280 chars")
			return
		}
		if len(in.Summary) > 10000 {
			httpx.ErrorJSON(w, http.StatusBadRequest, "summary too long")
			return
		}

		// Initialize R2 client if we have any files
		if audioFound || coverFound {
			var err error
			r2client, err = storage.NewR2Client(ctx)
			if err != nil {
				log.Printf("[admin books] r2 init error: %v", err)
				httpx.ErrorJSON(w, http.StatusInternalServerError, "storage client init failed")
				return
			}
		}

		// Handle audio upload
		if audioFound {
			if audioSize > maxAudioSize {
				httpx.ErrorJSON(w, http.StatusBadRequest, "audio too large")
				return
			}

			// sniff if missing
			if audioContentType == "" {
				head := make([]byte, 512)
				n, _ := audioFile.Read(head)
				_, _ = audioFile.Seek(0, io.SeekStart)
				audioContentType = http.DetectContentType(head[:n])
			}
			if !allowedAudioC[audioContentType] {
				httpx.ErrorJSON(w, http.StatusBadRequest, "unsupported audio content type")
				return
			}

			// build object key from TITLE (slugify for a safe path)
			safe := slugifyTitle(in.Title)
			if safe == "" {
				safe = fmt.Sprintf("book-%d", time.Now().UnixNano())
			}
			ext := ".mp3"
			switch audioContentType {
			case "audio/ogg":
				ext = ".ogg"
			case "audio/mp4":
				ext = ".m4a"
			case "audio/wav", "audio/x-wav":
				ext = ".wav"
			}
			audioKey = path.Join("books", fmt.Sprintf("%s-audio-%d%s", safe, time.Now().Unix(), ext))

			if err := uploadFileToR2(ctx, r2client, audioKey, audioFile, audioContentType, audioSize); err != nil {
				log.Printf("[admin books] audio upload error: %v", err)
				httpx.ErrorJSON(w, http.StatusInternalServerError, fmt.Sprintf("audio upload failed: %v", err))
				return
			}
			_ = audioFile.Close()

			// optional presigned GET for immediate preview
			if url, err := r2client.GeneratePresignedDownloadURL(ctx, audioKey); err == nil {
				audioURL = url
			}
		}

		// Handle cover upload
		if coverFound {
			if coverSize > maxCoverSize {
				// Cleanup audio if already uploaded
				if audioFound && r2client != nil && audioKey != "" {
					_ = r2client.DeleteObject(ctx, audioKey)
				}
				httpx.ErrorJSON(w, http.StatusBadRequest, "cover too large (max 10MB)")
				return
			}

			// sniff if missing
			if coverContentType == "" {
				head := make([]byte, 512)
				n, _ := coverFile.Read(head)
				_, _ = coverFile.Seek(0, io.SeekStart)
				coverContentType = http.DetectContentType(head[:n])
			}
			if !allowedCoverC[coverContentType] {
				// Cleanup audio if already uploaded
				if audioFound && r2client != nil && audioKey != "" {
					_ = r2client.DeleteObject(ctx, audioKey)
				}
				httpx.ErrorJSON(w, http.StatusBadRequest, "unsupported cover type (use jpeg, png, or webp)")
				return
			}

			// build object key: books/covers/<slug>-<timestamp>.<ext>
			safe := slugifyTitle(in.Title)
			if safe == "" {
				safe = fmt.Sprintf("book-%d", time.Now().UnixNano())
			}
			ext := ".jpg"
			switch coverContentType {
			case "image/png":
				ext = ".png"
			case "image/webp":
				ext = ".webp"
			}
			coverKey = path.Join("books/covers", fmt.Sprintf("%s-%d%s", safe, time.Now().Unix(), ext))

			if err := uploadFileToR2(ctx, r2client, coverKey, coverFile, coverContentType, coverSize); err != nil {
				log.Printf("[admin books] cover upload error: %v", err)
				// Cleanup audio if already uploaded
				if audioFound && r2client != nil && audioKey != "" {
					_ = r2client.DeleteObject(ctx, audioKey)
				}
				httpx.ErrorJSON(w, http.StatusInternalServerError, fmt.Sprintf("cover upload failed: %v", err))
				return
			}
			_ = coverFile.Close()

			// optional presigned GET for immediate preview
			if url, err := r2client.GeneratePresignedDownloadURL(ctx, coverKey); err == nil {
				coverURL = url
			}
		}

		// create the book via existing store; Coda stays as raw text
		dto := storebooks.CreateBookV2DTO{
			Coda:       in.Coda, // raw text (no slugifying)
			Title:      in.Title,
			Authors:    in.Authors,
			Categories: in.Categories,
			Short:      in.Short,
			Summary:    in.Summary,
		}

		book, err := storebooks.CreateV2(ctx, db, dto)
		if err != nil {
			log.Printf("[admin books] create error: %v", err)
			// cleanup uploaded files if create fails
			if r2client != nil {
				if audioKey != "" {
					_ = r2client.DeleteObject(ctx, audioKey)
				}
				if coverKey != "" {
					_ = r2client.DeleteObject(ctx, coverKey)
				}
			}
			if strings.Contains(strings.ToLower(err.Error()), "code_exists") {
				httpx.ErrorJSON(w, http.StatusConflict, "coda already exists")
				return
			}
			httpx.ErrorJSON(w, http.StatusInternalServerError, "failed to create book")
			return
		}

		// attach audio and cover to the created row
		if audioKey != "" || coverKey != "" {
			query := `UPDATE books SET `
			args := []interface{}{}
			argIdx := 1

			if audioKey != "" {
				query += fmt.Sprintf("audio_key = $%d", argIdx)
				args = append(args, audioKey)
				argIdx++
			}

			if coverKey != "" {
				if audioKey != "" {
					query += ", "
				}
				query += fmt.Sprintf("cover_url = $%d", argIdx)
				args = append(args, coverKey)
				argIdx++
			}

			query += fmt.Sprintf(" WHERE id = $%d", argIdx)
			args = append(args, book.ID)

			_, err := db.ExecContext(ctx, query, args...)
			if err != nil {
				if r2client != nil {
					if audioKey != "" {
						_ = r2client.DeleteObject(ctx, audioKey)
					}
					if coverKey != "" {
						_ = r2client.DeleteObject(ctx, coverKey)
					}
				}
				log.Printf("[admin books] attach files failed: %v", err)
				httpx.ErrorJSON(w, http.StatusInternalServerError, "failed to attach files to book")
				return
			}
		}

		resp := adminCreateResp{
			Status:   "success",
			Data:     book,
			AudioKey: audioKey,
			AudioURL: audioURL,
			CoverKey: coverKey,
			CoverURL: coverURL,
		}
		httpx.WriteJSON(w, http.StatusOK, resp)
	})
}
