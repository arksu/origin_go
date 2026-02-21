package systems

import (
	"context"
	"encoding/json"
	"math"
	"origin/internal/characterattrs"
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
	batchSize           = 100
	batchTimeout        = 500 * time.Millisecond
)

// InventorySnapshot represents a serialized inventory container for database storage
type InventorySnapshot struct {
	CharacterID  int64
	Kind         int16
	InventoryKey int16
	Data         json.RawMessage
	Version      int
}

// InventorySaverInterface defines the contract for inventory serialization
// This interface breaks circular dependencies between game and systems packages
type InventorySaverInterface interface {
	// SerializeInventories extracts and serializes all inventory containers for a character
	SerializeInventories(world interface{}, characterID types.EntityID, handle types.Handle) []InventorySnapshot
}

type CharacterSaveSystem struct {
	ecs.BaseSystem
	saver        *CharacterSaver
	saveInterval time.Duration
	logger       *zap.Logger
	dueEntityIDs []types.EntityID
}

func NewCharacterSaveSystem(saver *CharacterSaver, saveInterval time.Duration, logger *zap.Logger) *CharacterSaveSystem {
	return &CharacterSaveSystem{
		BaseSystem:   ecs.NewBaseSystem("CharacterSaveSystem", 500),
		saver:        saver,
		saveInterval: saveInterval,
		logger:       logger,
		dueEntityIDs: make([]types.EntityID, 0, 256),
	}
}

func (s *CharacterSaveSystem) Update(w *ecs.World, dt float64) {
	now := ecs.GetResource[ecs.TimeState](w).Now
	charEntities := ecs.GetResource[ecs.CharacterEntities](w)
	s.dueEntityIDs = charEntities.PopDue(now, s.dueEntityIDs[:0])

	for _, entityID := range s.dueEntityIDs {
		charEntity, exists := charEntities.Map[entityID]
		if !exists {
			continue
		}

		if !w.Alive(charEntity.Handle) {
			charEntities.Remove(entityID)
			s.logger.Warn("Character entity no longer alive, removed from save tracking",
				zap.Uint64("entity_id", uint64(entityID)))
			continue
		}

		s.saver.Save(w, entityID, charEntity.Handle)

		// Deterministic jitter based on entityID to spread saves (0-10% of interval)
		jitter := time.Duration(entityID%100) * s.saveInterval / 100
		nextSaveAt := now.Add(s.saveInterval + jitter)
		charEntities.UpdateSaveTime(entityID, now, nextSaveAt)
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
	Stamina     float64
	Energy      float64
	SHP         int16
	HHP         int16
	Attributes  string
	Exp         string
	Skills      string
	Discovery   string
	Inventories []InventorySnapshot
}

type CharacterSaver struct {
	db              *persistence.Postgres
	snapshotChannel chan CharacterSnapshot
	numWorkers      int
	logger          *zap.Logger
	wg              sync.WaitGroup
	ctx             context.Context
	cancel          context.CancelFunc
	inventorySaver  InventorySaverInterface
}

func NewCharacterSaver(db *persistence.Postgres, numWorkers int, inventorySaver InventorySaverInterface, logger *zap.Logger) *CharacterSaver {
	ctx, cancel := context.WithCancel(context.Background())

	cs := &CharacterSaver{
		db:              db,
		snapshotChannel: make(chan CharacterSnapshot, snapshotChannelSize),
		numWorkers:      numWorkers,
		logger:          logger,
		ctx:             ctx,
		cancel:          cancel,
		inventorySaver:  inventorySaver,
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

	attributesRaw, experienceRaw, skillsRaw, discoveryRaw := s.serializeCharacterProfile(w, entityID, handle)
	staminaValue, energyValue, hasStats := s.resolveStatsSnapshotValues(w, entityID, handle)
	if !hasStats {
		return
	}
	inventories := s.inventorySaver.SerializeInventories(w, entityID, handle)
	s.enqueueSnapshot(s.buildSnapshot(entityID, transform, attributesRaw, experienceRaw, skillsRaw, discoveryRaw, staminaValue, energyValue, inventories))
}

// SaveDetached enqueues a lightweight snapshot for detached-entity expiration path.
// Detached players cannot issue commands, so inventory state is expected to be unchanged.
func (s *CharacterSaver) SaveDetached(w *ecs.World, entityID types.EntityID, handle types.Handle) {
	transform, hasTransform := ecs.GetComponent[components.Transform](w, handle)
	if !hasTransform {
		s.logger.Warn("Character entity missing Transform component",
			zap.Uint64("entity_id", uint64(entityID)))
		return
	}

	attributesRaw, experienceRaw, skillsRaw, discoveryRaw := s.serializeCharacterProfile(w, entityID, handle)
	staminaValue, energyValue, hasStats := s.resolveStatsSnapshotValues(w, entityID, handle)
	if !hasStats {
		return
	}
	s.enqueueSnapshot(s.buildSnapshot(entityID, transform, attributesRaw, experienceRaw, skillsRaw, discoveryRaw, staminaValue, energyValue, nil))
}

func (s *CharacterSaver) buildSnapshot(
	entityID types.EntityID,
	transform components.Transform,
	attributesRaw string,
	experienceRaw string,
	skillsRaw string,
	discoveryRaw string,
	staminaValue float64,
	energyValue float64,
	inventories []InventorySnapshot,
) CharacterSnapshot {
	return CharacterSnapshot{
		CharacterID: int64(entityID),
		X:           int(transform.X),
		Y:           int(transform.Y),
		Heading:     normalizeCharacterHeading(transform.Direction),
		Stamina:     staminaValue,
		Energy:      energyValue,
		SHP:         100, // TODO
		HHP:         100, // TODO
		Attributes:  attributesRaw,
		Exp:         experienceRaw,
		Skills:      skillsRaw,
		Discovery:   discoveryRaw,
		Inventories: inventories,
	}
}

func (s *CharacterSaver) resolveStatsSnapshotValues(w *ecs.World, entityID types.EntityID, handle types.Handle) (float64, float64, bool) {
	if stats, hasStats := ecs.GetComponent[components.EntityStats](w, handle); hasStats {
		stamina := stats.Stamina
		if stamina < 0 {
			stamina = 0
		}
		energy := stats.Energy
		if energy < 0 {
			energy = 0
		}
		return stamina, energy, true
	}

	s.logger.Warn("Character entity missing EntityStats component, skip character save snapshot",
		zap.Uint64("entity_id", uint64(entityID)))
	return 0, 0, false
}

func (s *CharacterSaver) serializeCharacterProfile(w *ecs.World, entityID types.EntityID, handle types.Handle) (string, string, string, string) {
	values := characterattrs.Default()
	experience := components.CharacterExperience{}
	skills := []string{}
	discovery := []string{}
	if profile, hasProfile := ecs.GetComponent[components.CharacterProfile](w, handle); hasProfile {
		values = characterattrs.Normalize(profile.Attributes)
		experience = profile.Experience
		skills = profile.Skills
		discovery = profile.Discovery
	} else {
		s.logger.Warn("Character entity missing CharacterProfile component, using defaults",
			zap.Uint64("entity_id", uint64(entityID)))
	}

	attributesRaw, err := characterattrs.Marshal(values)
	if err != nil {
		s.logger.Error("Failed to marshal character attributes, using defaults",
			zap.Uint64("entity_id", uint64(entityID)),
			zap.Error(err))
		defaultRaw, defaultErr := characterattrs.Marshal(characterattrs.Default())
		if defaultErr != nil {
			s.logger.Error("Failed to marshal default character attributes",
				zap.Uint64("entity_id", uint64(entityID)),
				zap.Error(defaultErr))
			attributesRaw = []byte("{}")
		} else {
			attributesRaw = defaultRaw
		}
	}

	experienceRaw, err := components.MarshalCharacterExperience(experience)
	if err != nil {
		s.logger.Error("Failed to marshal character experience, using defaults",
			zap.Uint64("entity_id", uint64(entityID)),
			zap.Error(err))
		experienceRaw = []byte(`{"lp":0,"nature":0,"industry":0,"combat":0}`)
	}

	skillsRaw, err := components.MarshalStringSet(skills)
	if err != nil {
		s.logger.Error("Failed to marshal character skills, using defaults",
			zap.Uint64("entity_id", uint64(entityID)),
			zap.Error(err))
		skillsRaw = []byte("[]")
	}

	discoveryRaw, err := components.MarshalStringSet(discovery)
	if err != nil {
		s.logger.Error("Failed to marshal character discovery, using defaults",
			zap.Uint64("entity_id", uint64(entityID)),
			zap.Error(err))
		discoveryRaw = []byte("[]")
	}

	return string(attributesRaw), string(experienceRaw), string(skillsRaw), string(discoveryRaw)
}

func (s *CharacterSaver) enqueueSnapshot(snapshot CharacterSnapshot) {
	select {
	case s.snapshotChannel <- snapshot:
	default:
		s.logger.Warn("Character save channel full, dropping snapshot",
			zap.Int64("character_id", snapshot.CharacterID),
			zap.Int("channel_size", snapshotChannelSize))
	}
}

func normalizeCharacterHeading(direction float64) int16 {
	// Runtime keeps radians; DB stores integer degrees [0..359].
	if math.IsNaN(direction) || math.IsInf(direction, 0) {
		return 0
	}

	degrees := direction * 180 / math.Pi
	normalized := math.Mod(degrees, 360)
	if normalized < 0 {
		normalized += 360
	}

	return int16(math.Floor(normalized))
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
				// Create a new context for final save to avoid context cancellation
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
				defer cancel()

				s.flushBatchWithContext(ctx, batch)
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
	s.flushBatchWithContext(s.ctx, batch)
}

func (s *CharacterSaver) flushBatchWithContext(ctx context.Context, batch []CharacterSnapshot) {
	if len(batch) == 0 {
		return
	}

	// Convert batch to arrays for batch update
	ids := make([]int, len(batch))
	xs := make([]float64, len(batch))
	ys := make([]float64, len(batch))
	headings := make([]float64, len(batch))
	staminas := make([]float64, len(batch))
	energies := make([]float64, len(batch))
	shps := make([]int, len(batch))
	hhps := make([]int, len(batch))
	attributes := make([]string, len(batch))
	exps := make([]string, len(batch))
	skills := make([]string, len(batch))
	discovery := make([]string, len(batch))

	for i, snapshot := range batch {
		ids[i] = int(snapshot.CharacterID)
		xs[i] = float64(snapshot.X)
		ys[i] = float64(snapshot.Y)
		headings[i] = float64(snapshot.Heading)
		staminas[i] = snapshot.Stamina
		energies[i] = snapshot.Energy
		shps[i] = int(snapshot.SHP)
		hhps[i] = int(snapshot.HHP)
		attributes[i] = snapshot.Attributes
		exps[i] = snapshot.Exp
		skills[i] = snapshot.Skills
		discovery[i] = snapshot.Discovery
	}

	params := repository.UpdateCharactersParams{
		Ids:        ids,
		Xs:         xs,
		Ys:         ys,
		Headings:   headings,
		Staminas:   staminas,
		Energies:   energies,
		Shps:       shps,
		Hhps:       hhps,
		Attributes: attributes,
		Exps:       exps,
		Skills:     skills,
		Discovery:  discovery,
	}

	err := s.db.Queries().UpdateCharacters(ctx, params)
	if err != nil {
		s.logger.Error("Failed to execute batch update",
			zap.Int("batch_size", len(batch)),
			zap.Any("params", params),
			zap.Error(err))
		return
	}

	// Batch upsert all inventories in a single query
	totalInv := 0
	for _, snapshot := range batch {
		totalInv += len(snapshot.Inventories)
	}

	if totalInv > 0 {
		ownerIDs := make([]int64, 0, totalInv)
		kinds := make([]int, 0, totalInv)
		inventoryKeys := make([]int, 0, totalInv)
		datas := make([]string, 0, totalInv)
		versions := make([]int, 0, totalInv)

		for _, snapshot := range batch {
			for _, inv := range snapshot.Inventories {
				ownerIDs = append(ownerIDs, inv.CharacterID)
				kinds = append(kinds, int(inv.Kind))
				inventoryKeys = append(inventoryKeys, int(inv.InventoryKey))
				datas = append(datas, string(inv.Data))
				versions = append(versions, inv.Version)
			}
		}

		err = s.db.Queries().UpsertInventories(ctx, repository.UpsertInventoriesParams{
			OwnerIds:      ownerIDs,
			Kinds:         kinds,
			InventoryKeys: inventoryKeys,
			Datas:         datas,
			Versions:      versions,
		})
		if err != nil {
			s.logger.Error("Failed to batch upsert inventories",
				zap.Int("batch_size", len(batch)),
				zap.Int("inventory_count", totalInv),
				zap.Error(err))
		}
	}
}

// SaveAll saves all characters from CharacterEntities
func (s *CharacterSaver) SaveAll(w *ecs.World) {
	characterEntities := ecs.GetResource[ecs.CharacterEntities](w)
	entityIDs := characterEntities.GetAll()

	s.logger.Info("Saving all characters", zap.Int("count", len(entityIDs)))

	for _, entityID := range entityIDs {
		if charEntity, exists := characterEntities.Map[entityID]; exists {
			s.Save(w, entityID, charEntity.Handle)
		}
	}

	s.logger.Info("All characters saved")
}

func (s *CharacterSaver) Stop() {
	// Process any remaining snapshots in the channel
	remaining := make([]CharacterSnapshot, 0)
	for {
		select {
		case snapshot := <-s.snapshotChannel:
			remaining = append(remaining, snapshot)
		default:
			// No more snapshots
			goto processRemaining
		}
	}

processRemaining:
	if len(remaining) > 0 {
		// Create a new context for final save to avoid context cancellation
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
		defer cancel()

		s.flushBatchWithContext(ctx, remaining)
	}

	// Signal workers to stop and wait for them
	s.cancel()
	s.wg.Wait()
	close(s.snapshotChannel)
	s.logger.Info("CharacterSaver stopped")
}
