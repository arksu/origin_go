package inventory

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

// ResolveItemProgress projects existing authoritative state in object behavior
// order. It runs only while building a snapshot under the owning shard lock.
func ResolveItemProgress(w *ecs.World, registry contracts.BehaviorRegistry, container components.InventoryContainer, item components.InvItem) *netproto.ItemProgress {
	if w == nil || registry == nil {
		return nil
	}
	handle := w.GetHandleByEntityID(container.OwnerID)
	if handle == types.InvalidHandle || !w.Alive(handle) || ecs.ObjectDestructionPending(w, handle) {
		return nil
	}
	info, found := ecs.GetComponent[components.EntityInfo](w, handle)
	if !found || len(info.Behaviors) == 0 {
		return nil
	}
	var ctx *contracts.InventoryItemProgressContext
	for _, key := range info.Behaviors {
		behavior, found := registry.GetBehavior(key)
		if !found {
			continue
		}
		provider, supported := behavior.(contracts.InventoryItemProgressProvider)
		if !supported {
			continue
		}
		if ctx == nil {
			ctx = &contracts.InventoryItemProgressContext{
				World: w, Handle: handle, EntityID: container.OwnerID, EntityType: info.TypeID,
				Container: container, Item: item,
			}
		}
		progress := provider.InventoryItemProgress(ctx)
		if validItemProgress(progress) {
			return progress
		}
	}
	return nil
}

func validItemProgress(progress *netproto.ItemProgress) bool {
	if progress == nil || progress.DurationSeconds <= 0 {
		return false
	}
	switch timing := progress.Timing.(type) {
	case *netproto.ItemProgress_CompletionRuntimeSeconds:
		return timing != nil && timing.CompletionRuntimeSeconds >= 0
	case *netproto.ItemProgress_PausedRemainingSeconds:
		return timing != nil && timing.PausedRemainingSeconds >= 0 && timing.PausedRemainingSeconds <= progress.DurationSeconds
	default:
		return false
	}
}
