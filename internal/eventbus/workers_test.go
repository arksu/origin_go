package eventbus

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

const workersTestTimeout = 5 * time.Second

func waitWorkersTest(t *testing.T, done <-chan struct{}, operation string) {
	t.Helper()
	timer := time.NewTimer(workersTestTimeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		t.Fatalf("timed out waiting for %s", operation)
	}
}

func snapshotTestWorkers(t *testing.T, eb *EventBus) []*worker {
	t.Helper()
	eb.workersMu.Lock()
	defer eb.workersMu.Unlock()
	if got := eb.workerCount.Load(); got != int64(len(eb.workers)) {
		t.Errorf("worker count = %d, registered workers = %d", got, len(eb.workers))
	}
	return append([]*worker(nil), eb.workers...)
}

// Synthetic workers make scale-down thresholds deterministic without changing
// activeWorker's production meaning (it includes workers waiting for messages).
func newSyntheticWorkersBus(t *testing.T, minimum, maximum, current, active, depth int) *EventBus {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	eb := &EventBus{
		logger:         zap.NewNop(),
		highPriorityCh: make(chan *message, max(depth, 1)),
		minWorkers:     minimum,
		maxWorkers:     maximum,
		ctx:            ctx,
		cancel:         cancel,
		batchPublisher: &BatchPublisher{stopCh: make(chan struct{})},
	}
	for i := 0; i < current; i++ {
		eb.workers = append(eb.workers, &worker{eb: eb, stopCh: make(chan struct{})})
	}
	eb.workerCount.Store(int64(current))
	atomic.StoreInt32(&eb.activeWorker, int32(active))
	for i := 0; i < depth; i++ {
		eb.highPriorityCh <- &message{event: NewEvent("workers.synthetic", nil)}
	}
	t.Cleanup(func() {
		cancel()
		done := make(chan struct{})
		go func() {
			eb.wg.Wait()
			close(done)
		}()
		waitWorkersTest(t, done, "synthetic workers to exit")
	})
	return eb
}

func TestEventBus_ConcurrentWorkersGrowth(t *testing.T) {
	const (
		maximum      = 8
		publishers   = 8
		perPublisher = 100
		total        = publishers * perPublisher
	)
	eb := New(&Config{MinWorkers: 1, MaxWorkers: maximum, Logger: zap.NewNop()})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		ctx, cancel := context.WithTimeout(context.Background(), workersTestTimeout)
		defer cancel()
		if err := eb.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})

	started := make(chan struct{}, total)
	delivered := make(chan int, total)
	eb.SubscribeAsync("workers.growth", PriorityHigh, func(ctx context.Context, event Event) error {
		started <- struct{}{}
		select {
		case <-release:
			delivered <- event.(*BaseEvent).Payload.(int)
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	// Occupy the initial worker before the concurrent queue fill.
	eb.PublishAsync(NewEvent("workers.growth", 0), PriorityHigh)
	select {
	case <-started:
	case <-time.After(workersTestTimeout):
		t.Fatal("initial worker did not enter its handler")
	}

	start := make(chan struct{})
	var publishing sync.WaitGroup
	for publisher := 0; publisher < publishers; publisher++ {
		publishing.Add(1)
		go func(publisher int) {
			defer publishing.Done()
			<-start
			for i := 0; i < perPublisher; i++ {
				id := publisher*perPublisher + i
				if id != 0 {
					eb.PublishAsync(NewEvent("workers.growth", id), PriorityHigh)
				}
			}
		}(publisher)
	}
	close(start)
	published := make(chan struct{})
	go func() {
		publishing.Wait()
		close(published)
	}()
	waitWorkersTest(t, published, "concurrent publishers")

	workers := snapshotTestWorkers(t, eb)
	if len(workers) <= 1 || len(workers) > maximum {
		t.Fatalf("expected pool growth within [2, %d], got %d", maximum, len(workers))
	}
	releaseOnce.Do(func() { close(release) })

	seen := make([]bool, total)
	timer := time.NewTimer(workersTestTimeout)
	defer timer.Stop()
	for i := 0; i < total; i++ {
		select {
		case id := <-delivered:
			if id < 0 || id >= total || seen[id] {
				t.Fatalf("invalid or duplicate delivered event %d", id)
			}
			seen[id] = true
		case <-timer.C:
			t.Fatalf("received %d of %d events", i, total)
		}
	}
}

func TestEventBus_ShrinkWorkers(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		minimum, current, first, final int
	}{
		{name: "quarter", minimum: 4, current: 12, first: 9, final: 4},
		{name: "minimum", minimum: 4, current: 5, first: 4, final: 4},
		{name: "integer rounding", minimum: 1, current: 4, first: 3, final: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eb := newSyntheticWorkersBus(t, tc.minimum, tc.current, tc.current, 0, 0)
			original := snapshotTestWorkers(t, eb)
			eb.scaleWorkers()
			if got := len(snapshotTestWorkers(t, eb)); got != tc.first {
				t.Fatalf("first scale-down: got %d workers, want %d", got, tc.first)
			}
			for i := 0; i < tc.current; i++ {
				eb.scaleWorkers()
			}
			remaining := snapshotTestWorkers(t, eb)
			if len(remaining) != tc.final {
				t.Fatalf("final worker count = %d, want %d", len(remaining), tc.final)
			}
			for i, w := range original {
				select {
				case <-w.stopCh:
					if i < tc.final {
						t.Errorf("retained worker %d stopped", i)
					}
				default:
					if i >= tc.final {
						t.Errorf("removed worker %d was not stopped", i)
					}
				}
				if i < tc.final && remaining[i] != w {
					t.Errorf("retained worker %d changed", i)
				}
			}
		})
	}
}

func TestEventBus_NoChangeWorkers(t *testing.T) {
	for _, tc := range []struct {
		name                                     string
		minimum, maximum, current, active, depth int
	}{
		{name: "fixed pool", minimum: 4, maximum: 4, current: 4, depth: 900},
		{name: "minimum", minimum: 2, maximum: 8, current: 2, depth: 0},
		{name: "between thresholds", minimum: 1, maximum: 8, current: 4, depth: 120},
		{name: "active includes waiting", minimum: 1, maximum: 8, current: 8, active: 4},
		{name: "quarter rounds to zero", minimum: 1, maximum: 8, current: 3},
		{name: "maximum", minimum: 1, maximum: 8, current: 8, depth: 900},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eb := newSyntheticWorkersBus(t, tc.minimum, tc.maximum, tc.current, tc.active, tc.depth)
			before := snapshotTestWorkers(t, eb)
			eb.workersMu.Lock()
			func() {
				defer eb.workersMu.Unlock()
				done := make(chan struct{})
				go func() {
					eb.scaleWorkers()
					close(done)
				}()
				waitWorkersTest(t, done, "no-change scaling with workersMu held")
			}()
			eb.scaleWorkers()
			after := snapshotTestWorkers(t, eb)
			if len(after) != len(before) {
				t.Fatalf("worker count changed: %d -> %d", len(before), len(after))
			}
			for i, w := range before {
				if after[i] != w {
					t.Errorf("worker %d changed", i)
				}
				select {
				case <-w.stopCh:
					t.Errorf("worker %d stopped", i)
				default:
				}
			}
		})
	}
}

func TestEventBus_TryLockWorkers(t *testing.T) {
	eb := newSyntheticWorkersBus(t, 1, 6, 2, 2, 900)
	eb.workersMu.Lock()
	locked := true
	defer func() {
		if locked {
			eb.workersMu.Unlock()
		}
	}()
	done := make(chan struct{})
	go func() {
		eb.scaleWorkers()
		close(done)
	}()
	waitWorkersTest(t, done, "scaleWorkers with workersMu held")
	if got := eb.workerCount.Load(); got != 2 {
		t.Fatalf("busy mutex changed worker count to %d", got)
	}
	eb.workersMu.Unlock()
	locked = false

	eb.scaleWorkers()
	if got := len(snapshotTestWorkers(t, eb)); got != 6 {
		t.Fatalf("retry after mutex release: got %d workers, want 6", got)
	}
}

func TestEventBus_ShutdownWorkers(t *testing.T) {
	eb := newSyntheticWorkersBus(t, 1, 8, 1, 0, 800)
	eb.workersMu.Lock()
	locked := true
	defer func() {
		if locked {
			eb.workersMu.Unlock()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), workersTestTimeout)
	defer cancel()
	shutdownDone := make(chan struct{})
	var shutdownErr error
	go func() {
		shutdownErr = eb.Shutdown(ctx)
		close(shutdownDone)
	}()
	deadline := time.Now().Add(workersTestTimeout)
	for !eb.isShutdown.Load() {
		if time.Now().After(deadline) {
			t.Fatal("Shutdown did not publish its stop flag")
		}
		runtime.Gosched()
	}

	// The held mutex represents a registration already in flight. Shutdown
	// must wait for it, and concurrent scale attempts must not register more.
	var scaling sync.WaitGroup
	for i := 0; i < 8; i++ {
		scaling.Add(1)
		go func() {
			defer scaling.Done()
			eb.scaleWorkers()
		}()
	}
	scaled := make(chan struct{})
	go func() {
		scaling.Wait()
		close(scaled)
	}()
	waitWorkersTest(t, scaled, "scaling during shutdown")
	select {
	case <-shutdownDone:
		t.Fatal("Shutdown completed before the registration barrier was released")
	default:
	}
	eb.workersMu.Unlock()
	locked = false
	waitWorkersTest(t, shutdownDone, "Shutdown")
	if shutdownErr != nil {
		t.Fatalf("Shutdown: %v", shutdownErr)
	}

	// Backlogged work after shutdown must not trigger a late WaitGroup.Add.
	for i := 0; i < 800; i++ {
		eb.highPriorityCh <- &message{event: NewEvent("workers.synthetic", nil)}
	}
	eb.scaleWorkers()
	if got := len(snapshotTestWorkers(t, eb)); got != 1 {
		t.Fatalf("registered %d workers after shutdown, want 1", got)
	}
}
