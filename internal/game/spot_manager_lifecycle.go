package game

import (
	"errors"
	"time"

	"go.uber.org/zap"
)

const spotSaveShutdownTimeout = 30 * time.Second

var errSpotPersistenceShutdown = errors.New("spot persistence: shutdown has pending saves")

// Finish accepted saves before ChunkManager stops the workers that own them.
// Only manager updates hold the shard lock; waiting for I/O never does.
func (s *Shard) drainSpotSaves() {
	s.drainSpotSavesFor(spotSaveShutdownTimeout)
}

func (s *Shard) drainSpotSavesFor(timeout time.Duration) {
	if s.spotManager == nil {
		return
	}
	deadline := time.Now().Add(timeout)
	for {
		s.mu.Lock()
		s.spotManager.Update(time.Now())
		pending := s.spotManager.PendingCount()
		err := s.spotManager.Err()
		s.mu.Unlock()
		if pending == 0 {
			return
		}
		// A conflicted spot must not prevent acknowledgement of other accepted
		// writes before their shared workers are stopped.
		if !time.Now().Before(deadline) {
			if err == nil {
				err = errSpotPersistenceShutdown
			}
			s.logger.Error("Shutdown spot persistence incomplete", zap.Int("pending", pending), zap.Error(err))
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
