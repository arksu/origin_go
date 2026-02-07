package world

import (
	"encoding/json"
	"fmt"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

// DroppedItemData is the JSON structure stored in object.data for dropped items.
// Item details are stored in the inventory table (kind=DroppedItem, owner_id=object.id).
type DroppedItemData struct {
	HasInventory    bool   `json:"has_inventory"`
	ContainedItemID uint64 `json:"contained_item_id"`
	DropTime        int64  `json:"drop_time"`
	DropperID       uint64 `json:"dropper_id"`
}

// DroppedInventoryLoader loads a dropped item's inventory from DB and creates the ECS container.
type DroppedInventoryLoader interface {
	// LoadDroppedInventory loads inventory for a dropped item from DB,
	// creates the InventoryContainer ECS entity, and returns its handle.
	LoadDroppedInventory(w *ecs.World, ownerID types.EntityID) (types.Handle, error)
}

// ObjectFactory builds and serializes world objects using object definitions.
type ObjectFactory struct {
	droppedInvLoader DroppedInventoryLoader
}

// NewObjectFactory creates a factory backed by the given object definitions registry.
func NewObjectFactory(loader DroppedInventoryLoader) *ObjectFactory {
	return &ObjectFactory{droppedInvLoader: loader}
}

// Build creates an ECS entity from a raw database object using its definition.
func (f *ObjectFactory) Build(w *ecs.World, raw *repository.Object) (types.Handle, error) {
	if raw.TypeID == constt.DroppedItemTypeID {
		return f.buildDroppedItem(w, raw)
	}

	def, ok := objectdefs.Global().GetByID(raw.TypeID)
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
// Item data is loaded from the inventory table (kind=DroppedItem, owner_id=object.id).
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

	// Load inventory from DB via injected loader
	if f.droppedInvLoader == nil {
		return types.InvalidHandle, fmt.Errorf("dropped item %d: dropped inventory loader not configured", raw.ID)
	}

	entityID := types.EntityID(raw.ID)
	containedItemID := types.EntityID(data.ContainedItemID)

	// Pre-load inventory container handle before spawning entity
	containerHandle, err := f.droppedInvLoader.LoadDroppedInventory(w, entityID)
	if err != nil {
		return types.InvalidHandle, fmt.Errorf("dropped item %d: %w", raw.ID, err)
	}

	// Resolve resource from the loaded container's first item
	resource := ""
	container, hasContainer := ecs.GetComponent[components.InventoryContainer](w, containerHandle)
	if hasContainer && len(container.Items) > 0 {
		resource = container.Items[0].Resource
	}

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

		// Register pre-loaded container in InventoryRefIndex
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
	def, ok := objectdefs.Global().GetByID(raw.TypeID)
	if !ok {
		return true // safe default
	}
	return def.IsStatic
}
