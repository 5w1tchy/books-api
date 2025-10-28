package middlewares_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	mw "github.com/5w1tchy/books-api/internal/api/middlewares"
)

func TestRecovery(t *testing.T) {
	// Handler that panics
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	// Wrap with recovery middleware
	handler := mw.Recovery(panicHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	// Should not panic, should return 500
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", rec.Code)
	}

	body := rec.Body.String()
	if body != "Internal Server Error\n" {
		t.Errorf("Expected error message, got: %s", body)
	}
}

func TestRecoveryWithRequestID(t *testing.T) {
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("panic with request ID")
	})

	// Chain RequestID -> Recovery
	handler := mw.RequestID(mw.Recovery(panicHandler))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", rec.Code)
	}

	// Should have request ID in response
	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("Expected X-Request-ID header")
	}
}

func TestRecoveryDoesNotInterceptNormalRequests(t *testing.T) {
	normalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	handler := mw.Recovery(normalHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	if rec.Body.String() != "success" {
		t.Errorf("Expected 'success', got: %s", rec.Body.String())
	}
}
