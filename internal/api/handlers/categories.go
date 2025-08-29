package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func CategoriesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		rows, err := db.Query(`SELECT id, name, slug FROM categories ORDER BY name`)
		if err != nil {
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var out []Category
		for rows.Next() {
			var c Category
			if err := rows.Scan(&c.ID, &c.Name, &c.Slug); err != nil {
				http.Error(w, "DB scan error", http.StatusInternalServerError)
				return
			}
			out = append(out, c)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"count":  len(out),
			"data":   out,
		})
	}
}
