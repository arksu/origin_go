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
	ErrInvalidObjectDamageService       = errors.New("object damage: invalid world or destruction service")
	ErrInvalidObjectDamageTarget        = errors.New("object damage: invalid target identity or missing health")
	ErrObjectDamageTargetUnprepared     = errors.New("object damage: target is not prepared")
	ErrObjectDamageTargetDead           = errors.New("object damage: target is dead or being destroyed")
	ErrObjectDamageTargetIndestructible = errors.New("object damage: target is indestructible")
	ErrInvalidObjectDamageHealth        = errors.New("object damage: invalid health")
)

// ObjectDamageResult owns its before/after pools. Armor is zero for objects in v0.
// Damage is the calculated damage before limiting the deduction to remaining HP.
type ObjectDamageResult struct {
	Armor, Damage, BeforeHP, AfterHP float64
	EnteredDestruction               bool
}

// objectDamageCommit is a same-lock plan. Fatal plans reserve persistence before
// any hit in the action is committed; quarantine follows all health writes.
type objectDamageCommit struct {
	target      types.Handle
	result      ObjectDamageResult
	reservation objectDestructionReservation
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
	if info.Indestructible {
		return components.ObjectInternalState{}, ErrObjectDamageTargetIndestructible
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
	var result ObjectDamageResult
	if err := s.calculateDamage(target, rawDamage, &result); err != nil {
		return ObjectDamageResult{}, err
	}
	if !result.EnteredDestruction {
		if result.Damage != 0 {
			s.writeDamage(target, result.AfterHP)
		}
		return result, nil
	}
	commit := objectDamageCommit{target: target, result: result}
	if err := s.reserveDamage(&commit); err != nil {
		return ObjectDamageResult{}, err
	}
	s.commitDamage(commit)
	s.finalizeDamage(commit)
	return commit.result, nil
}

func (s *ObjectDamageService) prepareDamage(target types.Handle, rawDamage float64) (objectDamageCommit, error) {
	var result ObjectDamageResult
	if err := s.calculateDamage(target, rawDamage, &result); err != nil {
		return objectDamageCommit{}, err
	}
	return objectDamageCommit{target: target, result: result}, nil
}

func (s *ObjectDamageService) calculateDamage(target types.Handle, rawDamage float64, result *ObjectDamageResult) error {
	before, err := s.readTarget(target)
	if err != nil {
		return err
	}
	if !s.state.Prepared[target] {
		return ErrObjectDamageTargetUnprepared
	}
	damage, err := combat.DamageAfterArmor(rawDamage, 0)
	if err != nil {
		return err
	}
	*result = ObjectDamageResult{Damage: damage, BeforeHP: before.HP, AfterHP: before.HP}
	if damage == 0 {
		return nil
	} else if damage >= before.HP {
		result.AfterHP = 0
		result.EnteredDestruction = true
	} else {
		result.AfterHP = before.HP - damage
	}
	return nil
}

func (s *ObjectDamageService) reserveDamage(commit *objectDamageCommit) error {
	if !commit.result.EnteredDestruction {
		return nil
	}
	reservation, err := s.destruction.reserve(commit.target)
	if err != nil {
		return err
	}
	commit.reservation = reservation
	return nil
}

func (s *ObjectDamageService) abortDamage(commit objectDamageCommit) {
	if commit.result.EnteredDestruction {
		s.destruction.cancelReservation(commit.reservation)
	}
}

func (s *ObjectDamageService) commitDamage(commit objectDamageCommit) {
	if commit.result.Damage == 0 {
		return
	}
	if commit.result.EnteredDestruction && !s.destruction.commitReservation(commit.reservation) {
		panic("object damage: invalid same-lock reservation")
	}
	s.writeDamage(commit.target, commit.result.AfterHP)
}

func (s *ObjectDamageService) writeDamage(target types.Handle, afterHP float64) {
	ecs.WithComponent(s.world, target, func(state *components.ObjectInternalState) {
		state.HP = afterHP
		state.IsDirty = true
	})
}

func (s *ObjectDamageService) finalizeDamage(commit objectDamageCommit) {
	if commit.result.EnteredDestruction && !s.destruction.finalizeReservation(commit.reservation) {
		panic("object damage: invalid same-lock finalization")
	}
}
