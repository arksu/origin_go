package world

import (
	"encoding/json"
	"fmt"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

// DroppedItemData is the JSON structure stored in object.data for dropped items.
type DroppedItemData struct {
	HasInventory    bool   `json:"has_inventory"`
	ContainedItemID uint64 `json:"contained_item_id"`
	DropTime        int64  `json:"drop_time"`
	DropperID       uint64 `json:"dropper_id"`
	ItemTypeID      uint32 `json:"item_type_id"`
	ItemResource    string `json:"item_resource"`
	ItemQuality     uint32 `json:"item_quality"`
	ItemQuantity    uint32 `json:"item_quantity"`
	ItemW           uint8  `json:"item_w"`
	ItemH           uint8  `json:"item_h"`
}

// ObjectFactory builds and serializes world objects using object definitions.
type ObjectFactory struct {
	registry     *objectdefs.Registry
	itemRegistry *itemdefs.Registry
}

// NewObjectFactory creates a factory backed by the given object definitions registry.
func NewObjectFactory(registry *objectdefs.Registry) *ObjectFactory {
	return &ObjectFactory{registry: registry}
}

// SetItemRegistry sets the item definitions registry for resolving dropped item resources.
func (f *ObjectFactory) SetItemRegistry(r *itemdefs.Registry) {
	f.itemRegistry = r
}

// Registry returns the underlying object definitions registry.
func (f *ObjectFactory) Registry() *objectdefs.Registry {
	return f.registry
}

// Build creates an ECS entity from a raw database object using its definition.
func (f *ObjectFactory) Build(w *ecs.World, raw *repository.Object) (types.Handle, error) {
	if raw.TypeID == constt.DroppedItemTypeID {
		return f.buildDroppedItem(w, raw)
	}

	def, ok := f.registry.GetByID(raw.TypeID)
	if !ok {
		return types.InvalidHandle, fmt.Errorf("%w: type_id=%d", ErrDefNotFound, raw.TypeID)
	}

	h := w.Spawn(types.EntityID(raw.ID), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.CreateTransform(raw.X, raw.Y, int(raw.Heading.Int16)))

		ecs.AddComponent(w, h, components.EntityInfo{
			TypeID:    uint32(def.DefID),
			Behaviors: def.Behavior,
			IsStatic:  def.IsStatic,
			Region:    raw.Region,
			Layer:     raw.Layer,
		})

		// Collider from definition
		if def.Components != nil && def.Components.Collider != nil {
			c := def.Components.Collider
			ecs.AddComponent(w, h, components.Collider{
				HalfWidth:  c.W / 2.0,
				HalfHeight: c.H / 2.0,
				Layer:      c.Layer,
				Mask:       c.Mask,
			})
		}

		// Appearance: base resource from definition
		ecs.AddComponent(w, h, components.Appearance{
			Resource: def.Resource,
		})
	})
	if h == types.InvalidHandle {
		return types.InvalidHandle, ErrEntitySpawnFailed
	}

	return h, nil
}

// buildDroppedItem creates an ECS entity for a dropped item loaded from DB.
func (f *ObjectFactory) buildDroppedItem(w *ecs.World, raw *repository.Object) (types.Handle, error) {
	if !raw.Data.Valid {
		return types.InvalidHandle, fmt.Errorf("dropped item %d has no data", raw.ID)
	}

	var data DroppedItemData
	if err := json.Unmarshal(raw.Data.RawMessage, &data); err != nil {
		return types.InvalidHandle, fmt.Errorf("dropped item %d: invalid data JSON: %w", raw.ID, err)
	}

	// Check if already expired
	nowUnix := ecs.GetResource[ecs.TimeState](w).Now.Unix()
	if data.DropTime+constt.DroppedDespawnSeconds <= nowUnix {
		return types.InvalidHandle, fmt.Errorf("dropped item %d: expired", raw.ID)
	}

	// Resolve item resource from itemdefs if available
	resource := data.ItemResource
	if f.itemRegistry != nil {
		if itemDef, ok := f.itemRegistry.GetByID(int(data.ItemTypeID)); ok {
			resource = itemDef.ResolveResource(false)
		}
	}

	entityID := types.EntityID(raw.ID)
	containedItemID := types.EntityID(data.ContainedItemID)

	h := w.Spawn(entityID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.CreateTransform(raw.X, raw.Y, 0))

		ecs.AddComponent(w, h, components.EntityInfo{
			TypeID:   constt.DroppedItemTypeID,
			IsStatic: true,
			Region:   raw.Region,
			Layer:    raw.Layer,
		})

		ecs.AddComponent(w, h, components.Appearance{
			Name:     nil,
			Resource: resource,
		})

		ecs.AddComponent(w, h, components.DroppedItem{
			DropTime:        data.DropTime,
			DropperID:       types.EntityID(data.DropperID),
			ContainedItemID: containedItemID,
		})

		// Create inventory container with the single item
		container := components.InventoryContainer{
			OwnerID: entityID,
			Kind:    constt.InventoryDroppedItem,
			Key:     0,
			Version: 1,
			Items: []components.InvItem{
				{
					ItemID:   containedItemID,
					TypeID:   data.ItemTypeID,
					Resource: resource,
					Quality:  data.ItemQuality,
					Quantity: data.ItemQuantity,
					W:        data.ItemW,
					H:        data.ItemH,
				},
			},
		}
		containerHandle := w.SpawnWithoutExternalID()
		ecs.AddComponent(w, containerHandle, container)

		// Register in InventoryRefIndex
		refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
		refIndex.Add(constt.InventoryDroppedItem, entityID, 0, containerHandle)
	})
	if h == types.InvalidHandle {
		return types.InvalidHandle, ErrEntitySpawnFailed
	}

	return h, nil
}

// Serialize converts an ECS entity back to a database object for persistence.
func (f *ObjectFactory) Serialize(w *ecs.World, h types.Handle) (*repository.Object, error) {
	externalID, ok := ecs.GetComponent[ecs.ExternalID](w, h)
	if !ok {
		return nil, ErrEntityNotFound
	}

	info, ok := ecs.GetComponent[components.EntityInfo](w, h)
	if !ok {
		return nil, ErrEntityNotFound
	}

	transform, ok := ecs.GetComponent[components.Transform](w, h)
	if !ok {
		return nil, ErrEntityNotFound
	}

	chunkRef, ok := ecs.GetComponent[components.ChunkRef](w, h)
	if !ok {
		return nil, ErrEntityNotFound
	}

	obj := &repository.Object{
		ID:     int64(externalID.ID),
		TypeID: int(info.TypeID),
		Region: info.Region,
		X:      int(transform.X),
		Y:      int(transform.Y),
		Layer:  info.Layer,
		ChunkX: chunkRef.CurrentChunkX,
		ChunkY: chunkRef.CurrentChunkY,
	}

	return obj, nil
}

// IsStatic returns whether a raw object is static based on its definition.
func (f *ObjectFactory) IsStatic(raw *repository.Object) bool {
	if raw.TypeID == constt.DroppedItemTypeID {
		return true
	}
	def, ok := f.registry.GetByID(raw.TypeID)
	if !ok {
		return true // safe default
	}
	return def.IsStatic
}
