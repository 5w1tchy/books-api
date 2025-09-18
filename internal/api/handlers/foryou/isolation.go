package foryou

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// blockTimeout returns the per-block timeout (default 450ms).
// Override via FOR_YOU_BLOCK_TIMEOUT_MS; clamped to [100..1500] ms.
func blockTimeout() time.Duration {
	const def = 450
	ms := def
	if v := os.Getenv("FOR_YOU_BLOCK_TIMEOUT_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 100 && n <= 1500 {
			ms = n
		}
	}
	return time.Duration(ms) * time.Millisecond
}

// classify common DB errors so we can log sanely.
func isNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
func isScanErr(err error) bool {
	// Cheap, driver-agnostic check (covers "sql: Scan error on column ...",
	// "converting NULL", "cannot scan", etc.). We avoid importing driver types.
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "scan error") ||
		strings.Contains(msg, "cannot scan") ||
		strings.Contains(msg, "converting null") ||
		strings.Contains(msg, "invalid input syntax") ||
		strings.Contains(msg, "numeric field overflow")
}

// isolateBlock runs a block safely with its own timeout:
// - derives a child context with the per-block timeout,
// - recovers panics,
// - on error/timeout, logs once with a typed classification and returns [].
// - sql.ErrNoRows → [] with NO log (normal/quiet).
func isolateBlock[T any](parent context.Context, label string, timeout time.Duration, fn func(ctx context.Context) ([]T, error)) []T {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[for-you][%s] panic: %v", label, rec)
		}
	}()

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	items, err := fn(ctx)
	if err != nil {
		// Timeout?
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("[for-you][%s] timeout: %v", label, err)
			return []T{}
		}
		// No rows is normal → quiet.
		if isNoRows(err) {
			return []T{}
		}
		// Scan / NULL / type mismatch → warn once.
		if isScanErr(err) {
			log.Printf("[for-you][%s] warn: scan %v", label, err)
			return []T{}
		}
		// Everything else → error once.
		log.Printf("[for-you][%s] error: %v", label, err)
		return []T{}
	}
	if items == nil {
		return []T{}
	}
	return items
}
