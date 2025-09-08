package middlewares

import (
	"net/http"
	"strings"
)

type HPPOptions struct {
	CheckQuery                  bool
	CheckBody                   bool
	CheckBodyOnlyForContentType string
	Whitelist                   []string
}

func HPP(opts HPPOptions) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if opts.CheckBody && r.Method == http.MethodPost && isCorrectContentType(r, opts.CheckBodyOnlyForContentType) {
				filterBodyParams(r, opts.Whitelist)
			}
			if opts.CheckQuery && r.URL.Query() != nil {
				filterQueryParams(r, opts.Whitelist)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isCorrectContentType(r *http.Request, contentType string) bool {
	return strings.Contains(r.Header.Get("Content-Type"), contentType)
}

func filterBodyParams(r *http.Request, whitelist []string) {
	if err := r.ParseForm(); err != nil {
		return
	}
	for k, v := range r.Form {
		if len(v) > 1 {
			r.Form.Set(k, v[0])
		}
		if !isWhitelisted(k, whitelist) {
			delete(r.Form, k)
		}
	}
}

func isWhitelisted(param string, whitelist []string) bool {
	for _, w := range whitelist {
		if w == param {
			return true
		}
	}
	return false
}

func filterQueryParams(r *http.Request, whitelist []string) {
	query := r.URL.Query()
	for k, v := range query {
		if len(v) > 1 {
			query.Set(k, v[0])
		}
		if !isWhitelisted(k, whitelist) {
			query.Del(k)
		}
	}
	r.URL.RawQuery = query.Encode()
}

func DefaultHPPOptions() HPPOptions {
	return HPPOptions{
		CheckQuery:                  true,
		CheckBody:                   true,
		CheckBodyOnlyForContentType: "application/x-www-form-urlencoded",
		Whitelist: []string{
			"id", "user_id", "book_id", "chapter", "page",
			"limit", "offset",
			"lang", "search", "q",
			"category", "categories", "tags",
			"title", "author", "min_sim",
			"sort", "order", "match",
			"username", "email", "password", "token", "session_id",
			"note_id", "content", "created_at", "updated_at",
			"highlight_id", "text", "color",
			"progress_id", "percentage", "last_read_at",
		},
	}
}
