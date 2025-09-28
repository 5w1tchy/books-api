package books

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	storebooks "github.com/5w1tchy/books-api/internal/store/books"
	"github.com/redis/go-redis/v9"
)

type adminCreateReq struct {
	Code       string   `json:"code,omitempty"`
	Title      string   `json:"title"`
	Authors    []string `json:"authors"`    // >=1
	Categories []string `json:"categories"` // >=1
	Short      string   `json:"short,omitempty"`
	Summary    string   `json:"summary,omitempty"`
}

type adminCreateResp struct {
	Status string               `json:"status"`
	Data   storebooks.AdminBook `json:"data"`
}

var codeRE = regexp.MustCompile(`^[a-z0-9-]{3,64}$`)

func AdminCreate(db *sql.DB, _ *redis.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		var in adminCreateReq
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, `{"status":"error","error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		// normalize
		in.Code = strings.TrimSpace(in.Code)
		in.Title = strings.TrimSpace(in.Title)
		for i := range in.Authors {
			in.Authors[i] = strings.TrimSpace(in.Authors[i])
		}
		for i := range in.Categories {
			in.Categories[i] = strings.TrimSpace(in.Categories[i])
		}
		in.Short = strings.TrimSpace(in.Short)
		in.Summary = strings.TrimSpace(in.Summary)

		// quick validation
		if in.Title == "" {
			http.Error(w, `{"status":"error","error":"title is required"}`, http.StatusBadRequest)
			return
		}
		if len(in.Authors) == 0 || in.Authors[0] == "" {
			http.Error(w, `{"status":"error","error":"at least one author is required"}`, http.StatusBadRequest)
			return
		}
		if len(in.Categories) == 0 || in.Categories[0] == "" {
			http.Error(w, `{"status":"error","error":"at least one category is required"}`, http.StatusBadRequest)
			return
		}
		if in.Code != "" && !codeRE.MatchString(in.Code) {
			http.Error(w, `{"status":"error","error":"invalid code format"}`, http.StatusBadRequest)
			return
		}
		if len(in.Short) > 280 {
			http.Error(w, `{"status":"error","error":"short must be <= 280 chars"}`, http.StatusBadRequest)
			return
		}
		if len(in.Summary) > 10000 {
			http.Error(w, `{"status":"error","error":"summary too long"}`, http.StatusBadRequest)
			return
		}

		dto := storebooks.CreateBookV2DTO{
			Code:       in.Code,
			Title:      in.Title,
			Authors:    in.Authors,
			Categories: in.Categories,
			Short:      in.Short,
			Summary:    in.Summary,
		}

		book, err := storebooks.CreateV2(r.Context(), db, dto)
		if err != nil {
			// unique code
			if strings.Contains(strings.ToLower(err.Error()), "code_exists") {
				http.Error(w, `{"status":"error","error":"code already exists"}`, http.StatusConflict)
				return
			}
			http.Error(w, `{"status":"error","error":"failed to create book"}`, http.StatusInternalServerError)
			return
		}

		_ = json.NewEncoder(w).Encode(adminCreateResp{Status: "success", Data: book})
	})
}
