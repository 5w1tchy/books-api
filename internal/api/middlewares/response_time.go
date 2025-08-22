package middlewares

import (
	"net/http"
	"time"
)

type rtWriter struct {
	http.ResponseWriter
	start       time.Time
	wroteHeader bool
	status      int
}

func (w *rtWriter) stamp() {
	if !w.wroteHeader {
		w.Header().Set("X-Response-Time", time.Since(w.start).String())
		w.wroteHeader = true
	}
}

func (w *rtWriter) WriteHeader(code int) {
	w.status = code
	w.stamp()
	w.ResponseWriter.WriteHeader(code)
}

func (w *rtWriter) Write(b []byte) (int, error) {
	w.stamp()
	return w.ResponseWriter.Write(b)
}

func ResponseTimeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &rtWriter{
			ResponseWriter: w,
			start:          time.Now(),
			status:         http.StatusOK,
		}
		next.ServeHTTP(rw, r)

		// If nothing was written (e.g., 204/HEAD), set it now.
		if !rw.wroteHeader {
			rw.Header().Set("X-Response-Time", time.Since(rw.start).String())
		}
	})
}
