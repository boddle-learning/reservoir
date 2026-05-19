package user

import (
	"context"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

var (
	lastLoginEnqueued = promauto.NewCounter(prometheus.CounterOpts{
		Name: "reservoir_last_login_enqueued_total",
		Help: "User IDs accepted into the last_logged_on batch queue.",
	})
	lastLoginDropped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "reservoir_last_login_dropped_total",
		Help: "User IDs dropped because the last_logged_on queue was full.",
	})
	lastLoginFlushed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "reservoir_last_login_flushed_total",
		Help: "User rows successfully updated in last_logged_on batches.",
	})
	lastLoginBatchErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "reservoir_last_login_batch_errors_total",
		Help: "last_logged_on batch UPDATEs that returned an error.",
	})
	lastLoginQueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "reservoir_last_login_queue_depth",
		Help: "Current depth of the last_logged_on batch queue.",
	})
)

const (
	queueCapacity = 10000
	batchSize     = 500
	flushInterval = 5 * time.Second
	flushTimeout  = 5 * time.Second
)

// LastLoginWriter batches last_logged_on UPDATEs off the auth hot path.
//
// Auth handlers call Enqueue (non-blocking, drops on overflow). A single
// background goroutine flushes accumulated IDs every flushInterval or when
// batchSize is reached, whichever comes first. Per-batch failures are
// counted but never propagated back to callers — last_logged_on is
// best-effort by design, and a slow or failing DB write must not be able
// to stall auth responses.
type LastLoginWriter struct {
	db     *sqlx.DB
	logger *zap.Logger
	queue  chan int
	stop   chan struct{}
	wg     sync.WaitGroup
}

func NewLastLoginWriter(db *sqlx.DB, logger *zap.Logger) *LastLoginWriter {
	w := &LastLoginWriter{
		db:     db,
		logger: logger,
		queue:  make(chan int, queueCapacity),
		stop:   make(chan struct{}),
	}
	w.wg.Add(1)
	go w.run()
	return w
}

// Enqueue submits a user ID for a deferred last_logged_on update.
// Non-blocking: if the queue is full, the ID is dropped and a metric
// is incremented. Safe to call from any goroutine.
func (w *LastLoginWriter) Enqueue(userID int) {
	select {
	case w.queue <- userID:
		lastLoginEnqueued.Inc()
		lastLoginQueueDepth.Set(float64(len(w.queue)))
	default:
		lastLoginDropped.Inc()
	}
}

// Shutdown stops the background flusher and drains the queue with one
// final batch. Returns when draining completes or ctx is cancelled.
func (w *LastLoginWriter) Shutdown(ctx context.Context) {
	close(w.stop)
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		w.logger.Warn("last_login writer shutdown timed out")
	}
}

func (w *LastLoginWriter) run() {
	defer w.wg.Done()

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	pending := make(map[int]struct{}, batchSize)

	flush := func() {
		if len(pending) == 0 {
			return
		}
		ids := make([]int64, 0, len(pending))
		for id := range pending {
			ids = append(ids, int64(id))
		}
		pending = make(map[int]struct{}, batchSize)

		ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
		defer cancel()

		_, err := w.db.ExecContext(ctx,
			`UPDATE users SET last_logged_on = NOW() WHERE id = ANY($1)`,
			pq.Array(ids),
		)
		if err != nil {
			lastLoginBatchErrors.Inc()
			w.logger.Error("last_logged_on batch failed",
				zap.Int("batch_size", len(ids)),
				zap.Error(err),
			)
			return
		}
		lastLoginFlushed.Add(float64(len(ids)))
	}

	for {
		select {
		case <-w.stop:
			for drained := false; !drained; {
				select {
				case id := <-w.queue:
					pending[id] = struct{}{}
				default:
					drained = true
				}
			}
			flush()
			return

		case id := <-w.queue:
			pending[id] = struct{}{}
			lastLoginQueueDepth.Set(float64(len(w.queue)))
			if len(pending) >= batchSize {
				flush()
			}

		case <-ticker.C:
			flush()
		}
	}
}