package game

import (
	"slices"
	"time"

	_const "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors/contracts"
	gameworld "origin/internal/game/world"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
)

const (
	liftPendingTTL          = 15 * time.Second
	liftPointPhantomHalfLen = 1.0
)

type liftRuntimeSender interface {
	SendMiniAlert(entityID types.EntityID, alert *netproto.S2C_MiniAlert)
	SendLiftCarryState(entityID types.EntityID, msg *netproto.S2C_LiftCarryState)
}

type LiftService struct {
	world        *ecs.World
	chunkManager *gameworld.ChunkManager
	eventBus     *eventbus.EventBus
	alerts       liftRuntimeSender
	logger       *zap.Logger
}

var _ systems.LiftCommandService = (*LiftService)(nil)
var _ systems.LiftPlacementFinalizer = (*LiftService)(nil)
var _ systems.LiftCarryFollowCoordinator = (*LiftService)(nil)

func NewLiftService(
	world *ecs.World,
	chunkManager *gameworld.ChunkManager,
	eventBus *eventbus.EventBus,
	alerts liftRuntimeSender,
	logger *zap.Logger,
) *LiftService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &LiftService{
		world:        world,
		chunkManager: chunkManager,
		eventBus:     eventBus,
		alerts:       alerts,
		logger:       logger,
	}
}

func (s *LiftService) IsPlayerCarrying(w *ecs.World, playerHandle types.Handle) bool {
	if s == nil || w == nil || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return false
	}
	_, ok := ecs.GetComponent[components.LiftCarryState](w, playerHandle)
	return ok
}

func (s *LiftService) SendCarryLockedWarning(playerID types.EntityID) {
	s.sendWarning(playerID, "LIFT_ACTION_LOCKED")
}

func (s *LiftService) StartLiftFromContextAction(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	targetID types.EntityID,
	targetHandle types.Handle,
) contracts.BehaviorResult {
	if s == nil || w == nil || w != s.world {
		return contracts.BehaviorResult{OK: false}
	}
	if playerID == 0 || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return contracts.BehaviorResult{OK: false}
	}
	if !s.isLiftableTarget(w, targetHandle) {
		return contracts.BehaviorResult{OK: false}
	}
	if _, hasCollider := ecs.GetComponent[components.Collider](w, targetHandle); !hasCollider {
		// No-collider pickup is handled via interact special path.
		return contracts.BehaviorResult{OK: false}
	}
	if _, ok := ecs.GetComponent[components.LiftCarryState](w, playerHandle); ok {
		return s.warningResult("LIFT_ALREADY_CARRYING")
	}
	if _, ok := ecs.GetComponent[components.LiftedObjectState](w, targetHandle); ok {
		return s.warningResult("LIFT_TARGET_ALREADY_CARRIED")
	}
	if !s.startCarryingObject(w, playerID, playerHandle, targetID, targetHandle) {
		return s.warningResult("LIFT_INVALID_TARGET")
	}
	return contracts.BehaviorResult{OK: true}
}

func (s *LiftService) TryStartNoColliderLiftInteract(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	targetID types.EntityID,
	targetHandle types.Handle,
	interactionType netproto.InteractionType,
) bool {
	if s == nil || w == nil || w != s.world {
		return false
	}
	if playerID == 0 || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return false
	}
	if interactionType != netproto.InteractionType_AUTO && interactionType != netproto.InteractionType_OPEN {
		return false
	}
	if !s.isLiftableTarget(w, targetHandle) {
		return false
	}
	if _, hasCollider := ecs.GetComponent[components.Collider](w, targetHandle); hasCollider {
		return false
	}
	if _, ok := ecs.GetComponent[components.LiftCarryState](w, playerHandle); ok {
		s.sendWarning(playerID, "LIFT_ALREADY_CARRYING")
		return true
	}
	if _, ok := ecs.GetComponent[components.LiftedObjectState](w, targetHandle); ok {
		s.sendWarning(playerID, "LIFT_TARGET_ALREADY_CARRIED")
		return true
	}

	_, hasPlayerCollider := ecs.GetComponent[components.Collider](w, playerHandle)
	playerMov, hasMovement := ecs.GetComponent[components.Movement](w, playerHandle)
	targetTransform, hasTargetTransform := ecs.GetComponent[components.Transform](w, targetHandle)
	if !hasPlayerCollider || !hasMovement || !hasTargetTransform || playerMov.State == _const.StateStunned {
		s.sendWarning(playerID, "LIFT_INVALID_TARGET")
		return true
	}

	s.clearPendingLiftTransitionState(w, playerID, playerHandle, false)
	s.clearPendingInteractionIntents(w, playerID, playerHandle)
	s.breakActiveLink(w, playerID)

	ecs.WithComponent(w, playerHandle, func(col *components.Collider) {
		col.Phantom = &components.PhantomCollider{
			WorldX:     targetTransform.X,
			WorldY:     targetTransform.Y,
			HalfWidth:  liftPointPhantomHalfLen,
			HalfHeight: liftPointPhantomHalfLen,
		}
	})
	ecs.WithComponent(w, playerHandle, func(m *components.Movement) {
		m.SetTargetHandle(targetHandle, int(targetTransform.X), int(targetTransform.Y))
	})
	expireAt := ecs.GetResource[ecs.TimeState](w).UnixMs + liftPendingTTL.Milliseconds()
	ecs.AddComponent(w, playerHandle, components.PendingLiftTransition{
		Mode:           components.LiftTransitionModePickupNoCollider,
		ObjectEntityID: targetID,
		ObjectHandle:   targetHandle,
		TargetX:        targetTransform.X,
		TargetY:        targetTransform.Y,
		PhantomHalfW:   liftPointPhantomHalfLen,
		PhantomHalfH:   liftPointPhantomHalfLen,
		ExpireAtUnixMs: expireAt,
	})
	return true
}

func (s *LiftService) HandleLiftPutDown(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	msg *netproto.C2S_LiftPutDown,
) {
	if s == nil || w == nil || w != s.world || msg == nil || msg.Pos == nil {
		return
	}
	if playerID == 0 || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return
	}

	carry, ok := ecs.GetComponent[components.LiftCarryState](w, playerHandle)
	if !ok {
		s.sendWarning(playerID, "LIFT_PUTDOWN_INVALID")
		return
	}
	if types.EntityID(msg.EntityId) == 0 || types.EntityID(msg.EntityId) != carry.ObjectEntityID {
		s.sendWarning(playerID, "LIFT_PUTDOWN_INVALID")
		return
	}
	objectHandle, ok := s.resolveCarriedObjectHandle(w, carry)
	if !ok {
		s.clearCarryStateForPlayer(w, playerID, playerHandle, false)
		s.sendWarning(playerID, "LIFT_PUTDOWN_INVALID")
		return
	}
	lifted, hasLifted := ecs.GetComponent[components.LiftedObjectState](w, objectHandle)
	if !hasLifted {
		s.clearCarryStateForPlayer(w, playerID, playerHandle, false)
		s.sendWarning(playerID, "LIFT_PUTDOWN_INVALID")
		return
	}

	targetX := float64(msg.Pos.X)
	targetY := float64(msg.Pos.Y)
	coord := types.WorldToChunkCoord(int(targetX), int(targetY), _const.ChunkSize, _const.CoordPerTile)
	chunk := s.chunkManager.GetChunkFast(coord)
	if chunk == nil || chunk.GetState() != types.ChunkStateActive {
		s.sendWarning(playerID, "LIFT_PUTDOWN_INVALID")
		return
	}

	s.clearPendingLiftTransitionState(w, playerID, playerHandle, false)
	s.clearPendingInteractionIntents(w, playerID, playerHandle)
	s.breakActiveLink(w, playerID)

	halfW := liftPointPhantomHalfLen
	halfH := liftPointPhantomHalfLen
	usesObjectCollider := false
	if lifted.HadCollider {
		halfW = lifted.OriginalCollider.HalfWidth
		halfH = lifted.OriginalCollider.HalfHeight
		usesObjectCollider = true
	}

	ecs.WithComponent(w, playerHandle, func(col *components.Collider) {
		col.Phantom = &components.PhantomCollider{
			WorldX:     targetX,
			WorldY:     targetY,
			HalfWidth:  halfW,
			HalfHeight: halfH,
		}
	})
	ecs.WithComponent(w, playerHandle, func(m *components.Movement) {
		m.SetTargetPoint(int(targetX), int(targetY))
	})
	expireAt := ecs.GetResource[ecs.TimeState](w).UnixMs + liftPendingTTL.Milliseconds()
	ecs.AddComponent(w, playerHandle, components.PendingLiftTransition{
		Mode:               components.LiftTransitionModePutDown,
		ObjectEntityID:     carry.ObjectEntityID,
		ObjectHandle:       objectHandle,
		TargetX:            targetX,
		TargetY:            targetY,
		UsesObjectCollider: usesObjectCollider,
		PhantomHalfW:       halfW,
		PhantomHalfH:       halfH,
		ExpireAtUnixMs:     expireAt,
	})
}

func (s *LiftService) FinalizePendingLiftTransition(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	pending components.PendingLiftTransition,
) {
	if s == nil || w == nil || w != s.world || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return
	}
	current, hasPending := ecs.GetComponent[components.PendingLiftTransition](w, playerHandle)
	if !hasPending {
		return
	}
	if current.Mode != pending.Mode || current.ObjectEntityID != pending.ObjectEntityID {
		return
	}

	switch pending.Mode {
	case components.LiftTransitionModePickupNoCollider:
		targetHandle := pending.ObjectHandle
		if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
			targetHandle = w.GetHandleByEntityID(pending.ObjectEntityID)
		}
		if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) || !s.isLiftableTarget(w, targetHandle) {
			s.CancelPendingLiftTransition(w, playerID, playerHandle)
			s.sendWarning(playerID, "LIFT_INVALID_TARGET")
			return
		}
		if _, hasCollider := ecs.GetComponent[components.Collider](w, targetHandle); hasCollider {
			// If collider appeared meanwhile, let the normal path handle future interactions.
			s.CancelPendingLiftTransition(w, playerID, playerHandle)
			return
		}
		if _, ok := ecs.GetComponent[components.LiftCarryState](w, playerHandle); ok {
			s.CancelPendingLiftTransition(w, playerID, playerHandle)
			s.sendWarning(playerID, "LIFT_ALREADY_CARRYING")
			return
		}
		if _, ok := ecs.GetComponent[components.LiftedObjectState](w, targetHandle); ok {
			s.CancelPendingLiftTransition(w, playerID, playerHandle)
			s.sendWarning(playerID, "LIFT_TARGET_ALREADY_CARRIED")
			return
		}
		if !s.startCarryingObject(w, playerID, playerHandle, pending.ObjectEntityID, targetHandle) {
			s.CancelPendingLiftTransition(w, playerID, playerHandle)
			s.sendWarning(playerID, "LIFT_INVALID_TARGET")
			return
		}
		s.clearPendingLiftTransitionState(w, playerID, playerHandle, false)
	case components.LiftTransitionModePutDown:
		s.finalizeLiftPutDown(w, playerID, playerHandle, pending)
	default:
		s.CancelPendingLiftTransition(w, playerID, playerHandle)
	}
}

func (s *LiftService) CancelPendingLiftTransition(w *ecs.World, playerID types.EntityID, playerHandle types.Handle) {
	if s == nil || w == nil || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return
	}
	nowUnixMs := ecs.GetResource[ecs.TimeState](w).UnixMs
	if pending, ok := ecs.GetComponent[components.PendingLiftTransition](w, playerHandle); ok &&
		pending.ExpireAtUnixMs > 0 && nowUnixMs >= pending.ExpireAtUnixMs && playerID != 0 {
		if pending.Mode == components.LiftTransitionModePickupNoCollider {
			s.sendWarning(playerID, "LIFT_PICKUP_TIMEOUT")
		} else if pending.Mode == components.LiftTransitionModePutDown {
			s.sendWarning(playerID, "LIFT_PUTDOWN_TIMEOUT")
		}
	}
	s.clearPendingLiftTransitionState(w, playerID, playerHandle, false)
}

func (s *LiftService) SyncLiftCarryFollow(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, carry components.LiftCarryState) {
	if s == nil || w == nil || w != s.world || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return
	}
	playerTransform, hasPlayerTransform := ecs.GetComponent[components.Transform](w, playerHandle)
	if !hasPlayerTransform {
		return
	}

	objectHandle, ok := s.resolveCarriedObjectHandle(w, carry)
	if !ok {
		s.clearCarryStateForPlayer(w, playerID, playerHandle, true)
		return
	}
	if _, ok := ecs.GetComponent[components.LiftedObjectState](w, objectHandle); !ok {
		s.clearCarryStateForPlayer(w, playerID, playerHandle, true)
		return
	}

	gameworld.RelocateWorldObjectImmediate(
		w,
		s.chunkManager,
		s.eventBus,
		objectHandle,
		gameworld.RelocateWorldObjectImmediateOptions{IsTeleport: false},
		playerTransform.X,
		playerTransform.Y,
		s.logger,
	)

	// Keep cached handle fresh if entity was respawned/re-resolved.
	if carry.ObjectHandle != objectHandle {
		ecs.WithComponent(w, playerHandle, func(state *components.LiftCarryState) {
			state.ObjectHandle = objectHandle
		})
	}
}

func (s *LiftService) ForceDropCarryAtPlayerPosition(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	sendCarryState bool,
) bool {
	if s == nil || w == nil || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return false
	}
	carry, ok := ecs.GetComponent[components.LiftCarryState](w, playerHandle)
	if !ok {
		return false
	}
	playerTransform, hasPlayerTransform := ecs.GetComponent[components.Transform](w, playerHandle)
	if !hasPlayerTransform {
		s.clearCarryStateForPlayer(w, playerID, playerHandle, sendCarryState)
		return false
	}
	objectHandle, ok := s.resolveCarriedObjectHandle(w, carry)
	if !ok {
		s.clearCarryStateForPlayer(w, playerID, playerHandle, sendCarryState)
		return false
	}
	s.restoreCarriedObjectRuntime(w, objectHandle)
	_ = gameworld.RelocateWorldObjectImmediate(
		w,
		s.chunkManager,
		s.eventBus,
		objectHandle,
		gameworld.RelocateWorldObjectImmediateOptions{IsTeleport: false, ForceReindex: true},
		playerTransform.X,
		playerTransform.Y,
		s.logger,
	)
	ecs.RemoveComponent[components.LiftedObjectState](w, objectHandle)
	s.clearCarryStateForPlayer(w, playerID, playerHandle, sendCarryState)
	return true
}

func (s *LiftService) finalizeLiftPutDown(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	pending components.PendingLiftTransition,
) {
	carry, hasCarry := ecs.GetComponent[components.LiftCarryState](w, playerHandle)
	playerTransform, hasPlayerTransform := ecs.GetComponent[components.Transform](w, playerHandle)
	if !hasCarry || !hasPlayerTransform {
		s.clearPendingLiftTransitionState(w, playerID, playerHandle, false)
		s.sendWarning(playerID, "LIFT_PUTDOWN_INVALID")
		return
	}

	objectHandle, ok := s.resolveCarriedObjectHandle(w, carry)
	if !ok {
		s.clearCarryStateForPlayer(w, playerID, playerHandle, true)
		s.clearPendingLiftTransitionState(w, playerID, playerHandle, false)
		s.sendWarning(playerID, "LIFT_PUTDOWN_INVALID")
		return
	}

	if _, ok := ecs.GetComponent[components.LiftedObjectState](w, objectHandle); !ok {
		s.clearCarryStateForPlayer(w, playerID, playerHandle, true)
		s.clearPendingLiftTransitionState(w, playerID, playerHandle, false)
		s.sendWarning(playerID, "LIFT_PUTDOWN_INVALID")
		return
	}

	dropX := pending.TargetX
	dropY := pending.TargetY
	if !pending.UsesObjectCollider {
		dropX = playerTransform.X
		dropY = playerTransform.Y
	}

	coord := types.WorldToChunkCoord(int(dropX), int(dropY), _const.ChunkSize, _const.CoordPerTile)
	chunk := s.chunkManager.GetChunkFast(coord)
	if chunk == nil || chunk.GetState() != types.ChunkStateActive {
		s.clearPendingLiftTransitionState(w, playerID, playerHandle, false)
		s.sendWarning(playerID, "LIFT_PUTDOWN_INVALID")
		return
	}

	s.restoreCarriedObjectRuntime(w, objectHandle)
	if !gameworld.RelocateWorldObjectImmediate(
		w,
		s.chunkManager,
		s.eventBus,
		objectHandle,
		gameworld.RelocateWorldObjectImmediateOptions{IsTeleport: false, ForceReindex: true},
		dropX,
		dropY,
		s.logger,
	) {
		// Keep carrying on failed placement relocation.
		s.disableCarriedObjectRuntime(w, objectHandle, playerID, playerHandle)
		s.clearPendingLiftTransitionState(w, playerID, playerHandle, false)
		s.sendWarning(playerID, "LIFT_PUTDOWN_INVALID")
		return
	}

	ecs.RemoveComponent[components.LiftedObjectState](w, objectHandle)
	s.clearCarryStateForPlayer(w, playerID, playerHandle, true)
	s.clearPendingLiftTransitionState(w, playerID, playerHandle, false)
}

func (s *LiftService) startCarryingObject(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	targetID types.EntityID,
	targetHandle types.Handle,
) bool {
	if !s.isLiftableTarget(w, targetHandle) {
		return false
	}
	playerTransform, hasPlayerTransform := ecs.GetComponent[components.Transform](w, playerHandle)
	_, hasTargetTransform := ecs.GetComponent[components.Transform](w, targetHandle)
	targetInfo, hasTargetInfo := ecs.GetComponent[components.EntityInfo](w, targetHandle)
	if !hasPlayerTransform || !hasTargetTransform || !hasTargetInfo {
		return false
	}

	s.breakAllLinksToTarget(w, targetID)
	s.clearPendingLiftTransitionState(w, playerID, playerHandle, false)
	s.clearPendingInteractionIntents(w, playerID, playerHandle)

	liftedState := components.LiftedObjectState{
		CarrierPlayerID:  playerID,
		CarrierHandle:    playerHandle,
		OriginalIsStatic: targetInfo.IsStatic,
		LiftedAtUnixMs:   ecs.GetResource[ecs.TimeState](w).UnixMs,
	}
	if collider, hasCollider := ecs.GetComponent[components.Collider](w, targetHandle); hasCollider {
		liftedState.HadCollider = true
		liftedState.OriginalCollider = collider
	}
	ecs.AddComponent(w, targetHandle, liftedState)
	s.disableCarriedObjectRuntime(w, targetHandle, playerID, playerHandle)
	ecs.AddComponent(w, playerHandle, components.LiftCarryState{
		ObjectEntityID:  targetID,
		ObjectHandle:    targetHandle,
		StartedAtUnixMs: ecs.GetResource[ecs.TimeState](w).UnixMs,
	})

	if !gameworld.RelocateWorldObjectImmediate(
		w,
		s.chunkManager,
		s.eventBus,
		targetHandle,
		gameworld.RelocateWorldObjectImmediateOptions{IsTeleport: false, ForceReindex: true},
		playerTransform.X,
		playerTransform.Y,
		s.logger,
	) {
		// Roll back carry markers if relocation fails.
		s.restoreCarriedObjectRuntime(w, targetHandle)
		ecs.RemoveComponent[components.LiftedObjectState](w, targetHandle)
		ecs.RemoveComponent[components.LiftCarryState](w, playerHandle)
		return false
	}

	s.sendCarryState(playerID, true, targetID)
	return true
}

func (s *LiftService) disableCarriedObjectRuntime(w *ecs.World, objectHandle types.Handle, playerID types.EntityID, playerHandle types.Handle) {
	ecs.WithComponent(w, objectHandle, func(col *components.Collider) {
		col.Phantom = nil
		col.Layer = 0
		col.Mask = 0
	})
	ecs.WithComponent(w, objectHandle, func(info *components.EntityInfo) {
		info.IsStatic = false
	})
	ecs.WithComponent(w, objectHandle, func(lifted *components.LiftedObjectState) {
		lifted.CarrierPlayerID = playerID
		lifted.CarrierHandle = playerHandle
	})
}

func (s *LiftService) restoreCarriedObjectRuntime(w *ecs.World, objectHandle types.Handle) {
	lifted, hasLifted := ecs.GetComponent[components.LiftedObjectState](w, objectHandle)
	if !hasLifted {
		return
	}
	if lifted.HadCollider {
		restored := lifted.OriginalCollider
		restored.Phantom = nil
		ecs.AddComponent(w, objectHandle, restored)
	} else {
		ecs.RemoveComponent[components.Collider](w, objectHandle)
	}
	ecs.WithComponent(w, objectHandle, func(info *components.EntityInfo) {
		info.IsStatic = lifted.OriginalIsStatic
	})
	ecs.WithComponent(w, objectHandle, func(state *components.ObjectInternalState) {
		state.IsDirty = true
	})
}

func (s *LiftService) clearCarryStateForPlayer(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, sendPacket bool) {
	ecs.RemoveComponent[components.LiftCarryState](w, playerHandle)
	if sendPacket {
		s.sendCarryState(playerID, false, 0)
	}
}

func (s *LiftService) clearPendingLiftTransitionState(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, keepPhantom bool) {
	ecs.RemoveComponent[components.PendingLiftTransition](w, playerHandle)
	if !keepPhantom {
		ecs.WithComponent(w, playerHandle, func(col *components.Collider) {
			col.Phantom = nil
		})
	}
	if playerID != 0 {
		ecs.GetResource[ecs.LinkState](w).ClearIntent(playerID)
	}
}

func (s *LiftService) clearPendingInteractionIntents(w *ecs.World, playerID types.EntityID, playerHandle types.Handle) {
	systems.ClearPlayerInteractionIntents(w, playerHandle, playerID)
}

func (s *LiftService) breakActiveLink(w *ecs.World, playerID types.EntityID) {
	if playerID == 0 {
		return
	}
	if _, _, err := ecs.BreakLinkForPlayer(w, playerID, ecs.LinkBreakClosed); err != nil {
		s.logger.Warn("LiftService: failed to break active link", zap.Error(err), zap.Uint64("player_id", uint64(playerID)))
	}
}

func (s *LiftService) breakAllLinksToTarget(w *ecs.World, targetID types.EntityID) {
	if targetID == 0 {
		return
	}
	linkState := ecs.GetResource[ecs.LinkState](w)
	players := linkState.PlayersByTarget[targetID]
	if len(players) == 0 {
		return
	}
	playerIDs := make([]types.EntityID, 0, len(players))
	for playerID := range players {
		playerIDs = append(playerIDs, playerID)
	}
	slices.Sort(playerIDs)
	for _, playerID := range playerIDs {
		if _, _, err := ecs.BreakLinkForPlayer(w, playerID, ecs.LinkBreakClosed); err != nil {
			s.logger.Warn("LiftService: failed to break link for lift target",
				zap.Uint64("player_id", uint64(playerID)),
				zap.Uint64("target_id", uint64(targetID)),
				zap.Error(err),
			)
		}
	}
}

func (s *LiftService) isLiftableTarget(w *ecs.World, targetHandle types.Handle) bool {
	if w == nil || targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
		return false
	}
	info, hasInfo := ecs.GetComponent[components.EntityInfo](w, targetHandle)
	if !hasInfo {
		return false
	}
	for _, behaviorKey := range info.Behaviors {
		if behaviorKey == "lift" {
			return true
		}
	}
	return false
}

func (s *LiftService) resolveCarriedObjectHandle(w *ecs.World, carry components.LiftCarryState) (types.Handle, bool) {
	if w == nil || carry.ObjectEntityID == 0 {
		return types.InvalidHandle, false
	}
	objectHandle := carry.ObjectHandle
	if objectHandle == types.InvalidHandle || !w.Alive(objectHandle) {
		objectHandle = w.GetHandleByEntityID(carry.ObjectEntityID)
	}
	if objectHandle == types.InvalidHandle || !w.Alive(objectHandle) {
		return types.InvalidHandle, false
	}
	return objectHandle, true
}

func (s *LiftService) sendCarryState(playerID types.EntityID, active bool, entityID types.EntityID) {
	if s == nil || s.alerts == nil || playerID == 0 {
		return
	}
	s.alerts.SendLiftCarryState(playerID, &netproto.S2C_LiftCarryState{
		Active:   active,
		EntityId: uint64(entityID),
	})
}

func (s *LiftService) sendWarning(playerID types.EntityID, reasonCode string) {
	if s == nil || s.alerts == nil || playerID == 0 || reasonCode == "" {
		return
	}
	s.alerts.SendMiniAlert(playerID, &netproto.S2C_MiniAlert{
		Severity:   netproto.AlertSeverity_ALERT_SEVERITY_WARNING,
		ReasonCode: reasonCode,
		TtlMs:      1500,
	})
}

func (s *LiftService) warningResult(reasonCode string) contracts.BehaviorResult {
	return contracts.BehaviorResult{
		OK:          false,
		UserVisible: true,
		ReasonCode:  reasonCode,
		Severity:    contracts.BehaviorAlertSeverityWarning,
	}
}
