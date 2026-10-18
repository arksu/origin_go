package world

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

const spotSaveUpdateBudget = 32

type spotStateJob struct {
	store SpotStore
	write SpotStateWrite
	spot  *managedSpot // Only the owning shard accesses this record.

	mu      sync.Mutex
	running bool
	ready   bool
	err     error

	// Owner-only retry state; there is no background retry loop.
	retryAt  time.Time
	backoff  time.Duration
	terminal bool
}

func (j *spotStateJob) Run() error {
	ctx, cancel := context.WithTimeout(context.Background(), spotPersistenceTimeout)
	defer cancel()
	return j.store.SaveSpotState(ctx, j.write)
}

func (j *spotStateJob) Complete(err error) {
	j.mu.Lock()
	j.running, j.ready, j.err = false, true, err
	j.mu.Unlock()
}

// Update applies acknowledgements under the shard lock, then retries unresolved
// snapshots through the existing workers. now is monotonic wall time for I/O
// backoff, never a gameplay deadline or a change to persisted server runtime.
func (m *SpotManager) Update(now time.Time) {
	visits := min(len(m.pending), spotSaveUpdateBudget)
	for range visits {
		if m.nextPending >= len(m.pending) {
			m.nextPending = 0
		}
		job := m.pending[m.nextPending]
		job.mu.Lock()
		ready, running, err := job.ready, job.running, job.err
		job.ready = false
		job.mu.Unlock()
		if ready && err == nil {
			// Another object transaction may have published a newer revision while
			// this standalone acknowledgement was waiting for the owning shard.
			if revision := job.write.ExpectedRevision + 1; revision > job.spot.state.Revision {
				job.spot.state = SpotStateSnapshot{
					State: job.write.State, Revision: revision,
					LastRuntimeSeconds: job.write.RuntimeSeconds,
				}
			}
			job.spot.pending = nil
			last := len(m.pending) - 1
			m.pending[m.nextPending] = m.pending[last]
			m.pending[last] = nil
			m.pending = m.pending[:last]
			continue
		}
		if ready && err != nil {
			if errors.Is(err, ErrSpotRevisionConflict) {
				job.terminal = true
				job.spot.saveErr = fmt.Errorf("save spot %d: %w", job.write.ID, err)
				if m.err == nil {
					m.err = job.spot.saveErr
				}
				m.logger.Error("Spot state revision conflict", zap.Int64("spot_id", job.write.ID), zap.Error(err))
			} else {
				if job.backoff == 0 {
					job.backoff = 100 * time.Millisecond
				} else {
					job.backoff = min(2*job.backoff, 5*time.Second)
				}
				job.retryAt = now.Add(job.backoff)
				m.logger.Warn("Spot state save failed; retaining snapshot", zap.Int64("spot_id", job.write.ID), zap.Error(err))
			}
		}
		if !running && !job.terminal && !now.Before(job.retryAt) {
			job.mu.Lock()
			job.running = true
			job.mu.Unlock()
			if !m.submit(job) {
				job.mu.Lock()
				job.running = false
				job.mu.Unlock()
			}
		}
		m.nextPending++
	}
}
