package handlers

import "net/http"

func CategoriesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Write([]byte("Hello GET Method on Categories route"))
	case http.MethodPost:
		w.Write([]byte("Hello POST Method on Categories route"))
	case http.MethodPatch:
		w.Write([]byte("Hello PATCH Method on Categories route"))
	case http.MethodPut:
		w.Write([]byte("Hello PUT Method on Categories route"))
	case http.MethodDelete:
		w.Write([]byte("Hello DELETE Method on Categories route"))
	}
}
