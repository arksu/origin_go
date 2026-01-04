package eventbus

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

type Priority int

const (
	PriorityLow Priority = iota
	PriorityMedium
	PriorityHigh
)

const (
	highPriorityBuffer   = 1000
	mediumPriorityBuffer = 5000
	lowPriorityBuffer    = 10000

	defaultSyncTimeout  = 100 * time.Millisecond
	defaultAsyncTimeout = 5 * time.Second
	slowHandlerWarn     = 10 * time.Millisecond

	defaultBatchSize    = 100
	defaultBatchTimeout = 100 * time.Millisecond
)

type Event interface {
	Topic() string
}

type BaseEvent struct {
	topic     string
	Timestamp time.Time
	Payload   any
}

func (e *BaseEvent) Topic() string {
	return e.topic
}

func NewEvent(topic string, payload any) *BaseEvent {
	return &BaseEvent{
		topic:     topic,
		Timestamp: time.Now(),
		Payload:   payload,
	}
}

type Handler func(ctx context.Context, event Event) error

type ErrorCallback func(event Event, handlerID string, err error)

type handlerEntry struct {
	id          string
	priority    Priority
	handler     Handler
	timeout     time.Duration
	workerCount int
}

type subscription struct {
	id      uint64
	topic   string
	handler *handlerEntry
	isSync  bool
}

type message struct {
	event    Event
	priority Priority
}

type EventBus struct {
	logger *zap.Logger

	syncHandlers  map[string][]*handlerEntry
	asyncHandlers map[string][]*handlerEntry
	handlersMu    sync.RWMutex

	highPriorityCh   chan *message
	mediumPriorityCh chan *message
	lowPriorityCh    chan *message

	overflowQueue []*message
	overflowMu    sync.Mutex

	deadLetterQueue []*deadLetterEntry
	deadLetterMu    sync.Mutex

	workers      []*worker
	workersMu    sync.Mutex
	minWorkers   int
	maxWorkers   int
	activeWorker int32

	subscriptionID uint64
	subscriptions  map[uint64]*subscription
	subsMu         sync.RWMutex

	eventPool   sync.Pool
	messagePool sync.Pool

	batchPublisher *BatchPublisher

	onError ErrorCallback

	metrics *metrics

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	shutdownOnce sync.Once
	isShutdown   atomic.Bool
}

type deadLetterEntry struct {
	Event     Event
	HandlerID string
	Error     error
	Timestamp time.Time
}

type Config struct {
	MinWorkers      int
	MaxWorkers      int
	Logger          *zap.Logger
	OnError         ErrorCallback
	MetricsRegistry prometheus.Registerer
}

func DefaultConfig() *Config {
	return &Config{
		MinWorkers: runtime.NumCPU(),
		MaxWorkers: runtime.NumCPU() * 4,
	}
}

func New(cfg *Config) *EventBus {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if cfg.MinWorkers <= 0 {
		cfg.MinWorkers = runtime.NumCPU()
	}
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = runtime.NumCPU() * 4
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}

	ctx, cancel := context.WithCancel(context.Background())

	eb := &EventBus{
		logger:           cfg.Logger,
		syncHandlers:     make(map[string][]*handlerEntry),
		asyncHandlers:    make(map[string][]*handlerEntry),
		highPriorityCh:   make(chan *message, highPriorityBuffer),
		mediumPriorityCh: make(chan *message, mediumPriorityBuffer),
		lowPriorityCh:    make(chan *message, lowPriorityBuffer),
		overflowQueue:    make([]*message, 0, 1000),
		deadLetterQueue:  make([]*deadLetterEntry, 0, 100),
		minWorkers:       cfg.MinWorkers,
		maxWorkers:       cfg.MaxWorkers,
		subscriptions:    make(map[uint64]*subscription),
		onError:          cfg.OnError,
		ctx:              ctx,
		cancel:           cancel,
	}

	eb.eventPool = sync.Pool{
		New: func() any {
			return &BaseEvent{}
		},
	}
	eb.messagePool = sync.Pool{
		New: func() any {
			return &message{}
		},
	}

	eb.metrics = newMetrics(cfg.MetricsRegistry)
	eb.batchPublisher = newBatchPublisher(eb, defaultBatchSize, defaultBatchTimeout)

	eb.startWorkers(cfg.MinWorkers)

	return eb
}

func (eb *EventBus) startWorkers(count int) {
	eb.workersMu.Lock()
	defer eb.workersMu.Unlock()

	for i := 0; i < count; i++ {
		w := &worker{
			eb:     eb,
			stopCh: make(chan struct{}),
		}
		eb.workers = append(eb.workers, w)
		eb.wg.Add(1)
		go w.run()
	}
}

func (eb *EventBus) scaleWorkers() {
	queueDepth := len(eb.highPriorityCh) + len(eb.mediumPriorityCh) + len(eb.lowPriorityCh)
	activeWorkers := int(atomic.LoadInt32(&eb.activeWorker))
	currentWorkers := len(eb.workers)

	if queueDepth > currentWorkers*100 && currentWorkers < eb.maxWorkers {
		toAdd := min((queueDepth/100)-currentWorkers, eb.maxWorkers-currentWorkers)
		if toAdd > 0 {
			eb.startWorkers(toAdd)
			eb.logger.Debug("scaled up workers", zap.Int("added", toAdd), zap.Int("total", len(eb.workers)))
		}
	} else if queueDepth < currentWorkers*10 && currentWorkers > eb.minWorkers && activeWorkers < currentWorkers/2 {
		eb.workersMu.Lock()
		toRemove := min(currentWorkers-eb.minWorkers, currentWorkers/4)
		for i := 0; i < toRemove && len(eb.workers) > eb.minWorkers; i++ {
			w := eb.workers[len(eb.workers)-1]
			eb.workers = eb.workers[:len(eb.workers)-1]
			close(w.stopCh)
		}
		eb.workersMu.Unlock()
		if toRemove > 0 {
			eb.logger.Debug("scaled down workers", zap.Int("removed", toRemove), zap.Int("total", len(eb.workers)))
		}
	}
}

type SubscriptionToken uint64

func (eb *EventBus) SubscribeSync(topic string, priority Priority, handler Handler) SubscriptionToken {
	return eb.subscribe(topic, priority, handler, defaultSyncTimeout, 0, true)
}

func (eb *EventBus) SubscribeSyncWithTimeout(topic string, priority Priority, handler Handler, timeout time.Duration) SubscriptionToken {
	return eb.subscribe(topic, priority, handler, timeout, 0, true)
}

func (eb *EventBus) SubscribeAsync(topic string, priority Priority, handler Handler) SubscriptionToken {
	return eb.subscribe(topic, priority, handler, defaultAsyncTimeout, 1, false)
}

func (eb *EventBus) SubscribeAsyncWithWorkers(topic string, priority Priority, handler Handler, workerCount int) SubscriptionToken {
	return eb.subscribe(topic, priority, handler, defaultAsyncTimeout, workerCount, false)
}

func (eb *EventBus) subscribe(topic string, priority Priority, handler Handler, timeout time.Duration, workerCount int, isSync bool) SubscriptionToken {
	id := atomic.AddUint64(&eb.subscriptionID, 1)
	handlerID := generateHandlerID(topic, id)

	entry := &handlerEntry{
		id:          handlerID,
		priority:    priority,
		handler:     handler,
		timeout:     timeout,
		workerCount: workerCount,
	}

	sub := &subscription{
		id:      id,
		topic:   topic,
		handler: entry,
		isSync:  isSync,
	}

	eb.handlersMu.Lock()
	if isSync {
		eb.syncHandlers[topic] = insertSorted(eb.syncHandlers[topic], entry)
	} else {
		eb.asyncHandlers[topic] = insertSorted(eb.asyncHandlers[topic], entry)
	}
	eb.handlersMu.Unlock()

	eb.subsMu.Lock()
	eb.subscriptions[id] = sub
	eb.subsMu.Unlock()

	eb.metrics.subscriptions.WithLabelValues(topic).Inc()

	return SubscriptionToken(id)
}

func (eb *EventBus) Unsubscribe(token SubscriptionToken) {
	eb.subsMu.Lock()
	sub, ok := eb.subscriptions[uint64(token)]
	if !ok {
		eb.subsMu.Unlock()
		return
	}
	delete(eb.subscriptions, uint64(token))
	eb.subsMu.Unlock()

	eb.handlersMu.Lock()
	if sub.isSync {
		eb.syncHandlers[sub.topic] = removeHandler(eb.syncHandlers[sub.topic], sub.handler.id)
	} else {
		eb.asyncHandlers[sub.topic] = removeHandler(eb.asyncHandlers[sub.topic], sub.handler.id)
	}
	eb.handlersMu.Unlock()

	eb.metrics.subscriptions.WithLabelValues(sub.topic).Dec()
}

func (eb *EventBus) PublishSync(event Event) error {
	if eb.isShutdown.Load() {
		return ErrShutdown
	}

	start := time.Now()
	topic := event.Topic()

	handlers := eb.getMatchingHandlers(topic, true)
	if len(handlers) == 0 {
		return nil
	}

	var errs []error
	for _, h := range handlers {
		ctx, cancel := context.WithTimeout(eb.ctx, h.timeout)
		handlerStart := time.Now()

		err := eb.safeCall(ctx, h, event)

		elapsed := time.Since(handlerStart)
		eb.metrics.handlerDuration.WithLabelValues(topic, h.id).Observe(elapsed.Seconds())

		if elapsed > slowHandlerWarn {
			eb.logger.Warn("slow sync handler",
				zap.String("topic", topic),
				zap.String("handler", h.id),
				zap.Duration("duration", elapsed),
			)
		}

		cancel()

		if err != nil {
			errs = append(errs, err)
			eb.handleError(event, h.id, err)
		}
	}

	eb.metrics.eventsPublished.WithLabelValues(topic, "sync").Inc()
	eb.metrics.publishDuration.WithLabelValues(topic, "sync").Observe(time.Since(start).Seconds())

	if len(errs) > 0 {
		return &MultiError{Errors: errs}
	}
	return nil
}

func (eb *EventBus) PublishAsync(event Event, priority Priority) {
	if eb.isShutdown.Load() {
		return
	}

	msg := eb.acquireMessage()
	msg.event = event
	msg.priority = priority

	var ch chan *message
	switch priority {
	case PriorityHigh:
		ch = eb.highPriorityCh
	case PriorityMedium:
		ch = eb.mediumPriorityCh
	default:
		ch = eb.lowPriorityCh
	}

	select {
	case ch <- msg:
		eb.metrics.eventsPublished.WithLabelValues(event.Topic(), "async").Inc()
	default:
		eb.overflowMu.Lock()
		eb.overflowQueue = append(eb.overflowQueue, msg)
		eb.overflowMu.Unlock()
		eb.metrics.queueOverflow.WithLabelValues(event.Topic()).Inc()
		eb.logger.Warn("event queue overflow, added to overflow queue",
			zap.String("topic", event.Topic()),
			zap.Int("priority", int(priority)),
		)
	}

	eb.scaleWorkers()
}

func (eb *EventBus) getMatchingHandlers(topic string, sync bool) []*handlerEntry {
	eb.handlersMu.RLock()
	defer eb.handlersMu.RUnlock()

	var handlers map[string][]*handlerEntry
	if sync {
		handlers = eb.syncHandlers
	} else {
		handlers = eb.asyncHandlers
	}

	var result []*handlerEntry

	if h, ok := handlers[topic]; ok {
		result = append(result, h...)
	}

	parts := strings.Split(topic, ".")
	for i := len(parts) - 1; i >= 0; i-- {
		wildcard := strings.Join(parts[:i], ".") + ".*"
		if h, ok := handlers[wildcard]; ok {
			result = append(result, h...)
		}
	}

	if h, ok := handlers["*"]; ok {
		result = append(result, h...)
	}

	return sortByPriority(result)
}

func (eb *EventBus) safeCall(ctx context.Context, h *handlerEntry, event Event) (err error) {
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- &PanicError{Recovered: r, Stack: getStack()}
				eb.logger.Error("handler panic recovered",
					zap.String("handler", h.id),
					zap.Any("panic", r),
					zap.String("stack", string(getStack())),
				)
			}
		}()
		done <- h.handler(ctx, event)
	}()

	select {
	case err = <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (eb *EventBus) handleError(event Event, handlerID string, err error) {
	eb.metrics.handlerErrors.WithLabelValues(event.Topic(), handlerID).Inc()

	entry := &deadLetterEntry{
		Event:     event,
		HandlerID: handlerID,
		Error:     err,
		Timestamp: time.Now(),
	}

	eb.deadLetterMu.Lock()
	eb.deadLetterQueue = append(eb.deadLetterQueue, entry)
	eb.deadLetterMu.Unlock()

	eb.logger.Error("handler error",
		zap.String("topic", event.Topic()),
		zap.String("handler", handlerID),
		zap.Error(err),
	)

	if eb.onError != nil {
		eb.onError(event, handlerID, err)
	}
}

func (eb *EventBus) BatchPublisher() *BatchPublisher {
	return eb.batchPublisher
}

func (eb *EventBus) QueueDepth() (high, medium, low int) {
	return len(eb.highPriorityCh), len(eb.mediumPriorityCh), len(eb.lowPriorityCh)
}

func (eb *EventBus) DeadLetterQueue() []*deadLetterEntry {
	eb.deadLetterMu.Lock()
	defer eb.deadLetterMu.Unlock()
	result := make([]*deadLetterEntry, len(eb.deadLetterQueue))
	copy(result, eb.deadLetterQueue)
	return result
}

func (eb *EventBus) ClearDeadLetterQueue() {
	eb.deadLetterMu.Lock()
	eb.deadLetterQueue = eb.deadLetterQueue[:0]
	eb.deadLetterMu.Unlock()
}

func (eb *EventBus) Shutdown(ctx context.Context) error {
	var err error
	eb.shutdownOnce.Do(func() {
		eb.isShutdown.Store(true)
		eb.logger.Info("shutting down event bus")

		eb.batchPublisher.Stop()

		eb.drainQueues(ctx)

		eb.cancel()

		done := make(chan struct{})
		go func() {
			eb.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			eb.logger.Info("event bus shutdown complete")
		case <-ctx.Done():
			err = ctx.Err()
			eb.logger.Warn("event bus shutdown timeout", zap.Error(err))
		}
	})
	return err
}

func (eb *EventBus) drainQueues(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-eb.highPriorityCh:
			eb.processMessage(msg)
		default:
			goto drainMedium
		}
	}
drainMedium:
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-eb.mediumPriorityCh:
			eb.processMessage(msg)
		default:
			goto drainLow
		}
	}
drainLow:
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-eb.lowPriorityCh:
			eb.processMessage(msg)
		default:
			return
		}
	}
}

func (eb *EventBus) processMessage(msg *message) {
	handlers := eb.getMatchingHandlers(msg.event.Topic(), false)
	for _, h := range handlers {
		ctx, cancel := context.WithTimeout(eb.ctx, h.timeout)
		start := time.Now()

		err := eb.safeCall(ctx, h, msg.event)

		eb.metrics.handlerDuration.WithLabelValues(msg.event.Topic(), h.id).Observe(time.Since(start).Seconds())
		cancel()

		if err != nil {
			eb.handleError(msg.event, h.id, err)
		}
	}
	eb.releaseMessage(msg)
}

func (eb *EventBus) acquireEvent() *BaseEvent {
	return eb.eventPool.Get().(*BaseEvent)
}

func (eb *EventBus) releaseEvent(e *BaseEvent) {
	e.topic = ""
	e.Payload = nil
	eb.eventPool.Put(e)
}

func (eb *EventBus) acquireMessage() *message {
	return eb.messagePool.Get().(*message)
}

func (eb *EventBus) releaseMessage(m *message) {
	m.event = nil
	m.priority = 0
	eb.messagePool.Put(m)
}

func (eb *EventBus) AcquireEvent(topic string, payload any) *BaseEvent {
	e := eb.acquireEvent()
	e.topic = topic
	e.Timestamp = time.Now()
	e.Payload = payload
	return e
}

func (eb *EventBus) ReleaseEvent(e *BaseEvent) {
	eb.releaseEvent(e)
}

type worker struct {
	eb     *EventBus
	stopCh chan struct{}
}

func (w *worker) run() {
	defer w.eb.wg.Done()

	for {
		select {
		case <-w.stopCh:
			return
		case <-w.eb.ctx.Done():
			return
		default:
		}

		atomic.AddInt32(&w.eb.activeWorker, 1)

		var msg *message
		select {
		case <-w.stopCh:
			atomic.AddInt32(&w.eb.activeWorker, -1)
			return
		case <-w.eb.ctx.Done():
			atomic.AddInt32(&w.eb.activeWorker, -1)
			return
		case msg = <-w.eb.highPriorityCh:
		case msg = <-w.eb.mediumPriorityCh:
		case msg = <-w.eb.lowPriorityCh:
		}

		if msg != nil {
			w.eb.processMessage(msg)
		}

		atomic.AddInt32(&w.eb.activeWorker, -1)

		w.eb.overflowMu.Lock()
		if len(w.eb.overflowQueue) > 0 {
			msg = w.eb.overflowQueue[0]
			w.eb.overflowQueue = w.eb.overflowQueue[1:]
			w.eb.overflowMu.Unlock()
			w.eb.processMessage(msg)
		} else {
			w.eb.overflowMu.Unlock()
		}
	}
}

func insertSorted(handlers []*handlerEntry, entry *handlerEntry) []*handlerEntry {
	handlers = append(handlers, entry)
	for i := len(handlers) - 1; i > 0; i-- {
		if handlers[i].priority > handlers[i-1].priority {
			handlers[i], handlers[i-1] = handlers[i-1], handlers[i]
		} else {
			break
		}
	}
	return handlers
}

func removeHandler(handlers []*handlerEntry, id string) []*handlerEntry {
	for i, h := range handlers {
		if h.id == id {
			return append(handlers[:i], handlers[i+1:]...)
		}
	}
	return handlers
}

func sortByPriority(handlers []*handlerEntry) []*handlerEntry {
	n := len(handlers)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if handlers[j].priority < handlers[j+1].priority {
				handlers[j], handlers[j+1] = handlers[j+1], handlers[j]
			}
		}
	}
	return handlers
}

func generateHandlerID(topic string, id uint64) string {
	return topic + "_" + uitoa(id)
}

func uitoa(i uint64) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	return string(b[n:])
}

func getStack() []byte {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return buf[:n]
}

type metrics struct {
	eventsPublished *prometheus.CounterVec
	publishDuration *prometheus.HistogramVec
	handlerDuration *prometheus.HistogramVec
	handlerErrors   *prometheus.CounterVec
	queueDepth      *prometheus.GaugeVec
	queueOverflow   *prometheus.CounterVec
	subscriptions   *prometheus.GaugeVec
	workerCount     prometheus.Gauge
}

func newMetrics(reg prometheus.Registerer) *metrics {
	if reg == nil {
		reg = prometheus.NewRegistry()
	}

	factory := promauto.With(reg)

	return &metrics{
		eventsPublished: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "eventbus_events_published_total",
			Help: "Total number of events published",
		}, []string{"topic", "type"}),
		publishDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "eventbus_publish_duration_seconds",
			Help:    "Duration of event publishing",
			Buckets: prometheus.DefBuckets,
		}, []string{"topic", "type"}),
		handlerDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "eventbus_handler_duration_seconds",
			Help:    "Duration of handler execution",
			Buckets: prometheus.DefBuckets,
		}, []string{"topic", "handler"}),
		handlerErrors: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "eventbus_handler_errors_total",
			Help: "Total number of handler errors",
		}, []string{"topic", "handler"}),
		queueDepth: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "eventbus_queue_depth",
			Help: "Current depth of event queues",
		}, []string{"priority"}),
		queueOverflow: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "eventbus_queue_overflow_total",
			Help: "Total number of events that overflowed the queue",
		}, []string{"topic"}),
		subscriptions: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "eventbus_subscriptions",
			Help: "Current number of subscriptions",
		}, []string{"topic"}),
		workerCount: factory.NewGauge(prometheus.GaugeOpts{
			Name: "eventbus_worker_count",
			Help: "Current number of workers",
		}),
	}
}
