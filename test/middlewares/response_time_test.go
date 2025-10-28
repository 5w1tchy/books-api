package middlewares_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	mw "github.com/5w1tchy/books-api/internal/api/middlewares"
)

func TestResponseTimeMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	wrapped := mw.ResponseTimeMiddleware(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	// Check for response time header
	responseTime := rec.Header().Get("X-Response-Time")
	if responseTime == "" {
		t.Error("Expected X-Response-Time header")
	}

	// Should contain time duration format
	if len(responseTime) < 3 {
		t.Errorf("Response time seems invalid: %s", responseTime)
	}
}

func TestResponseTimeMiddleware_WithWrite(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test response"))
	})

	wrapped := mw.ResponseTimeMiddleware(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Header().Get("X-Response-Time") == "" {
		t.Error("Expected X-Response-Time header when using Write")
	}
}
