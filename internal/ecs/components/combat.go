package components

import (
	"origin/internal/combat"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/types"
	"time"
)

// CombatState is transient actor-owned state; cooldowns outlive the execution.
type CombatState struct {
	Execution         *CombatExecution
	Cooldowns         map[string]time.Time
	Sequence          uint64
	Revision          uint64
	LastCombatEventAt int64
	ClientID          uint64
	StreamEpoch       uint32
	RequestRevision   uint64
}

type CombatExecution struct {
	StartEventSequence  uint64
	ID                  uint64
	ActionID            string
	SelectionGeneration uint64
	Direction           combat.Point
	Weapon              combat.Weapon
	WeaponItemID        types.EntityID
	WeaponSlot          string
	AngleDegrees        float64
	Multiplier          float64
	Nearest             bool
	StartedAt           time.Time
	StrikeAt            time.Time
	RecoveryEnd         time.Time
	StartedAtMs         int64
	StrikeResolved      bool
}

const CombatStateComponentID ecs.ComponentID = 37

func init() { ecs.RegisterComponent[CombatState](CombatStateComponentID) }

func CombatCommitted(world *ecs.World, handle types.Handle) bool {
	state, exists := ecs.GetComponent[CombatState](world, handle)
	return exists && state.Execution != nil
}

func CombatMovementCapped(world *ecs.World, handle types.Handle) bool {
	state, exists := ecs.GetComponent[CombatState](world, handle)
	return exists && state.Execution != nil && !state.Execution.StrikeResolved
}

func EffectiveCombatMoveMode(world *ecs.World, handle types.Handle, mode constt.MoveMode) constt.MoveMode {
	if CombatMovementCapped(world, handle) {
		return constt.Crawl
	}
	return mode
}
