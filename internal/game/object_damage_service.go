package game

import (
	"errors"
	"math"

	"origin/internal/combat"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	gameworld "origin/internal/game/world"
	"origin/internal/types"
)

var (
	ErrInvalidObjectDamageService   = errors.New("object damage: invalid world or destruction service")
	ErrInvalidObjectDamageTarget    = errors.New("object damage: invalid target identity or missing health")
	ErrObjectDamageTargetUnprepared = errors.New("object damage: target is not prepared")
	ErrObjectDamageTargetDead       = errors.New("object damage: target is dead or being destroyed")
	ErrInvalidObjectDamageHealth    = errors.New("object damage: invalid health")
)

// ObjectDamageResult owns its before/after pools. Armor is zero for objects in v0.
// Damage is the calculated damage before limiting the deduction to remaining HP.
type ObjectDamageResult struct {
	Armor, Damage, BeforeHP, AfterHP float64
	EnteredDestruction               bool
}

// ObjectDamageService receives already calculated Draw without weapon rules.
// Preparation and Apply require the owning shard lock. Recreate on World changes.
type ObjectDamageService struct {
	world       *ecs.World
	destruction *ObjectDestructionService
	state       *ecs.ObjectDestructionState
	health      *ecs.ComponentStorage[components.ObjectInternalState]
	identities  *ecs.ComponentStorage[ecs.ExternalID]
	info        *ecs.ComponentStorage[components.EntityInfo]
	creatures   *ecs.ComponentStorage[components.EntityHealth]
	dropped     *ecs.ComponentStorage[components.DroppedItem]
}

func NewObjectDamageService(w *ecs.World, destruction *ObjectDestructionService) (*ObjectDamageService, error) {
	if w == nil || destruction == nil || destruction.world != w {
		return nil, ErrInvalidObjectDamageService
	}
	state, ok := ecs.TryGetResource[ecs.ObjectDestructionState](w)
	if !ok || state.Prepared == nil || state.Pending == nil {
		return nil, ErrInvalidObjectDamageService
	}
	return &ObjectDamageService{
		world: w, destruction: destruction, state: state,
		health:     ecs.GetOrCreateStorage[components.ObjectInternalState](w),
		identities: ecs.GetOrCreateStorage[ecs.ExternalID](w),
		info:       ecs.GetOrCreateStorage[components.EntityInfo](w),
		creatures:  ecs.GetOrCreateStorage[components.EntityHealth](w),
		dropped:    ecs.GetOrCreateStorage[components.DroppedItem](w),
	}, nil
}

func (s *ObjectDamageService) PrepareTarget(target types.Handle) error {
	if _, err := s.readTarget(target); err != nil {
		return err
	}
	return s.destruction.PrepareTarget(target)
}

func (s *ObjectDamageService) readTarget(target types.Handle) (components.ObjectInternalState, error) {
	if s == nil || s.world == nil {
		return components.ObjectInternalState{}, ErrInvalidObjectDamageService
	}
	if !s.world.Alive(target) {
		return components.ObjectInternalState{}, ErrInvalidObjectDamageTarget
	}
	identity, ok := s.identities.Get(target)
	if !ok || identity.ID == 0 || identity.ID > math.MaxInt64 || s.world.GetHandleByEntityID(identity.ID) != target {
		return components.ObjectInternalState{}, ErrInvalidObjectDamageTarget
	}
	info, ok := s.info.Get(target)
	if !ok || info.Region != s.destruction.deps.Region || info.Layer != s.world.Layer || info.TypeID == 0 || info.TypeID == constt.DroppedItemTypeID || s.creatures.Has(target) || s.dropped.Has(target) {
		return components.ObjectInternalState{}, ErrInvalidObjectDamageTarget
	}
	health, ok := s.health.Get(target)
	if !ok || !health.HasHP {
		return components.ObjectInternalState{}, ErrInvalidObjectDamageTarget
	}
	if gameworld.ValidateObjectHP(health.HP) != nil {
		return components.ObjectInternalState{}, ErrInvalidObjectDamageHealth
	}
	if health.HP == 0 || s.state.Pending[target] {
		return components.ObjectInternalState{}, ErrObjectDamageTargetDead
	}
	return health, nil
}

// Apply leaves ECS unchanged on rejection. Fatal damage is admitted before the
// destruction service atomically quarantines its target and commits zero HP.
func (s *ObjectDamageService) Apply(target types.Handle, rawDamage float64) (ObjectDamageResult, error) {
	before, err := s.readTarget(target)
	if err != nil {
		return ObjectDamageResult{}, err
	}
	if !s.state.Prepared[target] {
		return ObjectDamageResult{}, ErrObjectDamageTargetUnprepared
	}
	damage, err := combat.DamageAfterArmor(rawDamage, 0)
	if err != nil {
		return ObjectDamageResult{}, err
	}
	result := ObjectDamageResult{Damage: damage, BeforeHP: before.HP, AfterHP: before.HP}
	if damage == 0 {
		return result, nil
	}
	if damage >= before.HP {
		if err := s.destruction.admit(target); err != nil {
			return ObjectDamageResult{}, err
		}
		result.AfterHP = 0
		result.EnteredDestruction = true
		return result, nil
	}
	result.AfterHP = before.HP - damage
	ecs.WithComponent(s.world, target, func(state *components.ObjectInternalState) {
		state.HP = result.AfterHP
		state.IsDirty = true
	})
	return result, nil
}
