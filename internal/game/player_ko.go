package game

import (
	"go.uber.org/zap"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/playerstate"
	"origin/internal/types"
)

func (s *Shard) queueStandUp(w *ecs.World, handle types.Handle, command *network.PlayerCommand) {
	request, ok := command.Payload.(*netproto.StandUp)
	if !ok || request == nil {
		return
	}
	if !s.currentStandUpConnection(w, command.CharacterID, command.ClientID) {
		return
	}
	if s.pendingStandUps == nil {
		s.pendingStandUps = make(map[types.EntityID]*network.PlayerCommand)
	}
	s.pendingStandUps[command.CharacterID] = command
}

func (s *Shard) ApplyPendingStandUp(w *ecs.World, playerID types.EntityID, handle types.Handle) {
	command := s.pendingStandUps[playerID]
	if command == nil {
		return
	}
	delete(s.pendingStandUps, playerID)
	request := command.Payload.(*netproto.StandUp)
	if !s.currentStandUpConnection(w, playerID, command.ClientID) {
		return
	}
	if !s.validDirectionalSession(playerID, command.ClientID, request.StreamEpoch) {
		s.sendPlayerStats(w, playerID, handle, true)
		return
	}
	if playerstate.CanStandUp(w, handle) {
		ecs.WithComponent(w, handle, func(health *components.EntityHealth) { playerstate.SetLying(health, false) })
		ecs.MarkCharacterVisualDirty(w, playerID)
	}
	s.sendPlayerStats(w, playerID, handle, true)
}

func (s *Shard) HandlePlayerIncapacitated(w *ecs.World, playerID types.EntityID, handle types.Handle) {
	// KO terminates all station/object interactions. Use the normal synchronous
	// unlink so container, crafting and building subscribers perform their cleanup.
	// Retargeting and pending approaches must not survive until the player stands.
	systems.ClearPlayerInteractionIntents(w, handle, playerID)
	ecs.GetResource[ecs.PendingAdminTeleport](w).Clear(playerID)
	if _, _, err := ecs.BreakLinkForPlayer(w, playerID, ecs.LinkBreakKnockedOut); err != nil {
		s.logger.Error("KO link cleanup failed", zap.Uint64("player_id", uint64(playerID)), zap.Error(err))
	}
	if active, ok := ecs.GetComponent[components.ActiveGameAction](w, handle); ok && s.actionService != nil &&
		(s.actionService.requiresItemMutation(active.ActionID) || s.actionService.requiresObjectInteraction(active.ActionID) || active.Phase == components.GameActionApproaching) {
		if s.actionService.requiresItemMutation(active.ActionID) {
			s.actionService.alert(playerID, playerstate.ItemsLockedReason)
		}
		s.actionService.Cancel(w, playerID, handle)
	}
	if active, ok := ecs.GetComponent[components.ActiveCyclicAction](w, handle); ok && (active.MutatesItems || active.TargetKind == components.CyclicActionTargetObject) && s.contextActions != nil {
		reason := "link_broken"
		if active.MutatesItems {
			reason = playerstate.ItemsLockedReason
			s.contextActions.sendMiniAlert(playerID, netproto.AlertSeverity_ALERT_SEVERITY_WARNING, reason)
		}
		s.contextActions.cancelActiveCyclicAction(playerID, handle, reason)
	}
	if _, pending := ecs.GetComponent[components.PendingBuildPlacement](w, handle); pending && s.buildService != nil {
		s.buildService.CancelPendingBuildPlacement(w, playerID, handle)
	}
	if s.liftService != nil {
		s.liftService.CancelPendingLiftTransition(w, playerID, handle)
	} else {
		ecs.RemoveComponent[components.PendingLiftTransition](w, handle)
		ecs.WithComponent(w, handle, func(collider *components.Collider) { collider.Phantom = nil })
	}
	playerstate.StopMovement(w, handle)
}

func (s *Shard) currentStandUpConnection(w *ecs.World, playerID types.EntityID, clientID uint64) bool {
	if ecs.GetResource[ecs.DetachedEntities](w).IsDetached(playerID) {
		return false
	}
	s.ClientsMu.RLock()
	defer s.ClientsMu.RUnlock()
	client := s.Clients[playerID]
	return client != nil && client.ID == clientID && client.InWorld.Load()
}
