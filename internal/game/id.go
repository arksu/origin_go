package game

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/persistence"
)

type EntityIDManager struct {
	db        *persistence.Postgres
	logger    *zap.Logger
	rangeSize uint64

	currentID uint64
	rangeEnd  uint64

	mu sync.Mutex
}

func NewEntityIDManager(cfg *config.Config, db *persistence.Postgres, logger *zap.Logger) *EntityIDManager {
	lastUsedID := uint64(db.GetGlobalVarLong(context.Background(), LAST_USED_ID))
	logger.Info("EntityIDManager loaded lastUsedID from DB", zap.Uint64("last_used_id", lastUsedID))

	rangeSize := uint64(cfg.EntityID.RangeSize)
	if rangeSize <= 10 {
		panic("EntityIDManager: RangeSize must be greater than 10")
	}

	em := &EntityIDManager{
		db:        db,
		logger:    logger,
		rangeSize: rangeSize,
		currentID: lastUsedID,
		rangeEnd:  lastUsedID,
	}

	em.allocateNewRange()

	return em
}

func (em *EntityIDManager) allocateNewRange() {
	newRangeEnd := em.rangeEnd + em.rangeSize
	em.rangeEnd = newRangeEnd

	em.logger.Info("EntityIDManager allocated new range",
		zap.Uint64("start", em.currentID+1),
		zap.Uint64("end", newRangeEnd))

	go func(endID uint64) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := em.db.SetGlobalVarLong(ctx, LAST_USED_ID, int64(endID)); err != nil {
			em.logger.Error("EntityIDManager failed to persist LAST_USED_ID",
				zap.Uint64("end_id", endID),
				zap.Error(err))
		}
	}(newRangeEnd)
}

func (em *EntityIDManager) GetFreeID() ecs.EntityID {
	em.mu.Lock()
	defer em.mu.Unlock()

	em.currentID++

	if em.currentID >= em.rangeEnd {
		em.allocateNewRange()
	}

	return ecs.EntityID(em.currentID)
}

func (em *EntityIDManager) GetLastUsedID() uint64 {
	em.mu.Lock()
	defer em.mu.Unlock()
	return em.currentID
}

func (em *EntityIDManager) Stop() {
	em.mu.Lock()
	currentID := em.currentID
	em.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := em.db.SetGlobalVarLong(ctx, LAST_USED_ID, int64(currentID)); err != nil {
		em.logger.Error("EntityIDManager failed to persist final LAST_USED_ID",
			zap.Uint64("current_id", currentID),
			zap.Error(err))
	} else {
		em.logger.Info("EntityIDManager persisted final LAST_USED_ID", zap.Uint64("current_id", currentID))
	}
}
