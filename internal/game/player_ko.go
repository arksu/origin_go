package game

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
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

func (s *Shard) HandlePlayerItemsLocked(w *ecs.World, playerID types.EntityID, handle types.Handle) {
	if active, ok := ecs.GetComponent[components.ActiveGameAction](w, handle); ok && s.actionService != nil && s.actionService.requiresItemMutation(active.ActionID) {
		s.actionService.alert(playerID, playerstate.ItemsLockedReason)
		s.actionService.Cancel(w, playerID, handle)
	}
	if active, ok := ecs.GetComponent[components.ActiveCyclicAction](w, handle); ok && active.MutatesItems && s.contextActions != nil {
		s.contextActions.sendMiniAlert(playerID, netproto.AlertSeverity_ALERT_SEVERITY_WARNING, playerstate.ItemsLockedReason)
		s.contextActions.cancelActiveCyclicAction(playerID, handle, playerstate.ItemsLockedReason)
	}
	ecs.RemoveComponent[components.PendingInteraction](w, handle)
	if pending, ok := ecs.GetComponent[components.PendingContextAction](w, handle); ok && pending.MutatesItems {
		ecs.RemoveComponent[components.PendingContextAction](w, handle)
	}
	if _, pending := ecs.GetComponent[components.PendingBuildPlacement](w, handle); pending && s.buildService != nil {
		s.buildService.CancelPendingBuildPlacement(w, playerID, handle)
	}
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
