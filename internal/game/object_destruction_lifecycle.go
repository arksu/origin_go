package game

import (
	"sort"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/playerstate"
	"origin/internal/types"

	"go.uber.org/zap"
)

// quarantineDestroyedObject hides the dead source while keeping its inventories
// available exclusively to the background snapshot. The owning shard is locked.
func (s *Shard) quarantineDestroyedObject(target types.Handle) {
	w := s.world
	id, _ := w.GetExternalID(target)
	info, _ := ecs.GetComponent[components.EntityInfo](w, target)
	position, _ := ecs.GetComponent[components.Transform](w, target)
	ref, _ := ecs.GetComponent[components.ChunkRef](w, target)
	if chunk := s.chunkManager.GetChunkFast(types.ChunkCoord{X: ref.CurrentChunkX, Y: ref.CurrentChunkY}); chunk != nil {
		if info.IsStatic {
			chunk.Spatial().RemoveStatic(target, int(position.X), int(position.Y))
		} else {
			chunk.Spatial().RemoveDynamic(target, int(position.X), int(position.Y))
		}
	}
	ecs.RemoveComponent[components.Collider](w, target)
	ecs.RemoveComponent[components.StationState](w, target)
	ecs.WithComponent(w, target, func(info *components.EntityInfo) { info.Behaviors = nil })
	ecs.CancelBehaviorTicksByEntityID(w, id)
	s.cancelDestroyedObjectReferences(target)
	if lifted, exists := ecs.GetComponent[components.LiftedObjectState](w, target); exists {
		if s.liftService != nil && w.Alive(lifted.CarrierHandle) {
			s.liftService.clearCarryStateForPlayer(w, lifted.CarrierPlayerID, lifted.CarrierHandle, true)
			s.liftService.clearPendingLiftTransitionState(w, lifted.CarrierPlayerID, lifted.CarrierHandle, false)
			s.liftService.reconcileMovementModeForCarry(w, lifted.CarrierHandle)
		}
		ecs.RemoveComponent[components.LiftedObjectState](w, target)
	}
	links := ecs.GetResource[ecs.LinkState](w)
	players := make([]types.EntityID, 0, len(links.PlayersByTarget[id]))
	for player := range links.PlayersByTarget[id] {
		players = append(players, player)
	}
	sort.Slice(players, func(i, j int) bool { return players[i] < players[j] })
	for _, player := range players {
		_, _, _ = ecs.BreakLinkForPlayer(w, player, ecs.LinkBreakDespawn)
	}
	open := ecs.GetResource[ecs.OpenContainerState](w)
	players = players[:0]
	for player := range open.PlayersByRoot[id] {
		players = append(players, player)
	}
	sort.Slice(players, func(i, j int) bool { return players[i] < players[j] })
	for _, player := range players {
		for _, ref := range open.CloseAllForPlayer(player) {
			s.SendContainerClosed(player, &netproto.InventoryRef{Kind: netproto.InventoryKind(ref.Kind), OwnerId: uint64(ref.OwnerID), InventoryKey: ref.Key})
		}
	}
	invalidateEntityVisibility(w, info.Layer, target, id, s.eventBus)
}

// cancelDestroyedObjectReferences visits only actors referring to this exact
// generation. Revalidation keeps an older approach from canceling newer routes.
func (s *Shard) cancelDestroyedObjectReferences(target types.Handle) {
	w := s.world
	index := ecs.GetResource[objectTargetReferences](w)
	links := ecs.GetResource[ecs.LinkState](w)
	actors := make([]types.Handle, 0, len(index.byTarget[target])+len(links.IntentPlayersByTarget[target]))
	for actor := range index.byTarget[target] {
		actors = append(actors, actor)
	}
	for player := range links.IntentPlayersByTarget[target] {
		actors = append(actors, w.GetHandleByEntityID(player))
	}
	sort.Slice(actors, func(i, j int) bool { return actors[i] < actors[j] })
	for i, actor := range actors {
		if i > 0 && actor == actors[i-1] || !w.Alive(actor) {
			continue
		}
		playerID, exists := w.GetExternalID(actor)
		if !exists || w.GetHandleByEntityID(playerID) != actor {
			continue
		}
		pendingLift, hasLift := ecs.GetComponent[components.PendingLiftTransition](w, actor)
		liftMatches := hasLift && pendingLift.ObjectHandle == target
		activeAction, hasAction := ecs.GetComponent[components.ActiveGameAction](w, actor)
		liftOwnsAction := liftMatches && (!hasAction || pendingLift.ActionGeneration == activeAction.Generation)
		movement, hasMovement := ecs.GetComponent[components.Movement](w, actor)
		moveMatches := hasMovement && movement.TargetHandle == target
		if liftOwnsAction && pendingLift.Mode == components.LiftTransitionModePutDown && hasMovement &&
			movement.TargetType == constt.TargetPoint && movement.TargetX == float64(int(pendingLift.TargetX)) &&
			movement.TargetY == float64(int(pendingLift.TargetY)) {
			moveMatches = true
		}
		if moveMatches {
			// This preserves independent stun and schedules the existing movement
			// pass to publish the authoritative stop even after TransformUpdate.
			playerstate.StopMovement(w, actor)
		}
		if hasAction &&
			(activeAction.TargetHandle == target || liftOwnsAction && pendingLift.ActionGeneration != 0) {
			if s.actionService != nil {
				s.actionService.Cancel(w, playerID, actor)
			}
		}
		if pending, ok := ecs.GetComponent[components.PendingContextAction](w, actor); ok && pending.TargetHandle == target {
			ecs.RemoveComponent[components.PendingContextAction](w, actor)
		}
		if pending, ok := ecs.GetComponent[components.PendingInteraction](w, actor); ok && pending.TargetHandle == target {
			ecs.RemoveComponent[components.PendingInteraction](w, actor)
		}
		if active, ok := ecs.GetComponent[components.ActiveCyclicAction](w, actor); ok && active.TargetHandle == target && s.contextActions != nil {
			s.contextActions.cancelActiveCyclicAction(playerID, actor, "object_destroyed")
		}
		if pending, ok := ecs.GetComponent[components.PendingLiftTransition](w, actor); ok && pending.ObjectHandle == target {
			// Clear only this transition. The general lift cancel helper also
			// clears link intent, which could already point at another object.
			clearPhantom := liftTransitionOwnsPhantom(w, actor, pending)
			ecs.RemoveComponent[components.PendingLiftTransition](w, actor)
			if clearPhantom {
				ecs.WithComponent(w, actor, func(collider *components.Collider) { collider.Phantom = nil })
			}
			if s.liftService != nil {
				s.liftService.finishPendingAction(w, playerID, actor, pending, false, "OBJECT_DESTROYED")
			}
		}
		if intent, ok := links.IntentByPlayer[playerID]; ok && intent.TargetHandle == target {
			links.ClearIntent(playerID)
		}
	}
}

// drainObjectDestruction does not hold shard lock while waiting for I/O workers.
// Each completed page is still applied under the lock before ordinary saves.
func (s *Shard) drainObjectDestruction() {
	s.drainObjectDestructionFor(objectDestructionShutdownTimeout)
}

func (s *Shard) drainObjectDestructionFor(timeout time.Duration) {
	if s.objectDestruction == nil && s.chunkManager == nil {
		return
	}
	started := time.Now()
	deadline := started.Add(timeout)
	s.mu.Lock()
	baseNow := ecs.GetResource[ecs.TimeState](s.world).Now
	s.mu.Unlock()
	for {
		s.mu.Lock()
		// Shutdown retries use an advancing time while the regular game loop has
		// stopped. This does not advance the world's persisted runtime seconds.
		timeState := ecs.GetResource[ecs.TimeState](s.world)
		previous := timeState.Now
		timeState.Now = baseNow.Add(time.Since(started))
		pending := 0
		if s.objectDestruction != nil {
			s.objectDestruction.Update()
			pending = s.objectDestruction.PendingCount()
		}
		expired := 0
		if s.chunkManager != nil {
			s.chunkManager.UpdateExpiredDrops(timeState.Now)
			expired = s.chunkManager.PendingExpiredDrops()
		}
		timeState.Now = previous
		s.mu.Unlock()
		if pending == 0 && expired == 0 {
			return
		}
		if time.Now().After(deadline) {
			s.logger.Error("Shutdown object persistence incomplete", zap.Int("destructions", pending), zap.Int("expired_drops", expired), zap.Error(ErrObjectDestructionShutdown))
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
