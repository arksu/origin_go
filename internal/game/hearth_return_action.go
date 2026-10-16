package game

import (
	"fmt"
	"slices"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/hearth"
	"origin/internal/network"
	"origin/internal/persistence/repository"
	"origin/internal/playerstate"
	"origin/internal/types"
)

// Hearth policy stays in this feature adapter. Transfer and action execution
// expose only their ordinary admission, spawn and completion stages.
type hearthReturnActionHandler struct {
	game     *Game
	shard    *Shard
	index    *hearth.Index
	transfer *PlayerTransferService
}

func (h *hearthReturnActionHandler) UnavailableReason(w *ecs.World, playerID types.EntityID, player types.Handle) string {
	if h == nil || h.shard == nil || w != h.shard.world || !w.Alive(player) || w.GetHandleByEntityID(playerID) != player {
		return "Hearth return is unavailable."
	}
	health, healthy := ecs.GetComponent[components.EntityHealth](w, player)
	if !healthy || !(health.HHP > 0) || !(health.SHP > 0) || health.KOUntilUnixMs != 0 {
		return "Cannot return during knockout."
	}
	movement, moving := ecs.GetComponent[components.Movement](w, player)
	if !moving || movement.State == constt.StateStunned {
		return "Hearth return is unavailable."
	}
	if movement.Mode == constt.Swim {
		return "Cannot return while swimming."
	}
	combat, prepared := ecs.GetResource[ecs.CombatActivityState](w).Capture(player)
	if !prepared || ecs.ValidateCombatState(combat) != nil {
		return "Hearth return is unavailable."
	}
	if combat.HasEvent && ecs.GetResource[ecs.TimeState](w).UnixMs < combat.LastCombatEventAtUnixMs+ecs.CombatLogoutHoldMs {
		return "Cannot return during combat."
	}
	if h.index == nil {
		return "Hearth information is unavailable."
	}
	home := h.index.Snapshot(playerID)
	if home.State == hearth.Unknown {
		return "Hearth information is unavailable."
	}
	if !home.CanReturn() {
		return "You do not have an active Hearth Fire."
	}
	h.shard.ClientsMu.RLock()
	client := h.shard.Clients[playerID]
	h.shard.ClientsMu.RUnlock()
	if !hearthReturnSessionOpen(client, playerID, h.shard.layer) {
		return "Hearth return was interrupted."
	}
	return ""
}

func (h *hearthReturnActionHandler) ValidateTarget(w *ecs.World, playerID types.EntityID, player types.Handle, _ ActionTarget) string {
	return h.UnavailableReason(w, playerID, player)
}

func (*hearthReturnActionHandler) InterruptOnGameplayAttempt() bool { return true }

func (h *hearthReturnActionHandler) BeforeExecution(w *ecs.World, playerID types.EntityID, player types.Handle) string {
	if reason := h.UnavailableReason(w, playerID, player); reason != "" {
		return reason
	}
	if _, _, err := ecs.BreakLinkForPlayer(w, playerID, ecs.LinkBreakClosed); err != nil {
		return "Hearth return is unavailable."
	}
	if h.shard.buildService != nil {
		h.shard.buildService.CancelPendingBuildPlacement(w, playerID, player)
	}
	if h.shard.contextActions != nil {
		h.shard.contextActions.cancelActiveCyclicAction(playerID, player, "hearth_return")
	}
	systems.ClearPlayerInteractionIntents(w, player, playerID)
	playerstate.StopMovement(w, player)
	return ""
}

func (h *hearthReturnActionHandler) Start(w *ecs.World, playerID types.EntityID, player types.Handle, _ ActionTarget, generation uint64) ActionResult {
	if reason := h.UnavailableReason(w, playerID, player); reason != "" {
		return ActionResult{Outcome: ActionRejected, Reason: reason}
	}
	if h.game == nil || h.transfer == nil {
		return ActionResult{Outcome: ActionFailed, Reason: "Hearth return is unavailable."}
	}
	req := h.transferRequest(playerID, player, generation)
	if err := h.transfer.RequestTransfer(req); err != nil {
		return ActionResult{Outcome: ActionFailed, Reason: "Hearth return could not start."}
	}
	return ActionResult{Outcome: ActionDeferred}
}

func (*hearthReturnActionHandler) Cancel(*ecs.World, types.EntityID, types.Handle, components.ActiveGameAction) {
}

func hearthReturnSessionOpen(client *network.Client, playerID types.EntityID, layer int) bool {
	if client == nil || client.CharacterID != playerID || client.Layer != layer || !client.InWorld.Load() {
		return false
	}
	select {
	case <-client.Done():
		return false
	default:
		return true
	}
}

func (h *hearthReturnActionHandler) transferRequest(playerID types.EntityID, player types.Handle, generation uint64) PlayerTransferRequest {
	h.shard.ClientsMu.RLock()
	client := h.shard.Clients[playerID]
	h.shard.ClientsMu.RUnlock()
	home := h.index.Snapshot(playerID).Home
	completed := false
	return PlayerTransferRequest{
		PlayerID: playerID, SourceLayer: h.shard.layer, TargetLayer: hearth.SurfaceLayer,
		TargetX: home.X, TargetY: home.Y, Cause: PlayerTransferCauseHearthReturn,
		SourcePolicy: func(source *Shard, current types.Handle, currentClient *network.Client) error {
			if source != h.shard || current != player || currentClient != client || !hearthReturnSessionOpen(client, playerID, source.layer) {
				return fmt.Errorf("hearth return session changed")
			}
			active, exists := ecs.GetComponent[components.ActiveGameAction](source.world, player)
			if !exists || active.ActionID != hearth.ReturnActionID || active.Generation != generation ||
				source.actionService == nil || !source.actionService.CanCommit(source.world, playerID, player, generation) {
				return fmt.Errorf("hearth return action was interrupted")
			}
			return nil
		},
		SpawnTarget: func(target *Shard, character repository.Character, runtime playerRuntimeState, finalize playerSpawnFinalizer) (types.Handle, repository.Character, error) {
			// Only the target copy stands. The source rollback snapshot remains
			// untouched, including its lying revision and KO/combat metadata.
			playerstate.SetLying(&runtime.Health, false)
			character.IsLying = false
			return h.game.spawnTeleportedPlayerAtDestination(client, target, character, &runtime, finalize,
				func(shard *Shard) (playerSpawnDestination, error) {
					return resolveHearthSpawnDestinationLocked(shard, playerID)
				},
				func(shard *Shard, destination playerSpawnDestination) error {
					return validateHearthSpawnDestinationLocked(shard, playerID, destination)
				},
				SpawnCollisionPolicy{})
		},
		OnResult: func(resultShard *Shard, target types.Handle, err error) {
			if completed {
				return
			}
			completed = true
			if err == nil {
				if resultShard.actionService == nil || !resultShard.world.Alive(target) || resultShard.world.GetHandleByEntityID(playerID) != target {
					return
				}
				if definition, exists := resultShard.actionService.definitions.Get(hearth.ReturnActionID); exists {
					resultShard.actionService.startCooldown(resultShard.world, target, definition)
				}
				return
			}
			// A failure before detach must retire deferred progress. A late result
			// cannot clear a new body, a new connection or a later action attempt.
			if resultShard != h.shard || !resultShard.world.Alive(player) || resultShard.world.GetHandleByEntityID(playerID) != player {
				return
			}
			resultShard.ClientsMu.RLock()
			currentClient := resultShard.Clients[playerID]
			resultShard.ClientsMu.RUnlock()
			active, exists := ecs.GetComponent[components.ActiveGameAction](resultShard.world, player)
			if currentClient == client && exists && active.ActionID == hearth.ReturnActionID && active.Generation == generation && resultShard.actionService != nil {
				resultShard.actionService.Complete(resultShard.world, playerID, player, generation, false, "Hearth return failed. The destination may be blocked or unavailable.")
			}
		},
	}
}

func resolveHearthSpawnDestinationLocked(shard *Shard, playerID types.EntityID) (playerSpawnDestination, error) {
	if shard.layer != hearth.SurfaceLayer {
		return playerSpawnDestination{}, fmt.Errorf("hearth is not on the surface")
	}
	runtime, exists := ecs.TryGetResource[hearth.Runtime](shard.world)
	if !exists || runtime.Index == nil {
		return playerSpawnDestination{}, fmt.Errorf("hearth information unavailable")
	}
	snapshot := runtime.Index.Snapshot(playerID)
	if !snapshot.CanReturn() || snapshot.Home.Layer != shard.layer {
		return playerSpawnDestination{}, fmt.Errorf("no eligible hearth")
	}
	home := snapshot.Home
	return playerSpawnDestination{X: home.X, Y: home.Y, Region: home.Object.Region, ObjectID: home.Object.ID}, nil
}

func validateHearthSpawnDestinationLocked(shard *Shard, playerID types.EntityID, destination playerSpawnDestination) error {
	current, err := resolveHearthSpawnDestinationLocked(shard, playerID)
	if err != nil || current != destination {
		return fmt.Errorf("hearth changed")
	}
	handle := shard.world.GetHandleByEntityID(destination.ObjectID)
	if !shard.world.Alive(handle) || ecs.ObjectDestructionPending(shard.world, handle) {
		return fmt.Errorf("hearth is not available")
	}
	info, hasInfo := ecs.GetComponent[components.EntityInfo](shard.world, handle)
	position, hasPosition := ecs.GetComponent[components.Transform](shard.world, handle)
	if !hasInfo || !hasPosition || info.Region != destination.Region || info.Layer != shard.layer ||
		!slices.Contains(info.Behaviors, "hearth") || position.X != float64(destination.X) || position.Y != float64(destination.Y) {
		return fmt.Errorf("hearth identity or position changed")
	}
	return nil
}
