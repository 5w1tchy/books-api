package apperr

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgconn"
)

// Map well-known constraint names to fields (extend as you add constraints)
var constraintField = map[string]string{
	"books_slug_key":                   "slug",
	"books_title_key":                  "title",
	"books_author_fk":                  "author_id",
	"book_categories_book_id_fkey":     "book_id",
	"book_categories_category_id_fkey": "category_id",
}

// Guess a field from a column name present in PG error detail
func fieldFromDetail(detail string) string {
	// crude but useful
	for _, k := range []string{"slug", "title", "author", "author_id", "category_id", "book_id", "id"} {
		if strings.Contains(detail, k) {
			return k
		}
	}
	return ""
}

func fieldFromConstraint(c string) string {
	if f, ok := constraintField[c]; ok {
		return f
	}
	return ""
}

// FromPG maps a pgconn.PgError to a Problem. Returns (Problem, true) if mapped.
func FromPG(err error) (Problem, bool) {
	var pg *pgconn.PgError
	if !errors.As(err, &pg) {
		return Problem{}, false
	}

	// Defaults
	p := Problem{
		Title:  "Database error",
		Status: 500,
		Detail: strings.TrimSpace(pg.Message),
	}

	// Helpful field detection
	field := fieldFromConstraint(pg.ConstraintName)
	if field == "" && pg.Detail != "" {
		field = fieldFromDetail(pg.Detail)
	}

	// SQLSTATE switch
	switch pg.Code {
	case "23505": // unique_violation
		p.Status = 409
		p.Title = "Conflict"
		msg := "value already exists"
		if field == "" {
			field = "resource"
		}
		p.FieldErrors = []FieldError{{Field: field, Code: "unique", Message: msg}}
		p.Detail = ""
	case "23503": // foreign_key_violation
		p.Status = 409
		p.Title = "Conflict"
		msg := "resource is referenced by other records"
		if field == "" {
			field = "resource"
		}
		p.FieldErrors = []FieldError{{Field: field, Code: "fk", Message: msg}}
		p.Detail = ""
	case "23502": // not_null_violation
		p.Status = 400
		p.Title = "Bad Request"
		msg := "required field is missing"
		if field == "" && pg.ColumnName != "" {
			field = pg.ColumnName
		}
		if field == "" {
			field = "field"
		}
		p.FieldErrors = []FieldError{{Field: field, Code: "not_null", Message: msg}}
		p.Detail = ""
	case "23514": // check_violation
		p.Status = 422
		p.Title = "Unprocessable Entity"
		msg := "constraint failed"
		if field == "" {
			field = "field"
		}
		p.FieldErrors = []FieldError{{Field: field, Code: "check", Message: msg}}
		p.Detail = ""
	case "22P02": // invalid_text_representation (e.g., bad UUID)
		p.Status = 400
		p.Title = "Bad Request"
		msg := "invalid format"
		if field == "" {
			// common case: path param id/uuid
			field = "id"
		}
		p.FieldErrors = []FieldError{{Field: field, Code: "invalid", Message: msg}}
		p.Detail = ""
	case "22001": // string_data_right_truncation
		p.Status = 400
		p.Title = "Bad Request"
		msg := "value is too long"
		if field == "" {
			field = "field"
		}
		p.FieldErrors = []FieldError{{Field: field, Code: "too_long", Message: msg}}
		p.Detail = ""
	case "40001": // serialization_failure
		p.Status = 409
		p.Title = "Conflict"
		p.Detail = "transaction conflict, please retry"
		p.Retryable = true
	case "40P01": // deadlock_detected
		p.Status = 409
		p.Title = "Conflict"
		p.Detail = "deadlock detected, please retry"
		p.Retryable = true
	// Optional: treat malformed queries/columns as 400 (usually our bug, but safer for clients)
	case "42703": // undefined_column
		p.Status = 400
		p.Title = "Bad Request"
	case "42601": // syntax_error
		p.Status = 400
		p.Title = "Bad Request"
	default:
		// Keep default 500 with minimal detail
		p.Title = "Database error"
		p.Detail = ""
	}

	return p, true
}

// HandleDBError maps err to a Problem and writes it. Returns true if handled.
func HandleDBError(w http.ResponseWriter, r *http.Request, err error, fallbackTitle string) bool {
	if err == nil {
		return false
	}
	if p, ok := FromPG(err); ok {
		Write(w, r, p)
		return true
	}
	// Not a PG error: generic 500
	Write(w, r, Problem{Status: 500, Title: fallbackTitle})
	return true
}
