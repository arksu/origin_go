package game

import (
	"errors"
	"math"

	"origin/internal/combat"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entityhealth"
	"origin/internal/playerstate"
	"origin/internal/types"
)

var (
	ErrInvalidCreatureDamageService = errors.New("creature damage: invalid world or equipment resolver")
	ErrInvalidCreatureTarget        = errors.New("creature damage: invalid target identity or missing health")
	ErrCreatureTargetUnprepared     = errors.New("creature damage: target is not prepared")
	ErrCreatureTargetDead           = errors.New("creature damage: target is dead")
	ErrInvalidCreatureHealth        = errors.New("creature damage: invalid health state")
	ErrInvalidCreatureDamageTime    = errors.New("creature damage: invalid time or knockout deadline overflow")
)

// CreatureDamageResult owns its health snapshots. SoftDamage and HardDamage are
// the calculated split before limiting the deductions to the remaining pools.
type CreatureDamageResult struct {
	Armor, Damage, SoftDamage, HardDamage float64
	Before, After                         components.EntityHealth
	EnteredKO, Dead                       bool
}

// creatureDamageCommit is valid only during the shard-locked operation that
// prepared it. Calculation owns its snapshots; committing performs no reads
// that can reject a hit after another target has already been changed.
type creatureDamageCommit struct {
	target   types.Handle
	identity types.EntityID
	result   CreatureDamageResult
	now      int64
}

// CreatureDamageService applies already calculated Draw to a real health target.
// Construct and prepare under exclusive world access, outside the hit path. Apply
// requires the owning shard lock and must precede PlayerDeathSystem (priority 470).
// Recreate the service when replacing its World, resources, storages or definitions.
// It retains identities and notification registrations, never health or armor.
type CreatureDamageService struct {
	world      *ecs.World
	equipment  *CombatEquipmentResolver
	health     *ecs.ComponentStorage[components.EntityHealth]
	identities *ecs.ComponentStorage[ecs.ExternalID]
	time       *ecs.TimeState
	stats      *ecs.EntityStatsUpdateState
	visual     *ecs.CharacterVisualDirtyQueue
	config     *ecs.EntityStatsRuntimeConfig
	targets    map[types.Handle]types.EntityID
}

func NewCreatureDamageService(world *ecs.World, equipment *CombatEquipmentResolver) (*CreatureDamageService, error) {
	if world == nil || equipment == nil || equipment.world != world || equipment.catalog == nil ||
		equipment.catalog.items == nil || equipment.index == nil || equipment.identities == nil || equipment.containers == nil {
		return nil, ErrInvalidCreatureDamageService
	}
	clock, hasTime := ecs.TryGetResource[ecs.TimeState](world)
	stats, hasStats := ecs.TryGetResource[ecs.EntityStatsUpdateState](world)
	visual, hasVisual := ecs.TryGetResource[ecs.CharacterVisualDirtyQueue](world)
	config, hasConfig := ecs.TryGetResource[ecs.EntityStatsRuntimeConfig](world)
	if !hasTime || !hasStats || !hasVisual || !hasConfig {
		return nil, ErrInvalidCreatureDamageService
	}
	service := &CreatureDamageService{
		world: world, equipment: equipment,
		health:     ecs.GetOrCreateStorage[components.EntityHealth](world),
		identities: ecs.GetOrCreateStorage[ecs.ExternalID](world),
		time:       clock, stats: stats, visual: visual, config: config,
		targets: make(map[types.Handle]types.EntityID),
	}
	// StopMovement uses an observer-aware mutation. Ensure even an absent
	// Movement component cannot cause storage creation on the first KO.
	ecs.GetOrCreateStorage[components.Movement](world)
	world.AddDespawnObserver(service.releaseTarget)
	world.AddComponentObserver(components.EntityHealthComponentID, func(target types.Handle) {
		if !service.health.Has(target) {
			service.releaseTarget(target)
		}
	})
	return service, nil
}

// PrepareTarget allocates only on admission. Detaching a live body preserves its
// preparation; despawn and removal of EntityHealth release it synchronously.
func (service *CreatureDamageService) PrepareTarget(target types.Handle) error {
	identity, _, err := service.readTarget(target)
	if err != nil {
		return err
	}
	if previous, exists := service.targets[target]; exists && previous != identity {
		return ErrInvalidCreatureTarget
	}
	if !service.stats.PreparePlayer(identity, target) {
		return ErrInvalidCreatureTarget
	}
	service.visual.Prepare(target)
	service.targets[target] = identity
	return nil
}

func (service *CreatureDamageService) readTarget(target types.Handle) (types.EntityID, components.EntityHealth, error) {
	if service == nil || service.world == nil {
		return 0, components.EntityHealth{}, ErrInvalidCreatureDamageService
	}
	if !service.world.Alive(target) {
		return 0, components.EntityHealth{}, ErrInvalidCreatureTarget
	}
	identity, exists := service.identities.Get(target)
	if !exists || identity.ID == 0 || service.world.GetHandleByEntityID(identity.ID) != target {
		return 0, components.EntityHealth{}, ErrInvalidCreatureTarget
	}
	health, exists := service.health.Get(target)
	if !exists {
		return 0, components.EntityHealth{}, ErrInvalidCreatureTarget
	}
	if entityhealth.ValidatePools(health.SHP, health.HHP) != nil || health.SHP > health.HHP || health.KOUntilUnixMs < 0 {
		return 0, components.EntityHealth{}, ErrInvalidCreatureHealth
	}
	if health.HHP == 0 {
		return 0, components.EntityHealth{}, ErrCreatureTargetDead
	}
	return identity.ID, health, nil
}

// Apply performs bounded equipment reads and an atomic health commit. All errors
// are sentinels and leave health, movement and notification queues unchanged.
// Multiple calls are sequential: each sees the preceding call's committed state.
func (service *CreatureDamageService) Apply(target types.Handle, rawDamage float64) (CreatureDamageResult, error) {
	var result CreatureDamageResult
	identity, now, err := service.calculateDamage(target, rawDamage, &result)
	if err != nil {
		return CreatureDamageResult{}, err
	}
	service.commitDamageResult(target, identity, now, &result)
	return result, nil
}

// prepareDamage validates and calculates without writing health or queues.
// All prepared hits must be committed before lifecycle cleanup can run.
func (service *CreatureDamageService) prepareDamage(target types.Handle, rawDamage float64) (creatureDamageCommit, error) {
	var result CreatureDamageResult
	identity, now, err := service.calculateDamage(target, rawDamage, &result)
	if err != nil {
		return creatureDamageCommit{}, err
	}
	return creatureDamageCommit{target: target, identity: identity, result: result, now: now}, nil
}

// calculateDamage fills caller-owned storage so standalone Apply does not copy
// a larger coordinator plan through the call boundary.
func (service *CreatureDamageService) calculateDamage(target types.Handle, rawDamage float64, result *CreatureDamageResult) (types.EntityID, int64, error) {
	identity, before, err := service.readTarget(target)
	if err != nil {
		return 0, 0, err
	}
	preparedID, prepared := service.targets[target]
	if !prepared || preparedID != identity || !service.stats.IsPlayerPrepared(identity, target) || !service.visual.IsPrepared(target) {
		return 0, 0, ErrCreatureTargetUnprepared
	}
	if rawDamage < 0 || math.IsNaN(rawDamage) || math.IsInf(rawDamage, 0) {
		return 0, 0, combat.ErrInvalidInput
	}
	now := service.time.UnixMs
	if now < 0 {
		return 0, 0, ErrInvalidCreatureDamageTime
	}
	armor, err := service.equipment.ResolveArmor(target)
	if err != nil {
		return 0, 0, err
	}
	damage, err := combat.DamageAfterArmor(rawDamage, armor)
	if err != nil {
		return 0, 0, err
	}
	*result = CreatureDamageResult{Armor: armor, Damage: damage, Before: before, After: before}
	if damage == 0 {
		// Zero damage does not advance KO. The existing health pass will resolve
		// expired deadlines, even when this validated hit is a no-op.
		return identity, now, nil
	}
	if result.After.KOUntilUnixMs != 0 && now > result.After.KOUntilUnixMs {
		if !result.After.IsLying && result.After.LyingRevision == math.MaxUint64 {
			return 0, 0, ErrInvalidCreatureHealth
		}
		playerstate.ResolveKnockout(&result.After, now)
	}
	result.SoftDamage, result.HardDamage, err = combat.SplitCreatureDamage(damage, result.After.SHP, result.After.KOUntilUnixMs != 0)
	if err != nil {
		return 0, 0, err
	}
	result.After.SHP, result.After.HHP, _, result.Dead = entityhealth.ApplyDamage(
		result.After.SHP, result.After.HHP, result.After.HHP, result.SoftDamage, result.HardDamage,
	)
	if result.Dead {
		result.After.KOUntilUnixMs = 0
	} else if result.After.SHP == 0 && result.After.KOUntilUnixMs == 0 {
		if now > math.MaxInt64-playerstate.KnockoutDurationMs {
			return 0, 0, ErrInvalidCreatureDamageTime
		}
		if !result.After.IsLying && result.After.LyingRevision == math.MaxUint64 {
			return 0, 0, ErrInvalidCreatureHealth
		}
		playerstate.StartKnockout(&result.After, now)
		result.EnteredKO = true
	}
	return identity, now, nil
}

// commitDamage consumes a valid same-lock plan. The coordinator has completed
// every fallible check and must not run target cleanup between planning and this
// call. Observer-aware writes preserve the existing notification contract.
func (service *CreatureDamageService) commitDamage(commit creatureDamageCommit) {
	service.commitDamageResult(commit.target, commit.identity, commit.now, &commit.result)
}

func (service *CreatureDamageService) commitDamageResult(target types.Handle, identity types.EntityID, now int64, result *CreatureDamageResult) {
	if result.Damage == 0 {
		return
	}
	ecs.WithComponent(service.world, target, func(health *components.EntityHealth) { *health = result.After })
	if result.After.HHP == 0 || result.After.SHP == 0 || result.After.KOUntilUnixMs != 0 || result.After.IsLying {
		playerstate.StopMovement(service.world, target)
	}
	if result.Before.IsLying != result.After.IsLying {
		service.visual.Mark(target)
	}
	if result.Before != result.After {
		ttl := service.config.PlayerStatsTTLms
		if ttl == 0 {
			ttl = ecs.ResolvePlayerStatsTTLms(service.world)
		}
		if result.Before.KOUntilUnixMs != result.After.KOUntilUnixMs || result.Before.IsLying != result.After.IsLying || result.Dead {
			ttl = 0
		}
		service.stats.MarkPlayerDirty(identity, now, ttl)
	}
}

func (service *CreatureDamageService) releaseTarget(target types.Handle) {
	identity, prepared := service.targets[target]
	if !prepared {
		return
	}
	service.stats.ReleasePlayer(identity, target)
	service.visual.Forget(target)
	delete(service.targets, target)
}
