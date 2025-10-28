package middlewares_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	mw "github.com/5w1tchy/books-api/internal/api/middlewares"
)

func TestRequestID_GeneratesID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := mw.GetRequestID(r)
		if rid == "" {
			t.Error("Expected request ID in context")
		}
		w.WriteHeader(http.StatusOK)
	})

	wrapped := mw.RequestID(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	// Check response header
	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("Expected X-Request-ID in response header")
	}
}

func TestRequestID_UsesProvidedID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := mw.RequestID(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "custom-request-id")
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Header().Get("X-Request-ID") != "custom-request-id" {
		t.Errorf("Expected custom-request-id, got %s", rec.Header().Get("X-Request-ID"))
	}
}

func TestRequestID_RejectsInvalidID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := mw.RequestID(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "invalid@#$%id")
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	// Should generate new ID for invalid input
	rid := rec.Header().Get("X-Request-ID")
	if rid == "invalid@#$%id" {
		t.Error("Should have rejected invalid request ID")
	}
	if rid == "" {
		t.Error("Should have generated new request ID")
	}
}
