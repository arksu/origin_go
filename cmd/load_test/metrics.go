package main

import (
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

type Metrics struct {
	mu sync.Mutex

	loginAttempts  atomic.Int64
	loginSuccesses atomic.Int64
	loginFailures  atomic.Int64
	loginLatencies []time.Duration

	enterWorldAttempts  atomic.Int64
	enterWorldSuccesses atomic.Int64
	enterWorldFailures  atomic.Int64
	enterWorldLatencies []time.Duration

	movesSent     atomic.Int64
	movesReceived atomic.Int64

	packetsReceived atomic.Int64
	packetsSent     atomic.Int64

	errors atomic.Int64

	startTime time.Time
}

func NewMetrics() *Metrics {
	return &Metrics{
		startTime:           time.Now(),
		loginLatencies:      make([]time.Duration, 0, 1000),
		enterWorldLatencies: make([]time.Duration, 0, 1000),
	}
}

func (m *Metrics) RecordLoginAttempt() {
	m.loginAttempts.Add(1)
}

func (m *Metrics) RecordLoginSuccess(latency time.Duration) {
	m.loginSuccesses.Add(1)
	m.mu.Lock()
	m.loginLatencies = append(m.loginLatencies, latency)
	m.mu.Unlock()
}

func (m *Metrics) RecordLoginFailure() {
	m.loginFailures.Add(1)
}

func (m *Metrics) RecordEnterWorldAttempt() {
	m.enterWorldAttempts.Add(1)
}

func (m *Metrics) RecordEnterWorldSuccess(latency time.Duration) {
	m.enterWorldSuccesses.Add(1)
	m.mu.Lock()
	m.enterWorldLatencies = append(m.enterWorldLatencies, latency)
	m.mu.Unlock()
}

func (m *Metrics) RecordEnterWorldFailure() {
	m.enterWorldFailures.Add(1)
}

func (m *Metrics) RecordMoveSent() {
	m.movesSent.Add(1)
}

func (m *Metrics) RecordMoveReceived() {
	m.movesReceived.Add(1)
}

func (m *Metrics) RecordPacketReceived() {
	m.packetsReceived.Add(1)
}

func (m *Metrics) RecordPacketSent() {
	m.packetsSent.Add(1)
}

func (m *Metrics) RecordError() {
	m.errors.Add(1)
}

func (m *Metrics) PrintSummary(logger *zap.Logger) {
	duration := time.Since(m.startTime)

	m.mu.Lock()
	loginP50, loginP95, loginP99 := calculatePercentiles(m.loginLatencies)
	enterP50, enterP95, enterP99 := calculatePercentiles(m.enterWorldLatencies)
	m.mu.Unlock()

	logger.Info("=== Load Test Summary ===")
	logger.Info("Duration", zap.Duration("total", duration))

	logger.Info("Login",
		zap.Int64("attempts", m.loginAttempts.Load()),
		zap.Int64("successes", m.loginSuccesses.Load()),
		zap.Int64("failures", m.loginFailures.Load()),
		zap.Duration("p50", loginP50),
		zap.Duration("p95", loginP95),
		zap.Duration("p99", loginP99),
	)

	logger.Info("EnterWorld",
		zap.Int64("attempts", m.enterWorldAttempts.Load()),
		zap.Int64("successes", m.enterWorldSuccesses.Load()),
		zap.Int64("failures", m.enterWorldFailures.Load()),
		zap.Duration("p50", enterP50),
		zap.Duration("p95", enterP95),
		zap.Duration("p99", enterP99),
	)

	logger.Info("Movement",
		zap.Int64("moves_sent", m.movesSent.Load()),
		zap.Int64("moves_received", m.movesReceived.Load()),
	)

	logger.Info("Packets",
		zap.Int64("sent", m.packetsSent.Load()),
		zap.Int64("received", m.packetsReceived.Load()),
	)

	logger.Info("Errors", zap.Int64("total", m.errors.Load()))
}

func calculatePercentiles(latencies []time.Duration) (p50, p95, p99 time.Duration) {
	if len(latencies) == 0 {
		return 0, 0, 0
	}

	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	p50 = sorted[len(sorted)*50/100]
	p95 = sorted[len(sorted)*95/100]
	if len(sorted) > 0 {
		p99Idx := len(sorted) * 99 / 100
		if p99Idx >= len(sorted) {
			p99Idx = len(sorted) - 1
		}
		p99 = sorted[p99Idx]
	}

	return p50, p95, p99
}
