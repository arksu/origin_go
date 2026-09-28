package world

import (
	"time"

	"origin/internal/types"
)

const (
	activationRetryInitialDelay = time.Second
	activationRetryMaxDelay     = time.Minute
)

type activationRetry struct {
	due   time.Time
	delay time.Duration
}

// hasDueActivationRetryLocked requires readyMu.
func (cm *ChunkManager) hasDueActivationRetryLocked(now time.Time) bool {
	for _, retry := range cm.activationRetries {
		if !now.Before(retry.due) {
			return true
		}
	}
	return false
}

func (cm *ChunkManager) activationRetryDue(coord types.ChunkCoord, now time.Time) bool {
	cm.readyMu.Lock()
	retry, exists := cm.activationRetries[coord]
	cm.readyMu.Unlock()
	return !exists || !now.Before(retry.due)
}

func (cm *ChunkManager) scheduleActivationRetry(coord types.ChunkCoord, now time.Time) {
	cm.readyMu.Lock()
	defer cm.readyMu.Unlock()
	delay := activationRetryInitialDelay
	if previous, exists := cm.activationRetries[coord]; exists {
		delay = previous.delay * 2
		if delay > activationRetryMaxDelay {
			delay = activationRetryMaxDelay
		}
	}
	cm.activationRetries[coord] = activationRetry{due: now.Add(delay), delay: delay}
}

func (cm *ChunkManager) clearActivationRetry(coord types.ChunkCoord) {
	cm.readyMu.Lock()
	delete(cm.activationRetries, coord)
	cm.readyMu.Unlock()
}
