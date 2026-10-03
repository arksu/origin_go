package systems

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

type CombatActionCommands interface {
	IsCombatAction(string) bool
	CombatSupported() bool
	ActivateCombat(*ecs.World, types.EntityID, types.Handle, string)
	CommitCombat(*ecs.World, types.EntityID, types.Handle, *netproto.MapClick)
}

func (s *NetworkCommandSystem) consumeCombatRevision(world *ecs.World, handle types.Handle, command *network.PlayerCommand, epoch uint32, revision uint64) bool {
	if epoch == 0 || revision == 0 || command.Layer != world.Layer || s.directionalSessionValidator == nil ||
		!s.directionalSessionValidator(command.CharacterID, command.ClientID, epoch) || ecs.GetResource[ecs.DetachedEntities](world).IsDetached(command.CharacterID) {
		return false
	}
	state, exists := ecs.GetComponent[components.CombatState](world, handle)
	if !exists {
		state = components.CombatState{}
	}
	if state.ClientID != command.ClientID || state.StreamEpoch != epoch {
		state.ClientID = command.ClientID
		state.StreamEpoch = epoch
		state.RequestRevision = 0
	}
	if revision <= state.RequestRevision {
		return false
	}
	state.RequestRevision = revision
	ecs.AddComponent(world, handle, state)
	return true
}

func (s *NetworkCommandSystem) handleActionActivation(world *ecs.World, handle types.Handle, command *network.PlayerCommand) {
	request, ok := command.Payload.(*netproto.C2S_ActivateAction)
	if !ok || request == nil || s.actionService == nil {
		return
	}
	combat, hasCombat := s.actionService.(CombatActionCommands)
	tagged := request.RequestRevision != 0 || request.StreamEpoch != 0
	if tagged || (hasCombat && combat.IsCombatAction(request.ActionId)) {
		if !hasCombat || !combat.CombatSupported() || !s.consumeCombatRevision(world, handle, command, request.StreamEpoch, request.RequestRevision) {
			return
		}
		// A well-formed attempt remains consumed even when gameplay rejects it.
		combat.ActivateCombat(world, command.CharacterID, handle, request.ActionId)
		return
	}
	s.actionService.Activate(world, command.CharacterID, handle, request.ActionId)
}

// Tagged clicks are always consumed, including after selection has ended.
func (s *NetworkCommandSystem) handleCombatAttempt(world *ecs.World, handle types.Handle, command *network.PlayerCommand, click *netproto.MapClick) bool {
	attempt := click.CombatAttempt
	if attempt == nil {
		return false
	}
	service, ok := s.actionService.(CombatActionCommands)
	if !ok || !service.CombatSupported() || !s.consumeCombatRevision(world, handle, command, attempt.StreamEpoch, attempt.RequestRevision) {
		return true
	}
	if click.Button != netproto.MapClickButton_MAP_CLICK_BUTTON_PRIMARY {
		return true
	}
	if s.consumeAdminMapClick(world, handle, command.CharacterID, click) {
		return true
	}
	service.CommitCombat(world, command.CharacterID, handle, click)
	return true
}

func (s *NetworkCommandSystem) rejectIfCombatCommitted(world *ecs.World, handle types.Handle, playerID types.EntityID) bool {
	if !components.CombatCommitted(world, handle) {
		return false
	}
	if s.actionService != nil {
		s.actionService.SendState(world, playerID, handle)
	}
	return true
}
