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

func TestLoad_HighThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	eb := New(&Config{
		MinWorkers: runtime.NumCPU(),
		MaxWorkers: runtime.NumCPU() * 4,
		Logger:     zap.NewNop(),
	})

	const (
		numPublishers = 2
		eventsPerPub  = 5_000
		totalEvents   = numPublishers * eventsPerPub
	)

	var processed atomic.Int64
	var wg sync.WaitGroup

	eb.SubscribeAsync("load.test", PriorityMedium, func(ctx context.Context, event Event) error {
		processed.Add(1)
		return nil
	})

	start := time.Now()

	for p := 0; p < numPublishers; p++ {
		wg.Add(1)
		go func(publisherID int) {
			defer wg.Done()
			for i := 0; i < eventsPerPub; i++ {
				eb.PublishAsync(NewEvent("load.test", i), PriorityMedium)
			}
		}(p)
	}

	wg.Wait()
	publishDuration := time.Since(start)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout: processed %d/%d events", processed.Load(), totalEvents)
		case <-ticker.C:
			if processed.Load() >= totalEvents {
				goto done
			}
		}
	}

done:
	totalDuration := time.Since(start)

	t.Logf("High Throughput Test Results:")
	t.Logf("  Total events: %d", totalEvents)
	t.Logf("  Publishers: %d", numPublishers)
	t.Logf("  Publish duration: %v", publishDuration)
	t.Logf("  Total duration: %v", totalDuration)
	t.Logf("  Publish rate: %.0f events/sec", float64(totalEvents)/publishDuration.Seconds())
	t.Logf("  Process rate: %.0f events/sec", float64(totalEvents)/totalDuration.Seconds())

	eb.Shutdown(context.Background())
}

func TestLoad_MixedPriorities(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	eb := New(&Config{
		MinWorkers: runtime.NumCPU(),
		MaxWorkers: runtime.NumCPU() * 4,
		Logger:     zap.NewNop(),
	})

	const eventsPerPriority = 5_000

	var highProcessed, mediumProcessed, lowProcessed atomic.Int64
	var highFirst, mediumFirst, lowFirst atomic.Int64
	var orderCounter atomic.Int64

	eb.SubscribeAsync("priority.high", PriorityHigh, func(ctx context.Context, event Event) error {
		order := orderCounter.Add(1)
		if highFirst.Load() == 0 {
			highFirst.Store(order)
		}
		highProcessed.Add(1)
		return nil
	})

	eb.SubscribeAsync("priority.medium", PriorityMedium, func(ctx context.Context, event Event) error {
		order := orderCounter.Add(1)
		if mediumFirst.Load() == 0 {
			mediumFirst.Store(order)
		}
		mediumProcessed.Add(1)
		return nil
	})

	eb.SubscribeAsync("priority.low", PriorityLow, func(ctx context.Context, event Event) error {
		order := orderCounter.Add(1)
		if lowFirst.Load() == 0 {
			lowFirst.Store(order)
		}
		lowProcessed.Add(1)
		return nil
	})

	start := time.Now()

	for i := 0; i < eventsPerPriority; i++ {
		eb.PublishAsync(NewEvent("priority.low", i), PriorityLow)
	}
	for i := 0; i < eventsPerPriority; i++ {
		eb.PublishAsync(NewEvent("priority.medium", i), PriorityMedium)
	}
	for i := 0; i < eventsPerPriority; i++ {
		eb.PublishAsync(NewEvent("priority.high", i), PriorityHigh)
	}

	totalExpected := int64(eventsPerPriority * 3)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout: processed %d/%d events",
				highProcessed.Load()+mediumProcessed.Load()+lowProcessed.Load(), totalExpected)
		case <-ticker.C:
			total := highProcessed.Load() + mediumProcessed.Load() + lowProcessed.Load()
			if total >= totalExpected {
				goto done
			}
		}
	}

done:
	duration := time.Since(start)

	t.Logf("Mixed Priority Test Results:")
	t.Logf("  Duration: %v", duration)
	t.Logf("  High priority: %d (first at order %d)", highProcessed.Load(), highFirst.Load())
	t.Logf("  Medium priority: %d (first at order %d)", mediumProcessed.Load(), mediumFirst.Load())
	t.Logf("  Low priority: %d (first at order %d)", lowProcessed.Load(), lowFirst.Load())

	if highFirst.Load() > mediumFirst.Load() || mediumFirst.Load() > lowFirst.Load() {
		t.Logf("  Note: Priority ordering may vary due to concurrent processing")
	}

	eb.Shutdown(context.Background())
}

func TestLoad_WildcardScaling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	eb := New(&Config{
		MinWorkers: runtime.NumCPU(),
		MaxWorkers: runtime.NumCPU() * 4,
		Logger:     zap.NewNop(),
	})

	const (
		numTopics      = 20
		eventsPerTopic = 500
	)

	var wildcardHits atomic.Int64
	var exactHits atomic.Int64

	eb.SubscribeAsync("gameplay.*", PriorityMedium, func(ctx context.Context, event Event) error {
		wildcardHits.Add(1)
		return nil
	})

	eb.SubscribeAsync("gameplay.combat.*", PriorityMedium, func(ctx context.Context, event Event) error {
		wildcardHits.Add(1)
		return nil
	})

	for i := 0; i < numTopics; i++ {
		topic := fmt.Sprintf("gameplay.combat.action%d", i)
		eb.SubscribeAsync(topic, PriorityMedium, func(ctx context.Context, event Event) error {
			exactHits.Add(1)
			return nil
		})
	}

	start := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < numTopics; i++ {
		wg.Add(1)
		go func(topicIdx int) {
			defer wg.Done()
			topic := fmt.Sprintf("gameplay.combat.action%d", topicIdx)
			for j := 0; j < eventsPerTopic; j++ {
				eb.PublishAsync(NewEvent(topic, j), PriorityMedium)
			}
		}(i)
	}

	wg.Wait()
	publishDuration := time.Since(start)

	totalExpected := int64(numTopics * eventsPerTopic)
	expectedWildcard := totalExpected * 2
	expectedExact := totalExpected

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout: wildcard=%d/%d, exact=%d/%d",
				wildcardHits.Load(), expectedWildcard, exactHits.Load(), expectedExact)
		case <-ticker.C:
			if wildcardHits.Load() >= expectedWildcard && exactHits.Load() >= expectedExact {
				goto done
			}
		}
	}

done:
	totalDuration := time.Since(start)

	t.Logf("Wildcard Scaling Test Results:")
	t.Logf("  Topics: %d, Events per topic: %d", numTopics, eventsPerTopic)
	t.Logf("  Publish duration: %v", publishDuration)
	t.Logf("  Total duration: %v", totalDuration)
	t.Logf("  Wildcard hits: %d", wildcardHits.Load())
	t.Logf("  Exact hits: %d", exactHits.Load())
	t.Logf("  Rate: %.0f events/sec", float64(totalExpected)/totalDuration.Seconds())

	eb.Shutdown(context.Background())
}

func TestLoad_BurstTraffic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	eb := New(&Config{
		MinWorkers: runtime.NumCPU(),
		MaxWorkers: runtime.NumCPU() * 4,
		Logger:     zap.NewNop(),
	})

	const (
		burstSize  = 1_000
		numBursts  = 5
		burstDelay = 50 * time.Millisecond
	)

	var processed atomic.Int64
	var maxQueueDepth atomic.Int64

	eb.SubscribeAsync("burst.test", PriorityMedium, func(ctx context.Context, event Event) error {
		time.Sleep(100 * time.Microsecond)
		processed.Add(1)
		return nil
	})

	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			high, medium, low := eb.QueueDepth()
			total := int64(high + medium + low)
			for {
				old := maxQueueDepth.Load()
				if total <= old || maxQueueDepth.CompareAndSwap(old, total) {
					break
				}
			}
		}
	}()

	start := time.Now()

	for burst := 0; burst < numBursts; burst++ {
		for i := 0; i < burstSize; i++ {
			eb.PublishAsync(NewEvent("burst.test", i), PriorityMedium)
		}
		time.Sleep(burstDelay)
	}

	totalExpected := int64(burstSize * numBursts)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout: processed %d/%d events", processed.Load(), totalExpected)
		case <-ticker.C:
			if processed.Load() >= totalExpected {
				goto done
			}
		}
	}

done:
	duration := time.Since(start)

	t.Logf("Burst Traffic Test Results:")
	t.Logf("  Bursts: %d, Size per burst: %d", numBursts, burstSize)
	t.Logf("  Total duration: %v", duration)
	t.Logf("  Processed: %d", processed.Load())
	t.Logf("  Max queue depth: %d", maxQueueDepth.Load())
	t.Logf("  Rate: %.0f events/sec", float64(totalExpected)/duration.Seconds())

	eb.Shutdown(context.Background())
}

func TestLoad_SubscribeUnsubscribe(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	eb := New(&Config{
		MinWorkers: runtime.NumCPU(),
		MaxWorkers: runtime.NumCPU() * 4,
		Logger:     zap.NewNop(),
	})

	const (
		numOperations = 1_000
		numPublishers = 2
	)

	var subscribed atomic.Int64
	var unsubscribed atomic.Int64
	var published atomic.Int64

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numOperations; i++ {
			token := eb.SubscribeAsync("churn.test", PriorityMedium, func(ctx context.Context, event Event) error {
				return nil
			})
			subscribed.Add(1)

			if i%2 == 0 {
				eb.Unsubscribe(token)
				unsubscribed.Add(1)
			}
		}
	}()

	for p := 0; p < numPublishers; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < numOperations; i++ {
				eb.PublishAsync(NewEvent("churn.test", i), PriorityMedium)
				published.Add(1)
			}
		}()
	}

	wg.Wait()

	t.Logf("Subscribe/Unsubscribe Churn Test Results:")
	t.Logf("  Subscribed: %d", subscribed.Load())
	t.Logf("  Unsubscribed: %d", unsubscribed.Load())
	t.Logf("  Published: %d", published.Load())

	eb.Shutdown(context.Background())
}

func TestLoad_ErrorRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	var errors atomic.Int64

	eb := New(&Config{
		MinWorkers: runtime.NumCPU(),
		MaxWorkers: runtime.NumCPU() * 4,
		Logger:     zap.NewNop(),
		OnError: func(event Event, handlerID string, err error) {
			errors.Add(1)
		},
	})

	const totalEvents = 10_000

	var processed atomic.Int64

	eb.SubscribeAsync("error.test", PriorityMedium, func(ctx context.Context, event Event) error {
		processed.Add(1)
		if processed.Load()%100 == 0 {
			return fmt.Errorf("simulated error")
		}
		if processed.Load()%500 == 0 {
			panic("simulated panic")
		}
		return nil
	})

	start := time.Now()

	for i := 0; i < totalEvents; i++ {
		eb.PublishAsync(NewEvent("error.test", i), PriorityMedium)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout: processed %d/%d events", processed.Load(), totalEvents)
		case <-ticker.C:
			if processed.Load() >= totalEvents {
				goto done
			}
		}
	}

done:
	duration := time.Since(start)

	dlq := eb.DeadLetterQueue()

	t.Logf("Error Recovery Test Results:")
	t.Logf("  Duration: %v", duration)
	t.Logf("  Processed: %d", processed.Load())
	t.Logf("  Errors handled: %d", errors.Load())
	t.Logf("  Dead letter queue size: %d", len(dlq))
	t.Logf("  Rate: %.0f events/sec", float64(totalEvents)/duration.Seconds())

	eb.Shutdown(context.Background())
}

func TestLoad_MemoryStability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	eb := New(&Config{
		MinWorkers: runtime.NumCPU(),
		MaxWorkers: runtime.NumCPU() * 4,
		Logger:     zap.NewNop(),
	})

	const (
		iterations    = 3
		eventsPerIter = 10_000
	)

	var processed atomic.Int64

	eb.SubscribeAsync("memory.test", PriorityMedium, func(ctx context.Context, event Event) error {
		processed.Add(1)
		return nil
	})

	var memStats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memStats)
	initialAlloc := memStats.Alloc

	for iter := 0; iter < iterations; iter++ {
		for i := 0; i < eventsPerIter; i++ {
			event := eb.AcquireEvent("memory.test", i)
			eb.PublishAsync(event, PriorityMedium)
		}

		for processed.Load() < int64((iter+1)*eventsPerIter) {
			time.Sleep(10 * time.Millisecond)
		}

		runtime.GC()
		runtime.ReadMemStats(&memStats)
		t.Logf("  Iteration %d: Alloc=%dMB, TotalAlloc=%dMB, NumGC=%d",
			iter+1, memStats.Alloc/1024/1024, memStats.TotalAlloc/1024/1024, memStats.NumGC)
	}

	runtime.GC()
	runtime.ReadMemStats(&memStats)
	finalAlloc := memStats.Alloc

	t.Logf("Memory Stability Test Results:")
	t.Logf("  Initial alloc: %d MB", initialAlloc/1024/1024)
	t.Logf("  Final alloc: %d MB", finalAlloc/1024/1024)
	t.Logf("  Total processed: %d", processed.Load())

	if finalAlloc > initialAlloc*10 {
		t.Errorf("possible memory leak: final=%dMB, initial=%dMB", finalAlloc/1024/1024, initialAlloc/1024/1024)
	}

	eb.Shutdown(context.Background())
}

func TestLoad_Latency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	eb := New(&Config{
		MinWorkers: runtime.NumCPU(),
		MaxWorkers: runtime.NumCPU() * 4,
		Logger:     zap.NewNop(),
	})

	const numSamples = 1_000

	latencies := make([]time.Duration, 0, numSamples)
	var mu sync.Mutex
	var wg sync.WaitGroup

	eb.SubscribeSync("latency.test", PriorityHigh, func(ctx context.Context, event Event) error {
		return nil
	})

	for i := 0; i < numSamples; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			eb.PublishSync(NewEvent("latency.test", nil))
			latency := time.Since(start)

			mu.Lock()
			latencies = append(latencies, latency)
			mu.Unlock()
		}()

		if i%100 == 0 {
			time.Sleep(time.Millisecond)
		}
	}

	wg.Wait()

	var total time.Duration
	var min, max time.Duration = latencies[0], latencies[0]

	for _, l := range latencies {
		total += l
		if l < min {
			min = l
		}
		if l > max {
			max = l
		}
	}

	avg := total / time.Duration(len(latencies))

	sortDurations(latencies)
	p50 := latencies[len(latencies)*50/100]
	p95 := latencies[len(latencies)*95/100]
	p99 := latencies[len(latencies)*99/100]

	t.Logf("Latency Test Results (sync publish):")
	t.Logf("  Samples: %d", numSamples)
	t.Logf("  Min: %v", min)
	t.Logf("  Max: %v", max)
	t.Logf("  Avg: %v", avg)
	t.Logf("  P50: %v", p50)
	t.Logf("  P95: %v", p95)
	t.Logf("  P99: %v", p99)

	eb.Shutdown(context.Background())
}

func sortDurations(d []time.Duration) {
	for i := 0; i < len(d)-1; i++ {
		for j := i + 1; j < len(d); j++ {
			if d[j] < d[i] {
				d[i], d[j] = d[j], d[i]
			}
		}
	}
}
