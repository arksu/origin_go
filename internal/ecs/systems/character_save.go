package systems

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"origin/internal/characterattrs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entityhealth"
	"origin/internal/types"
	"time"

	"go.uber.org/zap"
)

const CharacterSaveCaptureRetryInterval = 5 * time.Second

var (
	ErrMissingCharacterHealth    = errors.New("save character: missing EntityHealth component")
	ErrMissingCharacterTransform = errors.New("save character: missing Transform component")
	ErrMissingCharacterStats     = errors.New("save character: missing EntityStats component")
	ErrInvalidCharacterHandle    = errors.New("save character: invalid entity handle")
	ErrCharacterSaverStopped     = errors.New("character saver is stopped")
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

// StrictInventorySaverInterface permits capture failures to abort a save while
// retaining compatibility with lightweight inventory serializers.
type StrictInventorySaverInterface interface {
	SerializeInventoriesStrict(world *ecs.World, characterID types.EntityID, handle types.Handle) ([]InventorySnapshot, error)
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

		if err := s.saver.Save(w, entityID, charEntity.Handle); err != nil {
			retryAt := now.Add(CharacterSaveCaptureRetryInterval)
			charEntities.RescheduleSave(entityID, retryAt)
			// Periodic and final detached capture share the same retry deadline.
			// Expiry runs later in this tick and must not recapture a rejected state.
			detached := ecs.GetResource[ecs.DetachedEntities](w)
			detached.SetSaveRetryAt(entityID, charEntity.Handle, retryAt)
			s.logger.Error("Character snapshot rejected; save rescheduled", zap.Uint64("entity_id", uint64(entityID)), zap.Error(err))
			continue
		}

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
	CharacterID     int64
	X               int
	Y               int
	Heading         int16
	Stamina         float64
	Energy          float64
	SHP             float64
	HHP             float64
	IsLying         bool
	Attributes      string
	Exp             string
	Skills          string
	ActionCooldowns string
	Discovery       string
	Inventories     []InventorySnapshot
}

// Save captures an owned snapshot under the shard lock. A nil error means it
// was accepted by the queue, not that the asynchronous database write completed.
func (s *CharacterSaver) Save(w *ecs.World, entityID types.EntityID, handle types.Handle) error {
	snapshot, err := s.captureSnapshot(w, entityID, handle)
	if err != nil {
		return err
	}
	if !s.enqueueSnapshot(snapshot) {
		return ErrCharacterSaverStopped
	}
	return nil
}

func (s *CharacterSaver) captureSnapshot(w *ecs.World, entityID types.EntityID, handle types.Handle) (CharacterSnapshot, error) {
	if w == nil || !w.Alive(handle) {
		return CharacterSnapshot{}, ErrInvalidCharacterHandle
	}
	shpValue, hhpValue, isLying, err := s.resolveHealthSnapshotValues(w, handle)
	if err != nil {
		return CharacterSnapshot{}, err
	}
	transform, hasTransform := ecs.GetComponent[components.Transform](w, handle)
	if !hasTransform {
		return CharacterSnapshot{}, ErrMissingCharacterTransform
	}
	staminaValue, energyValue, hasStats := s.resolveStatsSnapshotValues(w, entityID, handle)
	if !hasStats {
		return CharacterSnapshot{}, ErrMissingCharacterStats
	}
	attributesRaw, experienceRaw, skillsRaw, discoveryRaw := s.serializeCharacterProfile(w, entityID, handle)
	var inventories []InventorySnapshot
	if strictSaver, ok := s.inventorySaver.(StrictInventorySaverInterface); ok {
		inventories, err = strictSaver.SerializeInventoriesStrict(w, entityID, handle)
		if err != nil {
			return CharacterSnapshot{}, err
		}
	} else {
		inventories = s.inventorySaver.SerializeInventories(w, entityID, handle)
	}
	snapshot := s.buildSnapshot(entityID, transform, attributesRaw, experienceRaw, skillsRaw, discoveryRaw, staminaValue, energyValue, shpValue, hhpValue, isLying, inventories)
	if err := snapshot.captureActionCooldowns(w, handle); err != nil {
		return CharacterSnapshot{}, err
	}
	return snapshot, nil
}

// SaveSync persists character snapshot immediately in caller goroutine.
// Used by admin teleport before despawn to keep DB state consistent for immediate respawn.
func (s *CharacterSaver) SaveSync(w *ecs.World, entityID types.EntityID, handle types.Handle) error {
	snapshot, err := s.captureSnapshot(w, entityID, handle)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), characterSaveTimeout)
	defer cancel()

	if !s.enqueueSnapshot(snapshot) {
		return ErrCharacterSaverStopped
	}
	return s.flushPending(ctx, s.queueForCharacter(snapshot.CharacterID), snapshot.CharacterID)
}

// SaveDetached enqueues a snapshot for detached-entity expiration path.
// We still persist inventories here to avoid losing recent in-memory changes on detach expiry.
func (s *CharacterSaver) SaveDetached(w *ecs.World, entityID types.EntityID, handle types.Handle) error {
	return s.Save(w, entityID, handle)
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
	shpValue float64,
	hhpValue float64,
	isLying bool,
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
		IsLying:     isLying,
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

func (s *CharacterSaver) resolveHealthSnapshotValues(w *ecs.World, handle types.Handle) (float64, float64, bool, error) {
	if health, hasHealth := ecs.GetComponent[components.EntityHealth](w, handle); hasHealth {
		if err := entityhealth.ValidatePools(health.SHP, health.HHP); err != nil {
			return 0, 0, false, err
		}
		return health.SHP, health.HHP, health.IsLying, nil
	}
	return 0, 0, false, ErrMissingCharacterHealth
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
func (s *CharacterSaver) SaveAll(w *ecs.World) error {
	characterEntities := ecs.GetResource[ecs.CharacterEntities](w)
	entityIDs := characterEntities.GetAll()

	s.logger.Info("Saving all characters", zap.Int("count", len(entityIDs)))

	var failures []error
	for _, entityID := range entityIDs {
		if charEntity, exists := characterEntities.Map[entityID]; exists {
			if err := s.Save(w, entityID, charEntity.Handle); err != nil {
				failures = append(failures, fmt.Errorf("character %d: %w", entityID, err))
			}
		}
	}

	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	s.logger.Info("All character snapshots accepted")
	return nil
}

func (snapshot *CharacterSnapshot) captureActionCooldowns(w *ecs.World, handle types.Handle) error {
	cooldowns, _ := ecs.GetComponent[components.ActionCooldowns](w, handle)
	serialized, err := cooldowns.MarshalActive(ecs.GetResource[ecs.TimeState](w).UnixMs)
	if err != nil {
		return fmt.Errorf("save action cooldowns: %w", err)
	}
	snapshot.ActionCooldowns = serialized
	return nil
}
