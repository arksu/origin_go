package world

import (
	"origin/internal/core"
	"origin/internal/types"
	"sync/atomic"
	"time"
)

func (cm *ChunkManager) scheduleSaveRetry(coord types.ChunkCoord, chunk *core.Chunk) {
	if atomic.LoadInt32(&cm.stopped) != 0 {
		return
	}
	cm.retryMu.Lock()
	defer cm.retryMu.Unlock()
	if retry, exists := cm.saveRetries[coord]; !exists || retry.chunk != chunk {
		cm.saveRetries[coord] = saveRetry{chunk: chunk, due: time.Now().Add(chunkSaveRetryDelay)}
	}
}

func (cm *ChunkManager) saveRetryWorker() {
	defer cm.wg.Done()
	ticker := time.NewTicker(chunkSaveRetryDelay)
	defer ticker.Stop()
	for {
		select {
		case <-cm.stopCh:
			return
		case now := <-ticker.C:
			cm.enqueueSaveRetries(now)
		}
	}
}

func (cm *ChunkManager) enqueueSaveRetries(now time.Time) {
	cm.retryMu.Lock()
	defer cm.retryMu.Unlock()
	for coord, retry := range cm.saveRetries {
		if now.Before(retry.due) {
			continue
		}
		select {
		case cm.saveQueue <- saveRequest{coord: coord, chunk: retry.chunk}:
			delete(cm.saveRetries, coord)
		default:
			retry.due = now.Add(chunkSaveRetryDelay)
			cm.saveRetries[coord] = retry
		}
	}
}
