package systems

import (
	"math"
	"time"

	"go.uber.org/zap"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entitystats"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/playerstate"
	"origin/internal/types"
)

const DirectionalInputTTL = 800 * time.Millisecond

type ManualMovementCanceler interface {
	CancelForManualMovement(playerID types.EntityID, playerHandle types.Handle)
}

func (s *NetworkCommandSystem) SetDirectionalSessionValidator(validator func(types.EntityID, uint64, uint32) bool) {
	s.directionalSessionValidator = validator
}

func (s *NetworkCommandSystem) SetManualMovementCanceler(canceler ManualMovementCanceler) {
	s.manualMovementCanceler = canceler
}

func newerInputRevision(revision, previous uint32) bool {
	return revision != 0 && (previous == 0 || int32(revision-previous) > 0)
}

func directionalMovementRestricted(w *ecs.World, handle types.Handle, movement components.Movement) bool {
	if movement.State == constt.StateStunned {
		return true
	}
	return playerstate.IsIncapacitated(w, handle)
}

func (s *NetworkCommandSystem) handleMoveDirection(w *ecs.World, handle types.Handle, command *network.PlayerCommand) {
	direction, ok := command.Payload.(*netproto.MoveDirection)
	if !ok || !network.ValidMoveDirection(direction) || command.Layer != w.Layer ||
		s.directionalSessionValidator == nil || !s.directionalSessionValidator(command.CharacterID, command.ClientID, direction.StreamEpoch) ||
		ecs.GetResource[ecs.DetachedEntities](w).IsDetached(command.CharacterID) {
		return
	}
	if health, exists := ecs.GetComponent[components.EntityHealth](w, handle); exists && health.HHP <= 0 {
		return
	}
	movement, exists := ecs.GetComponent[components.Movement](w, handle)
	if !exists {
		return
	}
	if movement.Direction.ClientID != command.ClientID || movement.Direction.StreamEpoch != direction.StreamEpoch {
		movement.ResetDirectionSession(command.ClientID, direction.StreamEpoch)
	}
	timing := ecs.GetResource[ecs.TimeState](w)
	age := timing.WallNow.Sub(command.ReceivedAt)
	if age < 0 {
		age = 0
	}
	deadline := timing.Now.Add(DirectionalInputTTL - age)
	x, y := float64(direction.X), float64(direction.Y)
	length := math.Hypot(x, y)
	if length != 0 {
		x, y = x/length, y/length
	}
	if direction.InputRevision == movement.Direction.Revision {
		if movement.TargetType != constt.TargetDirection || movement.Direction.InputX != direction.X || movement.Direction.InputY != direction.Y {
			return
		}
		// Receipt, not drain time, decides whether a confirmation arrived before expiry.
		if age >= DirectionalInputTTL || !timing.Now.Add(-age).Before(movement.Direction.ExpiresAt) || directionalMovementRestricted(w, handle, movement) {
			movement.ReleaseDirection()
		} else if deadline.After(movement.Direction.ExpiresAt) {
			movement.Direction.ExpiresAt = deadline
		}
		ecs.WithComponent(w, handle, func(current *components.Movement) { *current = movement })
		return
	}
	if !newerInputRevision(direction.InputRevision, movement.Direction.Revision) {
		return
	}
	// Retain rejected fresh revisions so their later heartbeats cannot become presses.
	movement.Direction.Revision = direction.InputRevision
	if length == 0 || age >= DirectionalInputTTL || directionalMovementRestricted(w, handle, movement) {
		movement.ReleaseDirection()
		ecs.WithComponent(w, handle, func(current *components.Movement) { *current = movement })
		return
	}
	if stats, exists := ecs.GetComponent[components.EntityStats](w, handle); exists {
		_, carrying := ecs.GetComponent[components.LiftCarryState](w, handle)
		_, canMove := entitystats.ResolveAllowedMoveModeWithCarry(movement.Mode, stats.Stamina,
			entitystats.MaxStaminaFromCon(resolveConForHandle(w, handle)), stats.Energy, carrying)
		if !canMove {
			movement.ReleaseDirection()
			ecs.WithComponent(w, handle, func(current *components.Movement) { *current = movement })
			return
		}
	}
	ecs.WithComponent(w, handle, func(current *components.Movement) { *current = movement })
	if !s.prepareManualMovement(w, handle, command.CharacterID) {
		ecs.WithComponent(w, handle, func(current *components.Movement) { current.ReleaseDirection() })
		return
	}
	// Cancellation callbacks may have changed movement; never overwrite them with an old copy.
	movement, exists = ecs.GetComponent[components.Movement](w, handle)
	if !exists || directionalMovementRestricted(w, handle, movement) {
		return
	}
	ecs.WithComponent(w, handle, func(current *components.Movement) {
		current.SetDirection(x, y, direction.InputRevision, deadline)
		current.Direction.InputX, current.Direction.InputY = direction.X, direction.Y
	})
}

func (s *NetworkCommandSystem) prepareManualMovement(w *ecs.World, handle types.Handle, playerID types.EntityID) bool {
	if active, exists := ecs.GetComponent[components.ActiveGameAction](w, handle); exists && active.Phase != components.GameActionSelecting {
		if s.actionService == nil {
			return false
		}
		s.actionService.Cancel(w, playerID, handle)
	}
	ClearPlayerInteractionIntents(w, handle, playerID)
	if _, _, err := ecs.BreakLinkForPlayer(w, playerID, ecs.LinkBreakMoved); err != nil {
		s.logger.Error("manual movement link cleanup failed", zap.Uint64("player_id", uint64(playerID)), zap.Error(err))
		return false
	}
	if _, exists := ecs.GetComponent[components.ActiveCyclicAction](w, handle); exists {
		if s.manualMovementCanceler == nil {
			return false
		}
		s.manualMovementCanceler.CancelForManualMovement(playerID, handle)
	}
	return true
}
