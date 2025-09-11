package apperr

import (
	"encoding/json"
	"net/http"
)

type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`    // e.g. "unique", "not_null", "fk", "invalid", "too_long"
	Message string `json:"message"` // human readable
}

type Problem struct {
	Type        string       `json:"type,omitempty"`   // RFC7807 type URI
	Title       string       `json:"title"`            // short summary
	Status      int          `json:"status"`           // HTTP status code
	Detail      string       `json:"detail,omitempty"` // human details
	Instance    string       `json:"instance,omitempty"`
	RequestID   string       `json:"request_id,omitempty"`
	FieldErrors []FieldError `json:"field_errors,omitempty"`
	Retryable   bool         `json:"retryable,omitempty"`
}

func Write(w http.ResponseWriter, r *http.Request, p Problem) {
	if p.Status == 0 {
		p.Status = http.StatusInternalServerError
	}
	if p.Instance == "" && r != nil {
		p.Instance = r.URL.Path
	}
	if p.RequestID == "" && r != nil {
		// if you have a Request-ID middleware that sets a header
		if rid := r.Header.Get("X-Request-ID"); rid != "" {
			p.RequestID = rid
		}
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// Convenience: fast write with just status+title+detail
func WriteStatus(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	Write(w, r, Problem{Status: status, Title: title, Detail: detail})
}
