package eventbus

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestEventBus_BasicPublishSubscribe(t *testing.T) {
	eb := New(&Config{
		MinWorkers: 2,
		MaxWorkers: 4,
		Logger:     zap.NewNop(),
	})
	defer eb.Shutdown(context.Background())

	var received atomic.Int32

	eb.SubscribeSync("test.topic", PriorityMedium, func(ctx context.Context, event Event) error {
		received.Add(1)
		return nil
	})

	event := NewEvent("test.topic", "payload")
	err := eb.PublishSync(event)
	if err != nil {
		t.Fatalf("PublishSync failed: %v", err)
	}

	if received.Load() != 1 {
		t.Errorf("expected 1 received event, got %d", received.Load())
	}
}

func TestEventBus_WildcardSubscription(t *testing.T) {
	eb := New(&Config{
		MinWorkers: 2,
		MaxWorkers: 4,
		Logger:     zap.NewNop(),
	})
	defer eb.Shutdown(context.Background())

	var wildcardReceived atomic.Int32
	var exactReceived atomic.Int32

	eb.SubscribeSync("gameplay.*", PriorityMedium, func(ctx context.Context, event Event) error {
		wildcardReceived.Add(1)
		return nil
	})

	eb.SubscribeSync("gameplay.combat.damage", PriorityMedium, func(ctx context.Context, event Event) error {
		exactReceived.Add(1)
		return nil
	})

	eb.PublishSync(NewEvent("gameplay.combat.damage", nil))

	if wildcardReceived.Load() != 1 {
		t.Errorf("wildcard handler: expected 1, got %d", wildcardReceived.Load())
	}
	if exactReceived.Load() != 1 {
		t.Errorf("exact handler: expected 1, got %d", exactReceived.Load())
	}
}

func TestEventBus_AsyncPublish(t *testing.T) {
	eb := New(&Config{
		MinWorkers: 4,
		MaxWorkers: 8,
		Logger:     zap.NewNop(),
	})
	defer eb.Shutdown(context.Background())

	var received atomic.Int32
	done := make(chan struct{})

	eb.SubscribeAsync("async.topic", PriorityMedium, func(ctx context.Context, event Event) error {
		if received.Add(1) == 100 {
			close(done)
		}
		return nil
	})

	for i := 0; i < 100; i++ {
		eb.PublishAsync(NewEvent("async.topic", i), PriorityMedium)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for async events, received %d", received.Load())
	}

	if received.Load() != 100 {
		t.Errorf("expected 100 received events, got %d", received.Load())
	}
}

func TestEventBus_Priority(t *testing.T) {
	eb := New(&Config{
		MinWorkers: 1,
		MaxWorkers: 1,
		Logger:     zap.NewNop(),
	})
	defer eb.Shutdown(context.Background())

	var order []int
	var mu sync.Mutex

	eb.SubscribeSync("priority.test", PriorityLow, func(ctx context.Context, event Event) error {
		mu.Lock()
		order = append(order, 1)
		mu.Unlock()
		return nil
	})

	eb.SubscribeSync("priority.test", PriorityHigh, func(ctx context.Context, event Event) error {
		mu.Lock()
		order = append(order, 3)
		mu.Unlock()
		return nil
	})

	eb.SubscribeSync("priority.test", PriorityMedium, func(ctx context.Context, event Event) error {
		mu.Lock()
		order = append(order, 2)
		mu.Unlock()
		return nil
	})

	eb.PublishSync(NewEvent("priority.test", nil))

	mu.Lock()
	defer mu.Unlock()

	if len(order) != 3 {
		t.Fatalf("expected 3 handlers, got %d", len(order))
	}

	if order[0] != 3 || order[1] != 2 || order[2] != 1 {
		t.Errorf("expected order [3,2,1] (high,medium,low), got %v", order)
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	eb := New(&Config{
		MinWorkers: 2,
		MaxWorkers: 4,
		Logger:     zap.NewNop(),
	})
	defer eb.Shutdown(context.Background())

	var received atomic.Int32

	token := eb.SubscribeSync("unsub.test", PriorityMedium, func(ctx context.Context, event Event) error {
		received.Add(1)
		return nil
	})

	eb.PublishSync(NewEvent("unsub.test", nil))
	if received.Load() != 1 {
		t.Errorf("expected 1 before unsubscribe, got %d", received.Load())
	}

	eb.Unsubscribe(token)

	eb.PublishSync(NewEvent("unsub.test", nil))
	if received.Load() != 1 {
		t.Errorf("expected still 1 after unsubscribe, got %d", received.Load())
	}
}

func TestEventBus_HandlerError(t *testing.T) {
	var errorCalled atomic.Int32

	eb := New(&Config{
		MinWorkers: 2,
		MaxWorkers: 4,
		Logger:     zap.NewNop(),
		OnError: func(event Event, handlerID string, err error) {
			errorCalled.Add(1)
		},
	})
	defer eb.Shutdown(context.Background())

	eb.SubscribeSync("error.test", PriorityMedium, func(ctx context.Context, event Event) error {
		return fmt.Errorf("test error")
	})

	err := eb.PublishSync(NewEvent("error.test", nil))
	if err == nil {
		t.Error("expected error from PublishSync")
	}

	if errorCalled.Load() != 1 {
		t.Errorf("expected OnError to be called once, got %d", errorCalled.Load())
	}

	dlq := eb.DeadLetterQueue()
	if len(dlq) != 1 {
		t.Errorf("expected 1 entry in dead letter queue, got %d", len(dlq))
	}
}

func TestEventBus_HandlerPanic(t *testing.T) {
	var errorCalled atomic.Int32

	eb := New(&Config{
		MinWorkers: 2,
		MaxWorkers: 4,
		Logger:     zap.NewNop(),
		OnError: func(event Event, handlerID string, err error) {
			errorCalled.Add(1)
		},
	})
	defer eb.Shutdown(context.Background())

	eb.SubscribeSync("panic.test", PriorityMedium, func(ctx context.Context, event Event) error {
		panic("test panic")
	})

	err := eb.PublishSync(NewEvent("panic.test", nil))
	if err == nil {
		t.Error("expected error from PublishSync after panic")
	}

	if errorCalled.Load() != 1 {
		t.Errorf("expected OnError to be called once after panic, got %d", errorCalled.Load())
	}
}

func TestEventBus_Timeout(t *testing.T) {
	eb := New(&Config{
		MinWorkers: 2,
		MaxWorkers: 4,
		Logger:     zap.NewNop(),
	})
	defer eb.Shutdown(context.Background())

	eb.SubscribeSyncWithTimeout("timeout.test", PriorityMedium, func(ctx context.Context, event Event) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
			return nil
		}
	}, 50*time.Millisecond)

	start := time.Now()
	err := eb.PublishSync(NewEvent("timeout.test", nil))
	elapsed := time.Since(start)

	if err == nil {
		t.Error("expected timeout error")
	}

	if elapsed > 200*time.Millisecond {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}

func TestEventBus_BatchPublisher(t *testing.T) {
	eb := New(&Config{
		MinWorkers: 4,
		MaxWorkers: 8,
		Logger:     zap.NewNop(),
	})
	defer eb.Shutdown(context.Background())

	var bulkReceived atomic.Int32
	done := make(chan struct{})

	eb.SubscribeBulkAsync("batch.test", PriorityMedium, func(events []Event) error {
		bulkReceived.Add(int32(len(events)))
		if bulkReceived.Load() >= 50 {
			select {
			case <-done:
			default:
				close(done)
			}
		}
		return nil
	})

	bp := eb.BatchPublisher()
	for i := 0; i < 50; i++ {
		bp.Add(NewEvent("batch.test", i))
	}
	bp.Flush()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for batch events, received %d", bulkReceived.Load())
	}
}

func TestEventBus_GracefulShutdown(t *testing.T) {
	eb := New(&Config{
		MinWorkers: 4,
		MaxWorkers: 8,
		Logger:     zap.NewNop(),
	})

	var processed atomic.Int32

	eb.SubscribeAsync("shutdown.test", PriorityMedium, func(ctx context.Context, event Event) error {
		time.Sleep(10 * time.Millisecond)
		processed.Add(1)
		return nil
	})

	for i := 0; i < 100; i++ {
		eb.PublishAsync(NewEvent("shutdown.test", i), PriorityMedium)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := eb.Shutdown(ctx)
	if err != nil {
		t.Errorf("shutdown error: %v", err)
	}

	if processed.Load() < 50 {
		t.Errorf("expected at least 50 processed events during shutdown, got %d", processed.Load())
	}
}

func BenchmarkEventBus_PublishSync(b *testing.B) {
	eb := New(&Config{
		MinWorkers: runtime.NumCPU(),
		MaxWorkers: runtime.NumCPU() * 2,
		Logger:     zap.NewNop(),
	})
	defer eb.Shutdown(context.Background())

	eb.SubscribeSync("bench.sync", PriorityMedium, func(ctx context.Context, event Event) error {
		return nil
	})

	event := NewEvent("bench.sync", "payload")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		eb.PublishSync(event)
	}
}

func BenchmarkEventBus_PublishAsync(b *testing.B) {
	eb := New(&Config{
		MinWorkers: runtime.NumCPU(),
		MaxWorkers: runtime.NumCPU() * 4,
		Logger:     zap.NewNop(),
	})
	defer eb.Shutdown(context.Background())

	var processed atomic.Int64

	eb.SubscribeAsync("bench.async", PriorityMedium, func(ctx context.Context, event Event) error {
		processed.Add(1)
		return nil
	})

	event := NewEvent("bench.async", "payload")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		eb.PublishAsync(event, PriorityMedium)
	}

	b.StopTimer()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	eb.Shutdown(ctx)
}

func BenchmarkEventBus_PublishAsync_Parallel(b *testing.B) {
	eb := New(&Config{
		MinWorkers: runtime.NumCPU(),
		MaxWorkers: runtime.NumCPU() * 4,
		Logger:     zap.NewNop(),
	})
	defer eb.Shutdown(context.Background())

	eb.SubscribeAsync("bench.parallel", PriorityMedium, func(ctx context.Context, event Event) error {
		return nil
	})

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		event := NewEvent("bench.parallel", "payload")
		for pb.Next() {
			eb.PublishAsync(event, PriorityMedium)
		}
	})
}

func BenchmarkEventBus_WildcardMatching(b *testing.B) {
	eb := New(&Config{
		MinWorkers: runtime.NumCPU(),
		MaxWorkers: runtime.NumCPU() * 2,
		Logger:     zap.NewNop(),
	})
	defer eb.Shutdown(context.Background())

	eb.SubscribeSync("gameplay.*", PriorityMedium, func(ctx context.Context, event Event) error {
		return nil
	})
	eb.SubscribeSync("gameplay.combat.*", PriorityMedium, func(ctx context.Context, event Event) error {
		return nil
	})
	eb.SubscribeSync("*", PriorityLow, func(ctx context.Context, event Event) error {
		return nil
	})

	event := NewEvent("gameplay.combat.damage", "payload")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		eb.PublishSync(event)
	}
}

func BenchmarkEventBus_HighContention(b *testing.B) {
	eb := New(&Config{
		MinWorkers: runtime.NumCPU() * 2,
		MaxWorkers: runtime.NumCPU() * 8,
		Logger:     zap.NewNop(),
	})
	defer eb.Shutdown(context.Background())

	for i := 0; i < 10; i++ {
		topic := fmt.Sprintf("contention.topic.%d", i)
		eb.SubscribeAsync(topic, PriorityMedium, func(ctx context.Context, event Event) error {
			return nil
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			topic := fmt.Sprintf("contention.topic.%d", i%10)
			eb.PublishAsync(NewEvent(topic, i), PriorityMedium)
			i++
		}
	})
}

func BenchmarkEventBus_MixedPriority(b *testing.B) {
	eb := New(&Config{
		MinWorkers: runtime.NumCPU(),
		MaxWorkers: runtime.NumCPU() * 4,
		Logger:     zap.NewNop(),
	})
	defer eb.Shutdown(context.Background())

	eb.SubscribeAsync("mixed.priority", PriorityHigh, func(ctx context.Context, event Event) error {
		return nil
	})
	eb.SubscribeAsync("mixed.priority", PriorityMedium, func(ctx context.Context, event Event) error {
		return nil
	})
	eb.SubscribeAsync("mixed.priority", PriorityLow, func(ctx context.Context, event Event) error {
		return nil
	})

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		priorities := []Priority{PriorityHigh, PriorityMedium, PriorityLow}
		i := 0
		for pb.Next() {
			eb.PublishAsync(NewEvent("mixed.priority", i), priorities[i%3])
			i++
		}
	})
}
