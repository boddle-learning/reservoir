package user

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/zap"
)

// fakeExecutor stands in for *sqlx.DB in tests. It records every
// ExecContext call (count and decoded IDs) and supports injecting
// latency and errors to exercise the writer's edge cases.
type fakeExecutor struct {
	mu      sync.Mutex
	calls   int
	idsSeen [][]int64
	err     error
	latency time.Duration
	onExec  func(ids []int64)
}

type noopResult struct{}

func (noopResult) LastInsertId() (int64, error) { return 0, nil }
func (noopResult) RowsAffected() (int64, error) { return 0, nil }

func (f *fakeExecutor) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	if f.latency > 0 {
		select {
		case <-time.After(f.latency):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	ids := extractInt64s(args)
	f.mu.Lock()
	f.calls++
	f.idsSeen = append(f.idsSeen, append([]int64(nil), ids...))
	cb := f.onExec
	err := f.err
	f.mu.Unlock()
	if cb != nil {
		cb(ids)
	}
	return noopResult{}, err
}

// extractInt64s recovers the slice the writer wrapped in pq.Array.
// pq.Array([]int64{...}) returns *pq.Int64Array, which is just a
// named type over []int64.
func extractInt64s(args []interface{}) []int64 {
	for _, a := range args {
		if arr, ok := a.(*pq.Int64Array); ok {
			return []int64(*arr)
		}
		if arr, ok := a.(pq.Int64Array); ok {
			return []int64(arr)
		}
		if ids, ok := a.([]int64); ok {
			return ids
		}
	}
	return nil
}

func TestEnqueue_DropsWhenFull(t *testing.T) {
	// Block the flusher with a long latency so the queue actually fills.
	exec := &fakeExecutor{latency: 500 * time.Millisecond}
	w := newLastLoginWriter(exec, zap.NewNop())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		w.Shutdown(ctx)
	})

	dropsBefore := testutil.ToFloat64(lastLoginDropped)

	// Push well past the queue capacity. The flusher is blocked, so once
	// the channel buffer (queueCapacity) and the in-progress map fill,
	// further enqueues must drop.
	for i := 0; i < queueCapacity*2; i++ {
		w.Enqueue(i)
	}

	drops := testutil.ToFloat64(lastLoginDropped) - dropsBefore
	if drops == 0 {
		t.Fatalf("expected drops once queue saturated, got 0")
	}
}

func TestFlush_TriggersAtBatchSize(t *testing.T) {
	flushed := make(chan int, 4)
	exec := &fakeExecutor{
		onExec: func(ids []int64) { flushed <- len(ids) },
	}
	w := newLastLoginWriter(exec, zap.NewNop())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		w.Shutdown(ctx)
	})

	// Enqueue exactly batchSize unique IDs — flush fires as soon as the
	// pending map reaches batchSize, without waiting for the ticker.
	for i := 1; i <= batchSize; i++ {
		w.Enqueue(i)
	}

	select {
	case n := <-flushed:
		if n != batchSize {
			t.Fatalf("expected batch of %d, got %d", batchSize, n)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("expected batch-size flush within 2s, got none")
	}
}

func TestShutdown_DrainsPendingIDs(t *testing.T) {
	flushed := make(chan int, 1)
	exec := &fakeExecutor{
		onExec: func(ids []int64) { flushed <- len(ids) },
	}
	w := newLastLoginWriter(exec, zap.NewNop())

	w.Enqueue(1)
	w.Enqueue(2)
	w.Enqueue(3)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	w.Shutdown(ctx)

	select {
	case n := <-flushed:
		if n != 3 {
			t.Fatalf("expected 3 IDs in shutdown drain, got %d", n)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("expected one flush from shutdown drain, got none")
	}
}

func TestFlush_DeduplicatesIDs(t *testing.T) {
	flushed := make(chan int, 1)
	exec := &fakeExecutor{
		onExec: func(ids []int64) { flushed <- len(ids) },
	}
	w := newLastLoginWriter(exec, zap.NewNop())

	for i := 0; i < 50; i++ {
		w.Enqueue(42)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	w.Shutdown(ctx)

	select {
	case n := <-flushed:
		if n != 1 {
			t.Fatalf("expected dedupe to 1 ID, got %d", n)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("expected final flush, got none")
	}
}

func TestFlush_ErrorIncrementsCounters(t *testing.T) {
	exec := &fakeExecutor{err: errors.New("boom")}
	w := newLastLoginWriter(exec, zap.NewNop())

	batchErrBefore := testutil.ToFloat64(lastLoginBatchErrors)
	dbWriteErrBefore := testutil.ToFloat64(authDBWriteErrors.WithLabelValues("last_logged_on"))

	w.Enqueue(1)
	w.Enqueue(2)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	w.Shutdown(ctx)

	if got := testutil.ToFloat64(lastLoginBatchErrors) - batchErrBefore; got != 1 {
		t.Errorf("expected batch_errors +1, got %v", got)
	}
	if got := testutil.ToFloat64(authDBWriteErrors.WithLabelValues("last_logged_on")) - dbWriteErrBefore; got != 1 {
		t.Errorf("expected auth_db_write_errors{operation=last_logged_on} +1, got %v", got)
	}
}

func TestShutdown_RespectsContextDeadline(t *testing.T) {
	block := make(chan struct{})
	exec := &fakeExecutor{
		onExec: func(_ []int64) { <-block },
	}
	w := newLastLoginWriter(exec, zap.NewNop())
	t.Cleanup(func() { close(block) })

	w.Enqueue(1)

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	w.Shutdown(ctx)
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("Shutdown should return near ctx deadline (~100ms), took %v", elapsed)
	}
}
