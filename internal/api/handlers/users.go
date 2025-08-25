package handlers

import "net/http"

func UsersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Write([]byte("Hello GET Method on Users route"))
	case http.MethodPost:
		w.Write([]byte("Hello POST Method on Users route"))
	case http.MethodPatch:
		w.Write([]byte("Hello PATCH Method on Users route"))
	case http.MethodPut:
		w.Write([]byte("Hello PUT Method on Users route"))
	case http.MethodDelete:
		w.Write([]byte("Hello DELETE Method on Users route"))
	}
}
