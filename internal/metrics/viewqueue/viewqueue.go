package viewqueue

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

type event struct {
	bookID   string
	viewedAt time.Time
}

var (
	dbRef *sql.DB
	ch    chan event
	done  chan struct{}
	wg    sync.WaitGroup
	once  sync.Once
)

// Start spins up N workers with a buffered channel.
// Suggested: buf=10000, workers=2
func Start(db *sql.DB, buf, workers int) {
	once.Do(func() {
		dbRef = db
		ch = make(chan event, buf)
		done = make(chan struct{})
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go worker()
		}
	})
}

// Enqueue tries to queue a view event without blocking.
// If the buffer is full, the event is dropped (acceptable for metrics).
func Enqueue(bookID string) {
	if bookID == "" {
		return
	}
	ev := event{bookID: bookID, viewedAt: time.Now().UTC()}
	select {
	case ch <- ev:
	default:
		// buffer full; drop
	}
}

// Shutdown signals workers to stop, flushes remaining events, and waits.
func Shutdown() {
	if done == nil {
		return
	}
	close(done)
	wg.Wait()
}

// --- internal ---

const (
	batchSize  = 100
	flushEvery = 250 * time.Millisecond
	writeTO    = 500 * time.Millisecond
	insertTmpl = `INSERT INTO book_view_events (book_id, viewed_at) VALUES %s`
)

func worker() {
	defer wg.Done()
	tk := time.NewTicker(flushEvery)
	defer tk.Stop()

	batch := make([]event, 0, batchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		_ = insertBatch(batch) // best-effort; errors are ignored for metrics
		batch = batch[:0]
	}

	for {
		select {
		case <-done:
			// drain quickly then flush
			for {
				select {
				case ev := <-ch:
					batch = append(batch, ev)
					if len(batch) >= batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		case ev := <-ch:
			batch = append(batch, ev)
			if len(batch) >= batchSize {
				flush()
			}
		case <-tk.C:
			flush()
		}
	}
}

func insertBatch(batch []event) error {
	if len(batch) == 0 {
		return nil
	}
	// VALUES ($1,$2),($3,$4)...
	args := make([]any, 0, len(batch)*2)
	vals := make([]byte, 0, len(batch)*12)
	for i, ev := range batch {
		if i > 0 {
			vals = append(vals, ',')
		}
		p1 := 2*i + 1
		p2 := 2*i + 2
		vals = append(vals, fmt.Sprintf("($%d,$%d)", p1, p2)...)
		args = append(args, ev.bookID, ev.viewedAt)
	}
	ctx, cancel := context.WithTimeout(context.Background(), writeTO)
	defer cancel()
	_, err := dbRef.ExecContext(ctx, fmt.Sprintf(insertTmpl, string(vals)), args...)
	return err
}
