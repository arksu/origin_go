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
	bytesReceived   atomic.Int64
	bytesSent       atomic.Int64

	errors atomic.Int64

	// Diagnostic counters (aggregated, cheap to read every interval)
	readWaitNs       atomic.Int64
	readWaitSamples  atomic.Int64
	unmarshalNs      atomic.Int64
	unmarshalSamples atomic.Int64
	handleNs         atomic.Int64
	handleSamples    atomic.Int64
	sendMarshalNs    atomic.Int64
	sendMarshalCount atomic.Int64
	sendWriteNs      atomic.Int64
	sendWriteCount   atomic.Int64

	msgAuthResult   atomic.Int64
	msgEnterWorld   atomic.Int64
	msgObjectSpawn  atomic.Int64
	msgObjectMove   atomic.Int64
	msgServerError  atomic.Int64
	msgOther        atomic.Int64
	msgUnmarshalErr atomic.Int64

	startTime time.Time
}

type MetricsSnapshot struct {
	PacketsSent     int64
	PacketsReceived int64
	BytesSent       int64
	BytesReceived   int64

	ReadWaitNs       int64
	ReadWaitSamples  int64
	UnmarshalNs      int64
	UnmarshalSamples int64
	HandleNs         int64
	HandleSamples    int64
	SendMarshalNs    int64
	SendMarshalCount int64
	SendWriteNs      int64
	SendWriteCount   int64

	MsgAuthResult   int64
	MsgEnterWorld   int64
	MsgObjectSpawn  int64
	MsgObjectMove   int64
	MsgServerError  int64
	MsgOther        int64
	MsgUnmarshalErr int64
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

func (m *Metrics) RecordPacketReceivedN(n int64) {
	if n > 0 {
		m.packetsReceived.Add(n)
	}
}

func (m *Metrics) RecordPacketSent() {
	m.packetsSent.Add(1)
}

func (m *Metrics) RecordBytesReceived(n int) {
	if n > 0 {
		m.bytesReceived.Add(int64(n))
	}
}

func (m *Metrics) RecordBytesReceivedN(n int64) {
	if n > 0 {
		m.bytesReceived.Add(n)
	}
}

func (m *Metrics) RecordBytesSent(n int) {
	if n > 0 {
		m.bytesSent.Add(int64(n))
	}
}

func (m *Metrics) RecordReadWait(d time.Duration) {
	m.readWaitNs.Add(d.Nanoseconds())
	m.readWaitSamples.Add(1)
}

func (m *Metrics) RecordUnmarshal(d time.Duration) {
	m.unmarshalNs.Add(d.Nanoseconds())
	m.unmarshalSamples.Add(1)
}

func (m *Metrics) RecordHandle(d time.Duration) {
	m.handleNs.Add(d.Nanoseconds())
	m.handleSamples.Add(1)
}

func (m *Metrics) RecordSendMarshal(d time.Duration) {
	m.sendMarshalNs.Add(d.Nanoseconds())
	m.sendMarshalCount.Add(1)
}

func (m *Metrics) RecordSendWrite(d time.Duration) {
	m.sendWriteNs.Add(d.Nanoseconds())
	m.sendWriteCount.Add(1)
}

func (m *Metrics) RecordMsgAuthResult()  { m.msgAuthResult.Add(1) }
func (m *Metrics) RecordMsgEnterWorld()  { m.msgEnterWorld.Add(1) }
func (m *Metrics) RecordMsgObjectSpawn() { m.msgObjectSpawn.Add(1) }
func (m *Metrics) RecordMsgObjectMove()  { m.msgObjectMove.Add(1) }
func (m *Metrics) RecordMsgServerError() { m.msgServerError.Add(1) }
func (m *Metrics) RecordMsgOther()       { m.msgOther.Add(1) }
func (m *Metrics) RecordMsgUnmarshalErr() {
	m.msgUnmarshalErr.Add(1)
}

func (m *Metrics) RecordError() {
	m.errors.Add(1)
}

func (m *Metrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		PacketsSent:     m.packetsSent.Load(),
		PacketsReceived: m.packetsReceived.Load(),
		BytesSent:       m.bytesSent.Load(),
		BytesReceived:   m.bytesReceived.Load(),

		ReadWaitNs:       m.readWaitNs.Load(),
		ReadWaitSamples:  m.readWaitSamples.Load(),
		UnmarshalNs:      m.unmarshalNs.Load(),
		UnmarshalSamples: m.unmarshalSamples.Load(),
		HandleNs:         m.handleNs.Load(),
		HandleSamples:    m.handleSamples.Load(),
		SendMarshalNs:    m.sendMarshalNs.Load(),
		SendMarshalCount: m.sendMarshalCount.Load(),
		SendWriteNs:      m.sendWriteNs.Load(),
		SendWriteCount:   m.sendWriteCount.Load(),

		MsgAuthResult:   m.msgAuthResult.Load(),
		MsgEnterWorld:   m.msgEnterWorld.Load(),
		MsgObjectSpawn:  m.msgObjectSpawn.Load(),
		MsgObjectMove:   m.msgObjectMove.Load(),
		MsgServerError:  m.msgServerError.Load(),
		MsgOther:        m.msgOther.Load(),
		MsgUnmarshalErr: m.msgUnmarshalErr.Load(),
	}
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
