package world

import (
	"time"

	"origin/internal/core"
	"origin/internal/types"

	"go.uber.org/zap"
)

const (
	deactivationRetryInitialDelay = time.Second
	deactivationRetryMaxDelay     = time.Minute
)

type deactivationRetry struct {
	due   time.Time
	delay time.Duration
}

// hasDueDeactivationRetryLocked requires readyMu.
func (cm *ChunkManager) hasDueDeactivationRetryLocked(now time.Time) bool {
	for _, retry := range cm.deactivationRetries {
		if !now.Before(retry.due) {
			return true
		}
	}
	return false
}

func (cm *ChunkManager) deactivationRetryDue(coord types.ChunkCoord, now time.Time) bool {
	cm.readyMu.Lock()
	defer cm.readyMu.Unlock()
	retry, exists := cm.deactivationRetries[coord]
	return !exists || !now.Before(retry.due)
}

func (cm *ChunkManager) scheduleDeactivationRetry(coord types.ChunkCoord, now time.Time) {
	cm.readyMu.Lock()
	defer cm.readyMu.Unlock()
	delay := deactivationRetryInitialDelay
	if previous, exists := cm.deactivationRetries[coord]; exists {
		delay = previous.delay * 2
		if delay > deactivationRetryMaxDelay {
			delay = deactivationRetryMaxDelay
		}
	}
	cm.deactivationRetries[coord] = deactivationRetry{due: now.Add(delay), delay: delay}
}

func (cm *ChunkManager) clearDeactivationRetry(coord types.ChunkCoord) {
	cm.readyMu.Lock()
	delete(cm.deactivationRetries, coord)
	cm.readyMu.Unlock()
}

func (cm *ChunkManager) deactivateChunkWhenDue(chunk *core.Chunk, now time.Time) bool {
	coord := chunk.Coord
	if !cm.deactivationRetryDue(coord, now) {
		return false
	}
	if err := cm.deactivateChunkInternal(chunk); err != nil {
		cm.scheduleDeactivationRetry(coord, now)
		cm.logger.Error("failed to deactivate chunk",
			zap.Int("chunk_x", coord.X),
			zap.Int("chunk_y", coord.Y),
			zap.Error(err),
		)
		return false
	}
	cm.clearDeactivationRetry(coord)
	return true
}
