package middlewares_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mw "github.com/5w1tchy/books-api/internal/api/middlewares"
)

func TestBodySizeLimit_AcceptsSmallBodies(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("received: " + string(body)))
	})

	wrapped := mw.BodySizeLimit(handler)

	smallBody := []byte("small body")
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(smallBody))
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}
}

func TestBodySizeLimit_RejectsLargeBodies(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	wrapped := mw.BodySizeLimit(handler)

	// Create 11MB body (exceeds 10MB default limit)
	largeBody := bytes.Repeat([]byte("a"), 11*1024*1024)
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(largeBody))
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	// Should reject with 413 or handler's error
	if rec.Code == http.StatusOK {
		t.Error("Expected body size limit to reject large request")
	}
}

func TestBodySizeLimit_OnlyAppliesToMutatingMethods(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := mw.BodySizeLimit(handler)

	// GET request should not have body size limit applied
	req := httptest.NewRequest("GET", "/test", strings.NewReader("should not matter"))
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200 for GET, got %d", rec.Code)
	}
}

func TestBodySizeLimit_PUT_Method(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	wrapped := mw.BodySizeLimit(handler)

	largeBody := bytes.Repeat([]byte("x"), 11*1024*1024)
	req := httptest.NewRequest("PUT", "/test", bytes.NewReader(largeBody))
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("Expected PUT with large body to be rejected")
	}
}
