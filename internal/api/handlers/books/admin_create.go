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

const maxAudioSize = 200 << 20 // 200 MB

var allowedAudioC = map[string]bool{
	"audio/mpeg":  true,
	"audio/mp3":   true,
	"audio/mp4":   true,
	"audio/ogg":   true,
	"audio/wav":   true,
	"audio/x-wav": true,
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
}

// === Handler ===

func AdminCreate(db *sql.DB, _ *redis.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpx.ErrorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var (
			in          adminCreateReq
			ctx         = r.Context()
			audioFound  bool
			audioKey    string
			audioURL    string
			audioSize   int64
			contentType string
			file        multipart.File
			r2client    *storage.S3Client
		)

		ct := r.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "multipart/form-data") {
			// multipart path (supports optional audio)
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

			f, hdr, err := r.FormFile("audio")
			if err == nil {
				file = f
				audioFound = true
				audioSize = hdr.Size
				contentType = hdr.Header.Get("Content-Type")
			}
		} else {
			// JSON path (no audio)
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

		// if audio present: checks & upload via presigned PUT (server-side)
		if audioFound {
			if audioSize > maxAudioSize {
				httpx.ErrorJSON(w, http.StatusBadRequest, "audio too large")
				return
			}

			// sniff if missing
			if contentType == "" {
				head := make([]byte, 512)
				n, _ := file.Read(head)
				_, _ = file.Seek(0, io.SeekStart)
				contentType = http.DetectContentType(head[:n])
			}
			if !allowedAudioC[contentType] {
				httpx.ErrorJSON(w, http.StatusBadRequest, "unsupported audio content type")
				return
			}

			// build object key from TITLE (slugify for a safe path); do NOT use coda (free text)
			safe := slugifyTitle(in.Title)
			if safe == "" {
				safe = fmt.Sprintf("book-%d", time.Now().UnixNano())
			}
			ext := ".mp3"
			switch contentType {
			case "audio/ogg":
				ext = ".ogg"
			case "audio/mp4":
				ext = ".m4a"
			case "audio/wav", "audio/x-wav":
				ext = ".wav"
			}
			audioKey = path.Join("books", fmt.Sprintf("%s-audio-%d%s", safe, time.Now().Unix(), ext))

			var err error
			r2client, err = storage.NewR2Client(ctx)
			if err != nil {
				log.Printf("[admin books] r2 init error: %v", err)
				httpx.ErrorJSON(w, http.StatusInternalServerError, "storage client init failed")
				return
			}

			// IMPORTANT: pass contentLength to avoid 411 MissingContentLength
			if err := uploadFileToR2(ctx, r2client, audioKey, file, contentType, audioSize); err != nil {
				log.Printf("[admin books] audio upload error: %v", err)
				httpx.ErrorJSON(w, http.StatusInternalServerError, fmt.Sprintf("audio upload failed: %v", err))
				return
			}
			_ = file.Close()

			// optional presigned GET for immediate preview
			if url, err := r2client.GeneratePresignedDownloadURL(ctx, audioKey); err == nil {
				audioURL = url
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
			// cleanup uploaded file if create fails
			if audioFound && r2client != nil && audioKey != "" {
				_ = r2client.DeleteObject(ctx, audioKey)
			}
			if strings.Contains(strings.ToLower(err.Error()), "code_exists") {
				httpx.ErrorJSON(w, http.StatusConflict, "coda already exists")
				return
			}
			httpx.ErrorJSON(w, http.StatusInternalServerError, "failed to create book")
			return
		}

		// attach audio to the created row (books.audio_key is NULLABLE as you set)
		if audioFound && audioKey != "" {
			_, err := db.ExecContext(ctx, `
				UPDATE books
				SET audio_key = $1
				WHERE id = $2
			`, audioKey, book.ID)
			if err != nil {
				if r2client != nil {
					_ = r2client.DeleteObject(ctx, audioKey)
				}
				log.Printf("[admin books] attach audio failed: %v", err)
				httpx.ErrorJSON(w, http.StatusInternalServerError, "failed to attach audio to book")
				return
			}
		}

		resp := adminCreateResp{
			Status:   "success",
			Data:     book,
			AudioKey: audioKey,
			AudioURL: audioURL,
		}
		httpx.WriteJSON(w, http.StatusOK, resp)
	})
}
