package systems

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"origin/internal/characterattrs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
	"time"

	"go.uber.org/zap"
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
	shpValue, hhpValue := s.resolveHealthSnapshotValues(w, handle)
	inventories := s.inventorySaver.SerializeInventories(w, entityID, handle)
	s.enqueueSnapshot(s.buildSnapshot(entityID, transform, attributesRaw, experienceRaw, skillsRaw, discoveryRaw, staminaValue, energyValue, shpValue, hhpValue, inventories))
}

// SaveSync persists character snapshot immediately in caller goroutine.
// Used by admin teleport before despawn to keep DB state consistent for immediate respawn.
func (s *CharacterSaver) SaveSync(w *ecs.World, entityID types.EntityID, handle types.Handle) error {
	transform, hasTransform := ecs.GetComponent[components.Transform](w, handle)
	if !hasTransform {
		return fmt.Errorf("save character %d: missing Transform component", entityID)
	}

	attributesRaw, experienceRaw, skillsRaw, discoveryRaw := s.serializeCharacterProfile(w, entityID, handle)
	staminaValue, energyValue, hasStats := s.resolveStatsSnapshotValues(w, entityID, handle)
	if !hasStats {
		return fmt.Errorf("save character %d: missing EntityStats component", entityID)
	}
	shpValue, hhpValue := s.resolveHealthSnapshotValues(w, handle)
	inventories := s.inventorySaver.SerializeInventories(w, entityID, handle)
	snapshot := s.buildSnapshot(entityID, transform, attributesRaw, experienceRaw, skillsRaw, discoveryRaw, staminaValue, energyValue, shpValue, hhpValue, inventories)

	ctx, cancel := context.WithTimeout(context.Background(), characterSaveTimeout)
	defer cancel()

	if !s.enqueueSnapshot(snapshot) {
		return fmt.Errorf("character saver is stopped")
	}
	return s.flushPending(ctx, s.queueForCharacter(snapshot.CharacterID), snapshot.CharacterID)
}

// SaveDetached enqueues a snapshot for detached-entity expiration path.
// We still persist inventories here to avoid losing recent in-memory changes on detach expiry.
func (s *CharacterSaver) SaveDetached(w *ecs.World, entityID types.EntityID, handle types.Handle) {
	s.Save(w, entityID, handle)
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
	shpValue int16,
	hhpValue int16,
	inventories []InventorySnapshot,
) CharacterSnapshot {
	return CharacterSnapshot{
		CharacterID: int64(entityID),
		X:           int(transform.X),
		Y:           int(transform.Y),
		Heading:     normalizeCharacterHeading(transform.Direction),
		Stamina:     staminaValue,
		Energy:      energyValue,
		SHP:         shpValue,
		HHP:         hhpValue,
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

func (s *CharacterSaver) resolveHealthSnapshotValues(w *ecs.World, handle types.Handle) (int16, int16) {
	if health, hasHealth := ecs.GetComponent[components.EntityHealth](w, handle); hasHealth {
		return roundAndClampInt16(health.SHP), roundAndClampInt16(health.HHP)
	}
	return 100, 100
}

func roundAndClampInt16(value float64) int16 {
	if value <= math.MinInt16 {
		return math.MinInt16
	}
	if value >= math.MaxInt16 {
		return math.MaxInt16
	}
	return int16(math.Round(value))
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
