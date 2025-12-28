package game

import (
	"context"
	"log"
	"sync"

	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/persistence"
)

type EntityIdManager struct {
	db        *persistence.Postgres
	rangeSize uint64

	currentId uint64 // current ID to allocate
	rangeEnd  uint64 // end of current range (exclusive)

	mu     sync.Mutex
	stopCh chan struct{}
	doneCh chan struct{}
}

func NewEntityIdManager(cfg *config.Config, db *persistence.Postgres) *EntityIdManager {
	lastUsedId := uint64(db.GetGlobalVarLong(context.Background(), LAST_USED_ID))
	log.Printf("EntityIdManager: loaded lastUsedId=%d from DB", lastUsedId)

	rangeSize := uint64(cfg.EntityIdRangeSize)
	if rangeSize == 10 {
		panic("EntityIdManager: EntityIdRangeSize must be greater than 10")
	}

	em := &EntityIdManager{
		db:        db,
		rangeSize: rangeSize,
		currentId: lastUsedId,
		rangeEnd:  lastUsedId, // force immediate range allocation
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}

	// Allocate first range
	em.allocateNewRange()

	return em
}

// allocateNewRange reserves a new range of IDs and persists to DB in background
func (em *EntityIdManager) allocateNewRange() {
	newRangeEnd := em.rangeEnd + em.rangeSize
	em.rangeEnd = newRangeEnd

	log.Printf("EntityIdManager: allocated new range [%d, %d)", em.currentId+1, newRangeEnd)

	// Save to DB in background goroutine
	go func(endId uint64) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*1e9) // 5 seconds
		defer cancel()

		if err := em.db.SetGlobalVarLong(ctx, LAST_USED_ID, int64(endId)); err != nil {
			log.Printf("EntityIdManager: failed to persist LAST_USED_ID=%d: %v", endId, err)
		}
	}(newRangeEnd)
}

// GetFreeId returns the next available entity ID
func (em *EntityIdManager) GetFreeId() ecs.EntityID {
	em.mu.Lock()
	defer em.mu.Unlock()

	em.currentId++

	// Check if we've exhausted the current range
	if em.currentId >= em.rangeEnd {
		em.allocateNewRange()
	}

	return ecs.EntityID(em.currentId)
}

// GetLastUsedId returns the current last used ID
func (em *EntityIdManager) GetLastUsedId() uint64 {
	em.mu.Lock()
	defer em.mu.Unlock()
	return em.currentId
}

// Stop gracefully shuts down the manager and persists final state
func (em *EntityIdManager) Stop() {
	em.mu.Lock()
	currentId := em.currentId
	em.mu.Unlock()

	// Persist final state synchronously
	ctx, cancel := context.WithTimeout(context.Background(), 5*1e9)
	defer cancel()

	if err := em.db.SetGlobalVarLong(ctx, LAST_USED_ID, int64(currentId)); err != nil {
		log.Printf("EntityIdManager: failed to persist final LAST_USED_ID=%d: %v", currentId, err)
	} else {
		log.Printf("EntityIdManager: persisted final LAST_USED_ID=%d", currentId)
	}
}
