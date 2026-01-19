package systems

import (
	"context"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/types"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	snapshotChannelSize = 1000
	batchSize           = 50
	batchTimeout        = 100 * time.Millisecond
)

type CharacterSaveSystem struct {
	ecs.BaseSystem
	saver        *CharacterSaver
	saveInterval time.Duration
	logger       *zap.Logger
}

func NewCharacterSaveSystem(saver *CharacterSaver, saveInterval time.Duration, logger *zap.Logger) *CharacterSaveSystem {
	return &CharacterSaveSystem{
		BaseSystem:   ecs.NewBaseSystem("CharacterSaveSystem", 500),
		saver:        saver,
		saveInterval: saveInterval,
		logger:       logger,
	}
}

func (s *CharacterSaveSystem) Update(w *ecs.World, dt float64) {
	now := time.Now()
	charEntities := w.CharacterEntities()

	for entityID, charEntity := range charEntities.Map {
		if now.After(charEntity.NextSaveAt) {
			if !w.Alive(charEntity.Handle) {
				charEntities.Remove(entityID)
				s.logger.Warn("Character entity no longer alive, removed from save tracking",
					zap.Uint64("entity_id", uint64(entityID)))
				continue
			}

			s.saver.Save(w, entityID, charEntity.Handle)

			nextSaveAt := now.Add(s.saveInterval)
			charEntities.UpdateSaveTime(entityID, now, nextSaveAt)
		}
	}
}

func (s *CharacterSaveSystem) Stop() {
	s.saver.Stop()
}

type CharacterSnapshot struct {
	CharacterID int64
	X           int
	Y           int
	Heading     int16
	Stamina     int16
	SHP         int16
	HHP         int16
}

type CharacterSaver struct {
	db              *persistence.Postgres
	snapshotChannel chan CharacterSnapshot
	numWorkers      int
	logger          *zap.Logger
	wg              sync.WaitGroup
	ctx             context.Context
	cancel          context.CancelFunc
}

func NewCharacterSaver(db *persistence.Postgres, numWorkers int, logger *zap.Logger) *CharacterSaver {
	ctx, cancel := context.WithCancel(context.Background())

	cs := &CharacterSaver{
		db:              db,
		snapshotChannel: make(chan CharacterSnapshot, snapshotChannelSize),
		numWorkers:      numWorkers,
		logger:          logger,
		ctx:             ctx,
		cancel:          cancel,
	}

	for i := 0; i < numWorkers; i++ {
		cs.wg.Add(1)
		go cs.saveWorker(i)
	}

	return cs
}

func (s *CharacterSaver) Save(w *ecs.World, entityID types.EntityID, handle types.Handle) {
	transform, hasTransform := ecs.GetComponent[components.Transform](w, handle)
	if !hasTransform {
		s.logger.Warn("Character entity missing Transform component",
			zap.Uint64("entity_id", uint64(entityID)))
		return
	}

	snapshot := CharacterSnapshot{
		CharacterID: int64(entityID),
		X:           int(transform.X),
		Y:           int(transform.Y),
		Heading:     int16(transform.Direction),
		Stamina:     100, // TODO
		SHP:         100, // TODO
		HHP:         100, // TODO
	}

	select {
	case s.snapshotChannel <- snapshot:
	default:
		s.logger.Warn("Character save channel full, dropping snapshot",
			zap.Int64("character_id", snapshot.CharacterID),
			zap.Int("channel_size", snapshotChannelSize))
	}
}

func (s *CharacterSaver) saveWorker(workerID int) {
	defer s.wg.Done()

	batch := make([]CharacterSnapshot, 0, batchSize)
	ticker := time.NewTicker(batchTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			if len(batch) > 0 {
				s.flushBatch(batch)
			}
			return

		case snapshot := <-s.snapshotChannel:
			batch = append(batch, snapshot)
			if len(batch) >= batchSize {
				s.flushBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				s.flushBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

func (s *CharacterSaver) flushBatch(batch []CharacterSnapshot) {
	if len(batch) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()

	successCount := 0
	for _, snapshot := range batch {
		params := repository.BatchUpdateCharactersParams{
			ID:      snapshot.CharacterID,
			X:       snapshot.X,
			Y:       snapshot.Y,
			Heading: snapshot.Heading,
			Stamina: int(snapshot.Stamina),
			Shp:     int(snapshot.SHP),
			Hhp:     int(snapshot.HHP),
		}

		err := s.db.Queries().BatchUpdateCharacters(ctx, params)
		if err != nil {
			s.logger.Error("Failed to update character",
				zap.Int64("character_id", snapshot.CharacterID),
				zap.Error(err))
			continue
		}
		successCount++
	}

	s.logger.Debug("Batch character save completed",
		zap.Int("batch_size", len(batch)),
		zap.Int("success_count", successCount))

	if successCount != len(batch) {
		s.logger.Warn("Some character updates failed",
			zap.Int("batch_size", len(batch)),
			zap.Int("success_count", successCount))
	}
}

func (s *CharacterSaver) Stop() {
	s.cancel()
	s.wg.Wait()
	close(s.snapshotChannel)
	s.logger.Info("CharacterSaver stopped")
}
