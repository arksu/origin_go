package game

import (
	"fmt"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
	"sort"
	"time"
)

func CombatExecutionSnapshot(w *ecs.World, actor types.Handle) *netproto.CombatExecutionState {
	state, ok := ecs.GetComponent[components.CombatState](w, actor)
	if !ok || !w.Alive(actor) {
		return nil
	}
	timing := ecs.GetResource[ecs.TimeState](w)
	snapshot := &netproto.CombatExecutionState{Generation: fmt.Sprintf("%d:%d", w.Layer, actor), Revision: state.Revision, Phase: "idle", ServerTimeMs: timing.UnixMs}
	execution := state.Execution
	if execution == nil {
		return snapshot
	}
	duration := execution.RecoveryEnd.Sub(execution.StartedAt).Milliseconds()
	elapsed := max(int64(0), min(duration, timing.Now.Sub(execution.StartedAt).Milliseconds()))
	snapshot.ExecutionId = execution.ID
	snapshot.ActionId = execution.ActionID
	snapshot.Phase = "windup"
	if execution.StrikeResolved {
		snapshot.Phase = "recovery"
	}
	snapshot.LockedDirection = &netproto.CombatDirection{X: execution.Direction.X, Y: execution.Direction.Y}
	snapshot.ElapsedMs = float64(elapsed)
	snapshot.DurationMs = float64(duration)
	snapshot.StrikeAtMs = timing.UnixMs + execution.StrikeAt.Sub(timing.Now).Milliseconds()
	snapshot.RecoveryEndMs = timing.UnixMs + execution.RecoveryEnd.Sub(timing.Now).Milliseconds()
	snapshot.Range = execution.Weapon.Range
	snapshot.SectorAngleDegrees = execution.AngleDegrees
	snapshot.StartEventSequence = execution.StartEventSequence
	return snapshot
}
func CombatTargetSnapshot(w *ecs.World, handle types.Handle) *netproto.CombatTargetState {
	target, ok := ecs.GetComponent[components.CombatTestTarget](w, handle)
	if !ok || !target.Receiver || !w.Alive(handle) {
		return nil
	}
	return &netproto.CombatTargetState{Generation: fmt.Sprintf("%d:%d", w.Layer, handle), Revision: target.Revision, Hp: target.HP, MaxHp: target.MaxHP, Depleted: target.HP <= 0}
}
func combatVisible(w *ecs.World, observer, target types.Handle) bool {
	if observer == target {
		return true
	}
	visibility := ecs.GetResource[ecs.VisibilityState](w)
	visibility.Mu.RLock()
	defer visibility.Mu.RUnlock()
	_, visible := visibility.ObserversByVisibleTarget[target][observer]
	return visible
}
func combatObservers(w *ecs.World, target types.Handle, includeSelf bool) []types.Handle {
	visibility := ecs.GetResource[ecs.VisibilityState](w)
	visibility.Mu.RLock()
	observers := make([]types.Handle, 0, len(visibility.ObserversByVisibleTarget[target])+1)
	for observer := range visibility.ObserversByVisibleTarget[target] {
		if observer != target {
			observers = append(observers, observer)
		}
	}
	visibility.Mu.RUnlock()
	if includeSelf {
		observers = append(observers, target)
	}
	return observers
}
func (shard *Shard) sendCombat(observer types.Handle, build func(uint32) *netproto.ServerMessage) {
	id, alive := shard.world.GetExternalID(observer)
	if !alive {
		return
	}
	shard.ClientsMu.RLock()
	defer shard.ClientsMu.RUnlock()
	client := shard.Clients[id]
	if client == nil || !client.InWorld.Load() {
		return
	}
	sendCombatMessage(client, build(client.StreamEpoch.Load()), shard.logger)
}
func sendCombatMessage(client *network.Client, message *netproto.ServerMessage, logger *zap.Logger) {
	encoded, err := proto.Marshal(message)
	if err != nil {
		logger.Error("Unable to encode combat state", zap.Error(err))
		return
	}
	client.SendCritical(encoded)
}
func (shard *Shard) publishCombatState(actor types.Handle) {
	snapshot := CombatExecutionSnapshot(shard.world, actor)
	if snapshot == nil {
		return
	}
	id, _ := shard.world.GetExternalID(actor)
	for _, observer := range combatObservers(shard.world, actor, true) {
		shard.sendCombat(observer, func(epoch uint32) *netproto.ServerMessage {
			return &netproto.ServerMessage{Payload: &netproto.ServerMessage_CombatState{CombatState: &netproto.S2C_CombatState{EntityId: uint64(id), StreamEpoch: epoch, State: snapshot}}}
		})
	}
	shard.sendCombatOwner(actor)
}
func (shard *Shard) sendCombatOwner(actor types.Handle) {
	state, ok := ecs.GetComponent[components.CombatState](shard.world, actor)
	if !ok {
		return
	}
	timing := ecs.GetResource[ecs.TimeState](shard.world)
	keys := make([]string, 0, len(state.Cooldowns))
	for key := range state.Cooldowns {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	cooldowns := make([]*netproto.CombatCooldown, 0, len(keys))
	for _, key := range keys {
		cooldowns = append(cooldowns, &netproto.CombatCooldown{ActionId: key, ReadyAtMs: timing.UnixMs + state.Cooldowns[key].Sub(timing.Now).Milliseconds()})
	}
	shard.sendCombat(actor, func(epoch uint32) *netproto.ServerMessage {
		return &netproto.ServerMessage{Payload: &netproto.ServerMessage_CombatOwnerState{CombatOwnerState: &netproto.S2C_CombatOwnerState{Generation: fmt.Sprintf("%d:%d", shard.world.Layer, actor), Revision: state.Revision, StreamEpoch: epoch, ServerTimeMs: timing.UnixMs, Cooldowns: cooldowns}}}
	})
}
func (shard *Shard) publishCombatTarget(target types.Handle) {
	snapshot := CombatTargetSnapshot(shard.world, target)
	if snapshot == nil {
		return
	}
	id, _ := shard.world.GetExternalID(target)
	for _, observer := range combatObservers(shard.world, target, false) {
		shard.sendCombat(observer, func(epoch uint32) *netproto.ServerMessage {
			return &netproto.ServerMessage{Payload: &netproto.ServerMessage_CombatTarget{CombatTarget: &netproto.S2C_CombatTarget{EntityId: uint64(id), StreamEpoch: epoch, State: snapshot}}}
		})
	}
}
func combatResultSnapshot(w *ecs.World, observer types.Handle, result CombatResult) *netproto.S2C_CombatResult {
	message := &netproto.S2C_CombatResult{EntityId: uint64(result.ActorID), Generation: fmt.Sprintf("%d:%d", w.Layer, result.ActorHandle), ExecutionId: result.ExecutionID, EventSequence: result.EventSequence, ServerTimeMs: result.TimeMs}
	for _, hit := range result.Hits {
		if !combatVisible(w, observer, hit.TargetIncarnation) {
			continue
		}
		target := CombatTargetSnapshot(w, hit.TargetIncarnation)
		if target == nil {
			continue
		}
		message.Hits = append(message.Hits, &netproto.CombatHitResult{EventSequence: hit.EventSequence, TargetId: uint64(hit.TargetID), Target: target, RawDamage: hit.RawDamage, Damage: hit.Damage})
	}
	message.Hit = len(message.Hits) > 0
	return message
}
func (shard *Shard) publishCombatResult(result CombatResult) {
	for _, observer := range combatObservers(shard.world, result.ActorHandle, true) {
		message := combatResultSnapshot(shard.world, observer, result)
		shard.sendCombat(observer, func(epoch uint32) *netproto.ServerMessage {
			message.StreamEpoch = epoch
			return &netproto.ServerMessage{Payload: &netproto.ServerMessage_CombatResult{CombatResult: message}}
		})
	}
}
func (shard *Shard) removeCombatFixture(target types.Handle) {
	id, ok := shard.world.GetExternalID(target)
	if !ok {
		return
	}
	for _, observer := range combatObservers(shard.world, target, false) {
		shard.sendCombat(observer, func(epoch uint32) *netproto.ServerMessage {
			return &netproto.ServerMessage{Payload: &netproto.ServerMessage_ObjectDespawn{ObjectDespawn: &netproto.S2C_ObjectDespawn{EntityId: uint64(id), StreamEpoch: epoch}}}
		})
	}
	visibility := ecs.GetResource[ecs.VisibilityState](shard.world)
	visibility.Mu.Lock()
	defer visibility.Mu.Unlock()
	for observer := range visibility.ObserversByVisibleTarget[target] {
		state := visibility.VisibleByObserver[observer]
		delete(state.Known, target)
		state.NextUpdateTime = time.Time{}
		visibility.VisibleByObserver[observer] = state
	}
	delete(visibility.ObserversByVisibleTarget, target)
}
