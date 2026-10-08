package inventory

import (
	"encoding/json"
	"fmt"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/types"

	"go.uber.org/zap"
)

type InventorySaver struct {
	logger *zap.Logger
}

func NewInventorySaver(logger *zap.Logger) *InventorySaver {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &InventorySaver{
		logger: logger,
	}
}

func (is *InventorySaver) SerializeInventories(
	w interface{},
	characterID types.EntityID,
	handle types.Handle,
) []systems.InventorySnapshot {
	world, ok := w.(*ecs.World)
	if !ok || world == nil {
		is.logger.Error("InventorySaver requires an ECS world")
		return nil
	}

	result, err := is.SerializeInventoriesStrict(world, characterID, handle)
	if err != nil {
		is.logger.Error("Failed to serialize player inventories",
			zap.Uint64("character_id", uint64(characterID)),
			zap.Error(err))
		return nil
	}
	return result
}

// SerializeInventoriesStrict returns a complete player root-inventory snapshot
// or an error. Durable world transfers use it so malformed JSON can never be
// committed as a partial player side of a transaction.
func (is *InventorySaver) SerializeInventoriesStrict(
	world *ecs.World,
	characterID types.EntityID,
	handle types.Handle,
) ([]systems.InventorySnapshot, error) {
	if world == nil {
		return nil, fmt.Errorf("serialize inventories: world is nil")
	}
	result := make([]systems.InventorySnapshot, 0)

	owner, hasOwner := ecs.GetComponent[components.InventoryOwner](world, handle)
	if !hasOwner {
		return result, nil
	}

	var seenRoots inventoryTreeVisited[ecs.InventoryRefKey]
	for _, link := range owner.Inventories {
		if link.OwnerID != characterID {
			continue // Nested links are embedded beneath their actual root item.
		}
		if !world.Alive(link.Handle) {
			return nil, ErrInvalidInventoryTree
		}
		container, hasContainer := ecs.GetComponent[components.InventoryContainer](world, link.Handle)
		if !hasContainer || container.OwnerID != characterID || container.Kind != link.Kind || container.Key != link.Key ||
			!seenRoots.add(ecs.InventoryRefKey{Kind: link.Kind, OwnerID: link.OwnerID, Key: link.Key}) {
			return nil, ErrInvalidInventoryTree
		}

		snapshot, err := is.serializeContainer(world, characterID, container)
		if err != nil {
			return nil, err
		}
		result = append(result, snapshot)
	}

	return result, nil
}

func (is *InventorySaver) serializeContainer(
	world *ecs.World,
	characterID types.EntityID,
	container components.InventoryContainer,
) (systems.InventorySnapshot, error) {
	invData, err := SerializeInventoryTree(world, container)
	if err != nil {
		return systems.InventorySnapshot{}, err
	}

	data, err := json.Marshal(invData)
	if err != nil {
		return systems.InventorySnapshot{}, fmt.Errorf(
			"serialize inventory owner=%d kind=%d key=%d: %w",
			characterID,
			container.Kind,
			container.Key,
			err,
		)
	}

	return systems.InventorySnapshot{
		CharacterID:  int64(characterID),
		Kind:         int16(container.Kind),
		InventoryKey: int16(container.Key),
		Data:         data,
		Version:      int(container.Version),
	}, nil
}
