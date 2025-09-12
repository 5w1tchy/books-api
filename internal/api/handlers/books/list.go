package books

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	storebooks "github.com/5w1tchy/books-api/internal/store/books"
)

func list(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		q := strings.TrimSpace(r.URL.Query().Get("q"))
		minSim := parseFloat(r.URL.Query().Get("min_sim"), 0.2)
		author := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("author")))
		cats := parseCSV(r.URL.Query().Get("categories"))
		match := strings.ToLower(r.URL.Query().Get("match"))
		if match != "all" {
			match = "any"
		}
		limit := clamp(parseInt(r.URL.Query().Get("limit"), 20), 1, 100)
		offset := clamp(parseInt(r.URL.Query().Get("offset"), 0), 0, 100000)

		f := storebooks.ListFilters{
			Q:          q,
			MinSim:     minSim,
			Author:     author,
			Categories: cats,
			Match:      match,
			Limit:      limit,
			Offset:     offset,
		}

		items, total, err := storebooks.List(r.Context(), db, f)
		if err != nil {
			http.Error(w, `{"status":"error","error":"failed to list"}`, http.StatusInternalServerError)
			return
		}

		resp := struct {
			Status string                  `json:"status"`
			Data   []storebooks.PublicBook `json:"data"`
			Total  int                     `json:"total"`
			Limit  int                     `json:"limit"`
			Offset int                     `json:"offset"`
		}{
			Status: "success",
			Data:   items,
			Total:  total,
			Limit:  limit,
			Offset: offset,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func parseCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
func parseInt(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}
func parseFloat(s string, def float64) float64 {
	if s == "" {
		return def
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return def
}
func clamp(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}
