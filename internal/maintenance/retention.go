// internal/maintenance/retention.go
package maintenance

import (
	"context"
	"database/sql"
	"log"
	"strconv"
	"strings"
	"time"
)

// StartBookOutputsRetention runs a daily job at localTime ("HH:MM") in tzName
// that keeps only the latest keepN rows per book_id in book_outputs.
// Call once at startup: go maintenance.StartBookOutputsRetention(ctx, db, 5, "03:00", "Asia/Tbilisi")
func StartBookOutputsRetention(ctx context.Context, db *sql.DB, keepN int, localTime string, tzName string) {
	if keepN <= 0 {
		keepN = 5
	}
	go func() {
		loc, err := time.LoadLocation(tzName)
		if err != nil {
			loc = time.Local
		}
		h, m := 3, 0
		if parts := strings.Split(localTime, ":"); len(parts) == 2 {
			if v, err := strconv.Atoi(parts[0]); err == nil {
				h = v
			}
			if v, err := strconv.Atoi(parts[1]); err == nil {
				m = v
			}
		}

		ensureIdx := func(ctx context.Context) {
			const q = `CREATE INDEX IF NOT EXISTS idx_book_outputs_book_id_created_at
			           ON book_outputs (book_id, created_at DESC);`
			if _, err := db.ExecContext(ctx, q); err != nil {
				log.Printf("[retention] ensure index failed: %v", err)
			}
		}

		runOnce := func(ctx context.Context) {
			ensureIdx(ctx)
			const q = `
WITH ranked AS (
  SELECT id, ROW_NUMBER() OVER (PARTITION BY book_id ORDER BY created_at DESC) AS rn
  FROM book_outputs
)
DELETE FROM book_outputs bo
USING ranked r
WHERE bo.id = r.id
  AND r.rn > $1;`
			if _, err := db.ExecContext(ctx, q, keepN); err != nil {
				log.Printf("[retention] delete old book_outputs failed: %v", err)
			} else {
				log.Printf("[retention] book_outputs pruned to latest %d rows per book", keepN)
			}
		}

		for {
			now := time.Now().In(loc)
			next := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, loc)
			if !next.After(now) {
				next = next.Add(24 * time.Hour)
			}
			timer := time.NewTimer(time.Until(next))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				runOnce(ctx)
			}
		}
	}()
}
