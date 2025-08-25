package utils

import "net/http"

type Middleware func(http.Handler) http.Handler

func ApplyMiddleware(handler http.Handler, middlewares ...Middleware) http.Handler {
	for _, mw := range middlewares {
		handler = mw(handler)
	}
	return handler
}
