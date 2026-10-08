package game

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"origin/internal/const"
	"origin/internal/persistence/repository"
	"sync"
	"time"

	"go.uber.org/zap"

	"origin/internal/config"
	"origin/internal/persistence"
	"origin/internal/types"
)

type EntityIDManager struct {
	db        *persistence.Postgres
	logger    *zap.Logger
	rangeSize uint64

	currentID uint64
	rangeEnd  uint64

	mu sync.Mutex
}

var ErrEntityIDExhausted = errors.New("entity ID space exhausted")

// ReserveIDs reserves a contiguous range without I/O or goroutines. The caller
// must commit its last ID through UpsertGlobalVarLongMax before publishing items.
func (em *EntityIDManager) ReserveIDs(count uint64) (first, last types.EntityID, err error) {
	if count == 0 {
		return 0, 0, nil
	}
	em.mu.Lock()
	defer em.mu.Unlock()
	if count > math.MaxInt64 || em.currentID > math.MaxInt64-count {
		return 0, 0, ErrEntityIDExhausted
	}
	first = types.EntityID(em.currentID + 1)
	em.currentID += count
	if em.currentID > em.rangeEnd {
		em.rangeEnd = em.currentID
	}
	return first, types.EntityID(em.currentID), nil
}

func (em *EntityIDManager) persistHighWatermark(ctx context.Context, id uint64) error {
	return em.db.Queries().UpsertGlobalVarLongMax(ctx, repository.UpsertGlobalVarLongMaxParams{
		Name: _const.LAST_USED_ID, ValueLong: sql.NullInt64{Int64: int64(id), Valid: true},
	})
}

func NewEntityIDManager(cfg *config.Config, db *persistence.Postgres, logger *zap.Logger) *EntityIDManager {
	lastUsedID := uint64(db.GetGlobalVarLong(context.Background(), _const.LAST_USED_ID))
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

		if err := em.persistHighWatermark(ctx, endID); err != nil {
			em.logger.Error("EntityIDManager failed to persist LAST_USED_ID",
				zap.Uint64("end_id", endID),
				zap.Error(err))
		}
	}(newRangeEnd)
}

func (em *EntityIDManager) GetFreeID() types.EntityID {
	em.mu.Lock()
	defer em.mu.Unlock()

	em.currentID++

	if em.currentID >= em.rangeEnd {
		em.allocateNewRange()
	}

	return types.EntityID(em.currentID)
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

	if err := em.persistHighWatermark(ctx, currentID); err != nil {
		em.logger.Error("EntityIDManager failed to persist final LAST_USED_ID",
			zap.Uint64("current_id", currentID),
			zap.Error(err))
	} else {
		em.logger.Info("EntityIDManager persisted final LAST_USED_ID", zap.Uint64("current_id", currentID))
	}
}
