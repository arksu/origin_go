package inventory

import (
	"fmt"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/types"

	"go.uber.org/zap"
)

// inventoryContainerSnapshot is a deep copy because InventoryContainer.Items
// is a slice. It allows a failed durable transfer to leave ECS exactly as it
// was before its temporary mutation.
type inventoryContainerSnapshot struct {
	handle    types.Handle
	container components.InventoryContainer
}

func snapshotInventoryContainer(
	w *ecs.World,
	handle types.Handle,
) (inventoryContainerSnapshot, error) {
	container, ok := ecs.GetComponent[components.InventoryContainer](w, handle)
	if !ok {
		return inventoryContainerSnapshot{}, fmt.Errorf("inventory container is unavailable")
	}
	container.Items = append([]components.InvItem(nil), container.Items...)
	return inventoryContainerSnapshot{handle: handle, container: container}, nil
}

func (snapshot inventoryContainerSnapshot) restore(w *ecs.World) {
	if w == nil || snapshot.handle == types.InvalidHandle || !w.Alive(snapshot.handle) {
		return
	}
	restored := snapshot.container
	restored.Items = append([]components.InvItem(nil), restored.Items...)
	ecs.MutateComponent[components.InventoryContainer](w, snapshot.handle, func(c *components.InventoryContainer) bool {
		*c = restored
		return true
	})
}

func (s *InventoryOperationService) atomicTransferPersister() (AtomicDroppedItemTransferPersister, error) {
	if s == nil || s.persister == nil {
		return nil, fmt.Errorf("drop persistence is not configured")
	}
	persister, ok := s.persister.(AtomicDroppedItemTransferPersister)
	if !ok {
		return nil, fmt.Errorf("drop persistence does not support atomic transfers")
	}
	return persister, nil
}

func (s *InventoryOperationService) serializePlayerInventoriesForTransfer(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
) ([]systems.InventorySnapshot, error) {
	if s == nil || s.inventorySaver == nil {
		return nil, fmt.Errorf("inventory snapshot serializer is not configured")
	}
	inventories, err := s.inventorySaver.SerializeInventoriesStrict(w, playerID, playerHandle)
	if err != nil {
		return nil, err
	}
	if len(inventories) == 0 {
		return nil, fmt.Errorf("player %d has no root inventories to persist", playerID)
	}
	return inventories, nil
}

// bumpPlayerRootInventoryForNestedMutation advances the version of the root
// JSON row that embeds a modified nested container. Without it, an older async
// save with the same root version could overwrite a committed transfer.
func bumpPlayerRootInventoryForNestedMutation(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	nestedContainerHandle types.Handle,
) (*inventoryContainerSnapshot, *ContainerInfo, error) {
	nested, ok := ecs.GetComponent[components.InventoryContainer](w, nestedContainerHandle)
	if !ok {
		return nil, nil, fmt.Errorf("modified inventory container is unavailable")
	}
	if nested.OwnerID == playerID {
		return nil, nil, nil
	}

	owner, hasOwner := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
	if !hasOwner {
		return nil, nil, fmt.Errorf("player %d has no inventory owner", playerID)
	}

	for _, link := range owner.Inventories {
		if link.OwnerID != playerID || !w.Alive(link.Handle) {
			continue
		}
		root, hasRoot := ecs.GetComponent[components.InventoryContainer](w, link.Handle)
		if !hasRoot {
			continue
		}
		for _, item := range root.Items {
			if item.ItemID != nested.OwnerID {
				continue
			}

			before, err := snapshotInventoryContainer(w, link.Handle)
			if err != nil {
				return nil, nil, err
			}
			ecs.MutateComponent[components.InventoryContainer](w, link.Handle, func(c *components.InventoryContainer) bool {
				c.Version++
				return true
			})
			updated, _ := ecs.GetComponent[components.InventoryContainer](w, link.Handle)
			return &before, &ContainerInfo{
				Handle:    link.Handle,
				Container: &updated,
				Owner:     &owner,
			}, nil
		}
	}

	return nil, nil, fmt.Errorf("nested inventory owner %d has no player root", nested.OwnerID)
}

func appendUpdatedContainerIfMissing(containers []*ContainerInfo, candidate *ContainerInfo) []*ContainerInfo {
	if candidate == nil {
		return containers
	}
	for _, existing := range containers {
		if existing != nil && existing.Handle == candidate.Handle {
			return containers
		}
	}
	return append(containers, candidate)
}

// cleanupUncommittedDroppedEntities removes only the temporary dropped root.
// It intentionally preserves a pre-existing nested inventory with the same
// item ID while a full container-item drop is being prepared.
func cleanupUncommittedDroppedEntities(w *ecs.World, entityIDs []types.EntityID, logger *zap.Logger) {
	for _, entityID := range entityIDs {
		handle := w.GetHandleByEntityID(entityID)
		if handle == types.InvalidHandle {
			continue
		}
		deleteDroppedEntityFromECS(w, entityID, handle, false, logger)
	}
}
