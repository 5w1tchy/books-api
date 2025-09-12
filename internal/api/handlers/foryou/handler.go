package foryou

import (
	"crypto/sha1"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	storeforyou "github.com/5w1tchy/books-api/internal/store/foryou"
	"github.com/redis/go-redis/v9"
)

// Handler: GET /for-you[?shorts=5&recs=8&trending=6&new=6&fields=lite|summary]
func Handler(db *sql.DB, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		lim := storeforyou.Limits{
			Shorts:   clampInt(qInt(r, "shorts", 5), 1, 10),
			Recs:     clampInt(qInt(r, "recs", 8), 0, 16),
			Trending: clampInt(qInt(r, "trending", 6), 0, 16),
			New:      clampInt(qInt(r, "new", 6), 0, 16),
		}

		fieldsParam := r.URL.Query().Get("fields")
		fields := storeforyou.Fields{
			Lite:           fieldsParam == "lite",
			IncludeSummary: fieldsParam == "summary",
		}

		sec, err := storeforyou.Build(r.Context(), db, rdb, lim, fields)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		resp := struct {
			Status string               `json:"status"`
			Data   storeforyou.Sections `json:"data"`
		}{
			Status: "success",
			Data:   sec,
		}

		b, _ := json.Marshal(resp)
		writeWithETag(w, r, b)
	}
}

// ---------- tiny helpers (handler-scoped) ----------

func qInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func clampInt(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

func writeWithETag(w http.ResponseWriter, r *http.Request, body []byte) {
	etag := fmt.Sprintf(`W/"%x"`, sha1.Sum(body))
	w.Header().Set("ETag", etag)
	if match := r.Header.Get("If-None-Match"); match != "" && match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(body)
}
