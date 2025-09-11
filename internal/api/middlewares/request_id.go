package middlewares

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"regexp"
	"time"
)

type ctxKey int

const ctxKeyRequestID ctxKey = iota

var ridRe = regexp.MustCompile(`^[A-Za-z0-9_.\-]{1,64}$`)

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-ID")
		if !ridRe.MatchString(rid) {
			rid = genRID()
		}
		// make it available to handlers/middlewares and the response
		r = r.WithContext(context.WithValue(r.Context(), ctxKeyRequestID, rid))
		r.Header.Set("X-Request-ID", rid)
		w.Header().Set("X-Request-ID", rid)

		next.ServeHTTP(w, r)
	})
}

// GetRequestID extracts the value previously set by RequestID middleware.
func GetRequestID(r *http.Request) string {
	v, _ := r.Context().Value(ctxKeyRequestID).(string)
	if v != "" {
		return v
	}
	return r.Header.Get("X-Request-ID")
}

func genRID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	// timestamp prefix helps with log sorting
	ts := time.Now().UTC().Format("20060102T150405Z")
	return ts + "-" + hex.EncodeToString(b[:])
}
