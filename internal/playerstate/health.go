package playerstate

import (
	"math"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

const KnockoutDurationMs int64 = 60_000
const ItemsLockedReason = "PLAYER_ITEMS_LOCKED"

func SetLying(health *components.EntityHealth, lying bool) {
	if health.IsLying != lying {
		health.IsLying = lying
		health.LyingRevision++
	}
}

// Observe depletion immediately, before regeneration can hide it. Completion is
// deferred until the health system has processed this tick's incoming damage.
func StartKnockout(health *components.EntityHealth, nowMs int64) {
	if health.HHP > 0 && health.SHP <= 0 && health.KOUntilUnixMs == 0 {
		health.KOUntilUnixMs = nowMs + KnockoutDurationMs
		SetLying(health, true)
	}
}

func ResolveKnockout(health *components.EntityHealth, nowMs int64) {
	if health.HHP <= 0 {
		health.KOUntilUnixMs = 0
		return
	}
	if health.KOUntilUnixMs != 0 {
		if nowMs >= health.KOUntilUnixMs {
			health.SHP = math.Min(health.HHP, math.Max(health.SHP, 1))
			health.KOUntilUnixMs = 0
			SetLying(health, true)
		}
		return
	}
	StartKnockout(health, nowMs)
}

// IsIncapacitated also covers depletion before the health system observes it.
// KO and the remaining lying pose forbid movement, teleporting and object links.
func IsIncapacitated(w *ecs.World, handle types.Handle) bool {
	health, exists := ecs.GetComponent[components.EntityHealth](w, handle)
	return exists && (health.KOUntilUnixMs != 0 || health.IsLying || health.HHP <= 0 || health.SHP <= 0)
}

func ItemsLocked(w *ecs.World, handle types.Handle) bool {
	return ecs.InventoryHandleReserved(w, handle) || IsIncapacitated(w, handle)
}

func StopMovement(w *ecs.World, handle types.Handle) {
	ecs.WithComponent(w, handle, func(movement *components.Movement) {
		if movement.TargetType != constt.TargetNone || movement.State == constt.StateMoving || movement.VelocityX != 0 || movement.VelocityY != 0 {
			movement.ClearTarget()
			// Health runs after TransformUpdate; the next movement pass must publish
			// a stop even for a retired click route. Keep the accepted input revision.
			movement.Direction.UpdatePending = true
		}
	})
}

func CanStandUp(w *ecs.World, handle types.Handle) bool {
	health, exists := ecs.GetComponent[components.EntityHealth](w, handle)
	if !exists || health.HHP <= 0 || !health.IsLying || health.KOUntilUnixMs != 0 {
		return false
	}
	movement, exists := ecs.GetComponent[components.Movement](w, handle)
	return !exists || movement.State != constt.StateStunned
}

func ObserveHealthChange(w *ecs.World, playerID types.EntityID, handle types.Handle) {
	before, exists := ecs.GetComponent[components.EntityHealth](w, handle)
	if !exists {
		return
	}
	ecs.WithComponent(w, handle, func(health *components.EntityHealth) {
		StartKnockout(health, ecs.GetResource[ecs.TimeState](w).UnixMs)
	})
	after, _ := ecs.GetComponent[components.EntityHealth](w, handle)
	if IsIncapacitated(w, handle) {
		StopMovement(w, handle)
	}
	if before.IsLying != after.IsLying {
		ecs.MarkCharacterVisualDirty(w, playerID)
	}
	ecs.MarkPlayerStatsDirty(w, playerID, 0)
}
