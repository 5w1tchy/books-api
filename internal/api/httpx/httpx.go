package httpx

import (
	"encoding/json"
	"net/http"
)

type errorEnvelope struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

type CodedError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func ErrorJSON(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, errorEnvelope{Status: "error", Error: message})
}

func OK(w http.ResponseWriter, data any) {
	WriteJSON(w, http.StatusOK, map[string]any{"status": "success", "data": data})
}

func OKNoData(w http.ResponseWriter) {
	WriteJSON(w, http.StatusOK, map[string]any{"status": "success"})
}

func ErrorCode(w http.ResponseWriter, status int, code, msg string) {
	var e CodedError
	e.Error.Code = code
	e.Error.Message = msg
	WriteJSON(w, status, e)
}
