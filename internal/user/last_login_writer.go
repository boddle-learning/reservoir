package user

import (
	"context"
	"database/sql"
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

	// authDBWriteErrors counts DB write errors on the auth path, labeled
	// by operation. Lives here rather than internal/middleware to avoid
	// a middleware→auth→user→middleware import cycle; callers from other
	// auth-path packages can import this via internal/user.
	authDBWriteErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reservoir_auth_db_write_errors_total",
			Help: "Database write errors on the auth path, by operation. Spikes here preceded the 2026-05-19 cascade outage by ~31 hours and would have surfaced the read-only DB_HOST in seconds.",
		},
		[]string{"operation"},
	)
)

// RecordAuthDBWriteError increments the auth-path DB write error counter
// for the given operation (e.g. "last_logged_on", "login_token_delete").
func RecordAuthDBWriteError(operation string) {
	authDBWriteErrors.WithLabelValues(operation).Inc()
}

const (
	queueCapacity = 10000
	batchSize     = 500
	flushInterval = 5 * time.Second
	flushTimeout  = 5 * time.Second
)

// LastLoginEnqueuer defers last_logged_on updates off the synchronous
// auth path. Implementations must not block on the database. Wired in
// response to the 2026-05-19 outage, where synchronous UPDATE failures
// against a read-only DB endpoint contributed to CPU saturation.
//
// Lives in package user (alongside *LastLoginWriter, the production
// implementation) so both auth and oauth — which already import user
// for Repository — can depend on it without sibling-package coupling.
type LastLoginEnqueuer interface {
	Enqueue(userID int)
}

// sqlExecutor is the subset of *sqlx.DB the writer needs. Defined as an
// interface so tests can substitute a fake without a live database.
type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

// LastLoginWriter batches last_logged_on UPDATEs off the auth hot path.
//
// Auth handlers call Enqueue (non-blocking, drops on overflow). A single
// background goroutine flushes accumulated IDs every flushInterval or when
// batchSize is reached, whichever comes first. Per-batch failures are
// counted but never propagated back to callers — last_logged_on is
// best-effort by design, and a slow or failing DB write must not be able
// to stall auth responses.
type LastLoginWriter struct {
	db     sqlExecutor
	logger *zap.Logger
	queue  chan int
	// stop carries the caller's shutdown context so the final drain flush
	// honors the same deadline as the rest of graceful shutdown. Buffered
	// (cap 1) so Shutdown never blocks.
	stop chan context.Context
	wg   sync.WaitGroup
}

func NewLastLoginWriter(db *sqlx.DB, logger *zap.Logger) *LastLoginWriter {
	return newLastLoginWriter(db, logger)
}

func newLastLoginWriter(db sqlExecutor, logger *zap.Logger) *LastLoginWriter {
	w := &LastLoginWriter{
		db:     db,
		logger: logger,
		queue:  make(chan int, queueCapacity),
		stop:   make(chan context.Context, 1),
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
// final batch. The passed ctx bounds the final flush; if it expires
// before draining completes, Shutdown returns and the goroutine is
// abandoned (process is exiting anyway). Not safe to call twice — the
// stop channel is buffered cap-1 and run() consumes the value exactly
// once, so a second call would block forever on the send.
func (w *LastLoginWriter) Shutdown(ctx context.Context) {
	w.stop <- ctx
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

	flush := func(parent context.Context) {
		if len(pending) == 0 {
			return
		}
		ids := make([]int64, 0, len(pending))
		for id := range pending {
			ids = append(ids, int64(id))
		}
		pending = make(map[int]struct{}, batchSize)

		ctx, cancel := context.WithTimeout(parent, flushTimeout)
		defer cancel()

		_, err := w.db.ExecContext(ctx,
			`UPDATE users SET last_logged_on = NOW() WHERE id = ANY($1)`,
			pq.Array(ids),
		)
		if err != nil {
			lastLoginBatchErrors.Inc()
			RecordAuthDBWriteError("last_logged_on")
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
		case shutdownCtx := <-w.stop:
			for drained := false; !drained; {
				select {
				case id := <-w.queue:
					pending[id] = struct{}{}
				default:
					drained = true
				}
			}
			flush(shutdownCtx)
			return

		case id := <-w.queue:
			pending[id] = struct{}{}
			lastLoginQueueDepth.Set(float64(len(w.queue)))
			if len(pending) >= batchSize {
				flush(context.Background())
			}

		case <-ticker.C:
			flush(context.Background())
		}
	}
}
