package game

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
)

// notifyRootInventoryMutation runs under the owning shard lock, immediately
// after each committed inventory operation and before the next command.
func notifyRootInventoryMutation(w *ecs.World, registry contracts.BehaviorRegistry, deps *contracts.ExecutionDeps, rootID types.EntityID) {
	if registry == nil {
		return
	}
	handle := w.GetHandleByEntityID(rootID)
	if handle == types.InvalidHandle || !w.Alive(handle) || ecs.ObjectDestructionPending(w, handle) {
		return
	}
	info, ok := ecs.GetComponent[components.EntityInfo](w, handle)
	if !ok {
		return
	}
	for _, key := range info.Behaviors {
		behavior, found := registry.GetBehavior(key)
		if !found {
			continue
		}
		listener, supports := behavior.(contracts.RootInventoryMutationBehavior)
		if !supports {
			continue
		}
		result, err := listener.OnRootInventoryMutation(&contracts.RootInventoryMutationContext{
			World: w, Handle: handle, EntityID: rootID, EntityType: info.TypeID, Deps: deps,
		})
		if err != nil && deps != nil && deps.Logger != nil {
			deps.Logger.Error("object inventory mutation failed", zap.String("behavior", key), zap.Uint64("entity_id", uint64(rootID)), zap.Error(err))
		}
		if result.StateChanged {
			ecs.MarkObjectBehaviorDirty(w, handle)
		}
	}
}

func broadcastRootInventoryUpdate(w *ecs.World, service *OpenContainerService, rootID types.EntityID) {
	if service == nil {
		return
	}
	handle, found := ecs.GetResource[ecs.InventoryRefIndex](w).Lookup(constt.InventoryGrid, rootID, 0)
	if !found || !w.Alive(handle) {
		return
	}
	container, found := ecs.GetComponent[components.InventoryContainer](w, handle)
	if !found || container.OwnerID != rootID || container.Kind != constt.InventoryGrid || container.Key != 0 {
		return
	}
	service.BroadcastInventoryUpdates(w, 0, []*netproto.InventoryState{buildInventoryStateFromContainer(w, container)})
}
