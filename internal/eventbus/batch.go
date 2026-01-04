package eventbus

import (
	"context"
	"sync"
	"time"
)

type BulkEvent struct {
	topic  string
	Events []Event
}

func (e *BulkEvent) Topic() string {
	return e.topic
}

func NewBulkEvent(topic string, events []Event) *BulkEvent {
	return &BulkEvent{
		topic:  topic,
		Events: events,
	}
}

type BatchPublisher struct {
	eb           *EventBus
	batchSize    int
	flushTimeout time.Duration

	batches  map[string][]Event
	batchMu  sync.Mutex
	priority Priority

	stopCh chan struct{}
	wg     sync.WaitGroup
}

func newBatchPublisher(eb *EventBus, batchSize int, flushTimeout time.Duration) *BatchPublisher {
	bp := &BatchPublisher{
		eb:           eb,
		batchSize:    batchSize,
		flushTimeout: flushTimeout,
		batches:      make(map[string][]Event),
		priority:     PriorityMedium,
		stopCh:       make(chan struct{}),
	}

	bp.wg.Add(1)
	go bp.flushLoop()

	return bp
}

func (bp *BatchPublisher) Add(event Event) {
	bp.batchMu.Lock()
	defer bp.batchMu.Unlock()

	topic := event.Topic()
	bp.batches[topic] = append(bp.batches[topic], event)

	if len(bp.batches[topic]) >= bp.batchSize {
		bp.flushTopicLocked(topic)
	}
}

func (bp *BatchPublisher) AddWithPriority(event Event, priority Priority) {
	bp.batchMu.Lock()
	defer bp.batchMu.Unlock()

	bp.priority = priority
	topic := event.Topic()
	bp.batches[topic] = append(bp.batches[topic], event)

	if len(bp.batches[topic]) >= bp.batchSize {
		bp.flushTopicLocked(topic)
	}
}

func (bp *BatchPublisher) Flush() {
	bp.batchMu.Lock()
	defer bp.batchMu.Unlock()

	for topic := range bp.batches {
		bp.flushTopicLocked(topic)
	}
}

func (bp *BatchPublisher) flushTopicLocked(topic string) {
	events := bp.batches[topic]
	if len(events) == 0 {
		return
	}

	bulkEvent := NewBulkEvent(topic+".bulk", events)
	bp.eb.PublishAsync(bulkEvent, bp.priority)

	bp.batches[topic] = bp.batches[topic][:0]
}

func (bp *BatchPublisher) flushLoop() {
	defer bp.wg.Done()

	ticker := time.NewTicker(bp.flushTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-bp.stopCh:
			bp.Flush()
			return
		case <-ticker.C:
			bp.Flush()
		}
	}
}

func (bp *BatchPublisher) Stop() {
	close(bp.stopCh)
	bp.wg.Wait()
}

func (bp *BatchPublisher) SetBatchSize(size int) {
	bp.batchMu.Lock()
	bp.batchSize = size
	bp.batchMu.Unlock()
}

func (bp *BatchPublisher) SetFlushTimeout(timeout time.Duration) {
	bp.batchMu.Lock()
	bp.flushTimeout = timeout
	bp.batchMu.Unlock()
}

type BulkHandler func(events []Event) error

func (eb *EventBus) SubscribeBulkSync(topic string, priority Priority, handler BulkHandler) SubscriptionToken {
	wrappedHandler := func(ctx context.Context, event Event) error {
		bulk, ok := event.(*BulkEvent)
		if !ok {
			return handler([]Event{event})
		}
		return handler(bulk.Events)
	}
	return eb.SubscribeSync(topic+".bulk", priority, wrappedHandler)
}

func (eb *EventBus) SubscribeBulkAsync(topic string, priority Priority, handler BulkHandler) SubscriptionToken {
	wrappedHandler := func(ctx context.Context, event Event) error {
		bulk, ok := event.(*BulkEvent)
		if !ok {
			return handler([]Event{event})
		}
		return handler(bulk.Events)
	}
	return eb.SubscribeAsync(topic+".bulk", priority, wrappedHandler)
}
