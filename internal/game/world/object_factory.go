package world

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"

	"github.com/sqlc-dev/pqtype"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
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
func (f *ObjectFactory) Build(w *ecs.World, raw *repository.Object, inventories []repository.Inventory) (types.Handle, error) {
	if raw.TypeID == constt.DroppedItemTypeID {
		if len(inventories) > 0 {
			return f.buildDroppedItemFromRecords(w, raw, inventories)
		}
		return f.buildDroppedItem(w, raw)
	}

	def, ok := objectdefs.Global().GetByID(raw.TypeID)
	if !ok {
		return types.InvalidHandle, fmt.Errorf("%w: type_id=%d", ErrDefNotFound, raw.TypeID)
	}

	h := w.Spawn(types.EntityID(raw.ID), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{
			X:         float64(raw.X),
			Y:         float64(raw.Y),
			Direction: headingDegreesToRadians(raw.Heading),
		})

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

		// Container object inventory is instantiated only when:
		// - behavior includes "container"
		// - definition has components.inventory
		if f.isContainerDefinition(def) {
			links := f.spawnObjectInventories(w, types.EntityID(raw.ID), def, inventories)
			ecs.AddComponent(w, h, components.InventoryOwner{
				Inventories: links,
			})
		}
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

	// Pre-load inventory container handle before spawning entity
	containerHandle, err := f.droppedInvLoader.LoadDroppedInventory(w, entityID)
	if err != nil {
		return types.InvalidHandle, fmt.Errorf("dropped item %d: %w", raw.ID, err)
	}

	return f.spawnDroppedItemEntity(w, raw, data, containerHandle)
}

func (f *ObjectFactory) buildDroppedItemFromRecords(
	w *ecs.World,
	raw *repository.Object,
	inventories []repository.Inventory,
) (types.Handle, error) {
	if !raw.Data.Valid {
		return types.InvalidHandle, fmt.Errorf("dropped item %d has no data", raw.ID)
	}

	var data DroppedItemData
	if err := json.Unmarshal(raw.Data.RawMessage, &data); err != nil {
		return types.InvalidHandle, fmt.Errorf("dropped item %d: invalid data JSON: %w", raw.ID, err)
	}

	var rootData *objectInventoryDataV1
	for _, dbInv := range inventories {
		if constt.InventoryKind(dbInv.Kind) != constt.InventoryDroppedItem {
			continue
		}
		var invData objectInventoryDataV1
		if err := json.Unmarshal(dbInv.Data, &invData); err != nil {
			continue
		}
		invData.Kind = uint8(dbInv.Kind)
		invData.Key = uint32(dbInv.InventoryKey)
		invData.Version = dbInv.Version
		rootData = &invData
		break
	}
	if rootData == nil {
		return types.InvalidHandle, fmt.Errorf("dropped item %d: no inventory data", raw.ID)
	}

	containerHandle := f.spawnContainerTreeFromData(w, types.EntityID(raw.ID), *rootData)
	if containerHandle == types.InvalidHandle {
		return types.InvalidHandle, fmt.Errorf("dropped item %d: failed to spawn inventory container", raw.ID)
	}

	return f.spawnDroppedItemEntity(w, raw, data, containerHandle)
}

func (f *ObjectFactory) spawnDroppedItemEntity(
	w *ecs.World,
	raw *repository.Object,
	data DroppedItemData,
	containerHandle types.Handle,
) (types.Handle, error) {
	// Resolve resource from the loaded container's first item
	resource := ""
	container, hasContainer := ecs.GetComponent[components.InventoryContainer](w, containerHandle)
	if hasContainer && len(container.Items) > 0 {
		resource = container.Items[0].Resource
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

		// Register pre-loaded container in InventoryRefIndex
		refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
		refIndex.Add(constt.InventoryDroppedItem, entityID, 0, containerHandle)
	})
	if h == types.InvalidHandle {
		return types.InvalidHandle, ErrEntitySpawnFailed
	}

	return h, nil
}

func (f *ObjectFactory) isContainerDefinition(def *objectdefs.ObjectDef) bool {
	if def == nil || def.Components == nil || len(def.Components.Inventory) == 0 {
		return false
	}
	for _, behavior := range def.Behavior {
		if behavior == "container" {
			return true
		}
	}
	return false
}

type objectInventoryDataV1 struct {
	Kind    uint8                 `json:"kind"`
	Key     uint32                `json:"key"`
	Width   uint8                 `json:"width,omitempty"`
	Height  uint8                 `json:"height,omitempty"`
	Version int                   `json:"v"`
	Items   []objectInventoryItem `json:"items"`
}

type objectInventoryItem struct {
	ItemID          uint64                 `json:"item_id"`
	TypeID          uint32                 `json:"type_id"`
	Quality         uint32                 `json:"quality"`
	Quantity        uint32                 `json:"quantity"`
	X               uint8                  `json:"x,omitempty"`
	Y               uint8                  `json:"y,omitempty"`
	EquipSlot       string                 `json:"equip_slot,omitempty"`
	NestedInventory *objectInventoryDataV1 `json:"nested_inventory,omitempty"`
}

func (f *ObjectFactory) spawnObjectInventories(
	w *ecs.World,
	ownerID types.EntityID,
	def *objectdefs.ObjectDef,
	dbInventories []repository.Inventory,
) []components.InventoryLink {
	links := make([]components.InventoryLink, 0, len(def.Components.Inventory))
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)

	loadedRoot := false
	for _, dbInv := range dbInventories {
		var data objectInventoryDataV1
		if err := json.Unmarshal(dbInv.Data, &data); err != nil {
			continue
		}

		data.Kind = uint8(dbInv.Kind)
		data.Key = uint32(dbInv.InventoryKey)
		data.Version = dbInv.Version

		rootHandle := f.spawnContainerTreeFromData(w, ownerID, data)
		if rootHandle == types.InvalidHandle {
			continue
		}

		container, ok := ecs.GetComponent[components.InventoryContainer](w, rootHandle)
		if !ok {
			continue
		}
		refIndex.Add(container.Kind, container.OwnerID, container.Key, rootHandle)
		links = append(links, components.InventoryLink{
			Kind:    container.Kind,
			Key:     container.Key,
			OwnerID: container.OwnerID,
			Handle:  rootHandle,
		})
		loadedRoot = true
	}

	if loadedRoot {
		return links
	}

	for _, invDef := range def.Components.Inventory {
		kind := parseInventoryKind(invDef.Kind)
		containerHandle := w.SpawnWithoutExternalID()
		if containerHandle == types.InvalidHandle {
			continue
		}

		container := components.InventoryContainer{
			OwnerID: ownerID,
			Kind:    kind,
			Key:     invDef.Key,
			Version: 1,
			Width:   uint8(maxInt(invDef.W, 0)),
			Height:  uint8(maxInt(invDef.H, 0)),
			Items:   []components.InvItem{},
		}
		ecs.AddComponent(w, containerHandle, container)
		refIndex.Add(container.Kind, container.OwnerID, container.Key, containerHandle)
		links = append(links, components.InventoryLink{
			Kind:    container.Kind,
			Key:     container.Key,
			OwnerID: container.OwnerID,
			Handle:  containerHandle,
		})
	}

	return links
}

func parseInventoryKind(kind string) constt.InventoryKind {
	switch kind {
	case "hand":
		return constt.InventoryHand
	case "equipment":
		return constt.InventoryEquipment
	case "dropped_item":
		return constt.InventoryDroppedItem
	default:
		return constt.InventoryGrid
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func headingDegreesToRadians(heading sql.NullInt16) float64 {
	if !heading.Valid {
		return 0
	}
	degrees := math.Mod(float64(heading.Int16), 360)
	if degrees < 0 {
		degrees += 360
	}
	return degrees * math.Pi / 180
}

func radiansToHeadingDegrees(direction float64) int16 {
	if math.IsNaN(direction) || math.IsInf(direction, 0) {
		return 0
	}
	degrees := direction * 180 / math.Pi
	normalized := math.Mod(degrees, 360)
	if normalized < 0 {
		normalized += 360
	}
	return int16(math.Floor(normalized))
}

func (f *ObjectFactory) spawnContainerTreeFromData(
	w *ecs.World,
	ownerID types.EntityID,
	data objectInventoryDataV1,
) types.Handle {
	containerHandle := w.SpawnWithoutExternalID()
	if containerHandle == types.InvalidHandle {
		return types.InvalidHandle
	}

	container := components.InventoryContainer{
		OwnerID: ownerID,
		Kind:    constt.InventoryKind(data.Kind),
		Key:     data.Key,
		Version: uint64(maxInt(data.Version, 1)),
		Width:   data.Width,
		Height:  data.Height,
		Items:   make([]components.InvItem, 0, len(data.Items)),
	}

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	for _, dbItem := range data.Items {
		itemDef, found := itemdefs.Global().GetByID(int(dbItem.TypeID))
		if !found {
			continue
		}

		hasNestedItems := dbItem.NestedInventory != nil && len(dbItem.NestedInventory.Items) > 0
		item := components.InvItem{
			ItemID:    types.EntityID(dbItem.ItemID),
			TypeID:    dbItem.TypeID,
			Resource:  itemDef.ResolveResource(hasNestedItems),
			Quality:   dbItem.Quality,
			Quantity:  dbItem.Quantity,
			W:         uint8(itemDef.Size.W),
			H:         uint8(itemDef.Size.H),
			X:         dbItem.X,
			Y:         dbItem.Y,
			EquipSlot: parseEquipSlot(dbItem.EquipSlot),
		}
		container.Items = append(container.Items, item)

		if dbItem.NestedInventory != nil {
			nestedHandle := f.spawnContainerTreeFromData(w, item.ItemID, *dbItem.NestedInventory)
			if nestedHandle != types.InvalidHandle {
				refIndex.Add(constt.InventoryGrid, item.ItemID, 0, nestedHandle)
			}
		}
	}

	ecs.AddComponent(w, containerHandle, container)
	return containerHandle
}

func parseEquipSlot(slot string) netproto.EquipSlot {
	switch slot {
	case "head":
		return netproto.EquipSlot_EQUIP_SLOT_HEAD
	case "chest":
		return netproto.EquipSlot_EQUIP_SLOT_CHEST
	case "legs":
		return netproto.EquipSlot_EQUIP_SLOT_LEGS
	case "feet":
		return netproto.EquipSlot_EQUIP_SLOT_FEET
	case "hands":
		return netproto.EquipSlot_EQUIP_SLOT_HANDS
	case "back":
		return netproto.EquipSlot_EQUIP_SLOT_BACK
	case "neck":
		return netproto.EquipSlot_EQUIP_SLOT_NECK
	case "ring1":
		return netproto.EquipSlot_EQUIP_SLOT_RING_1
	case "ring2":
		return netproto.EquipSlot_EQUIP_SLOT_RING_2
	default:
		return netproto.EquipSlot_EQUIP_SLOT_NONE
	}
}

func equipSlotToString(slot netproto.EquipSlot) string {
	switch slot {
	case netproto.EquipSlot_EQUIP_SLOT_HEAD:
		return "head"
	case netproto.EquipSlot_EQUIP_SLOT_CHEST:
		return "chest"
	case netproto.EquipSlot_EQUIP_SLOT_LEGS:
		return "legs"
	case netproto.EquipSlot_EQUIP_SLOT_FEET:
		return "feet"
	case netproto.EquipSlot_EQUIP_SLOT_HANDS:
		return "hands"
	case netproto.EquipSlot_EQUIP_SLOT_BACK:
		return "back"
	case netproto.EquipSlot_EQUIP_SLOT_NECK:
		return "neck"
	case netproto.EquipSlot_EQUIP_SLOT_RING_1:
		return "ring1"
	case netproto.EquipSlot_EQUIP_SLOT_RING_2:
		return "ring2"
	default:
		return ""
	}
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

	// Skip players - they are saved via character table, not objects table
	if info.TypeID == f.getPlayerTypeID() {
		return nil, nil
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
		Heading: sql.NullInt16{
			Int16: radiansToHeadingDegrees(transform.Direction),
			Valid: true,
		},
	}

	// Serialize runtime object state for regular world objects.
	// For dropped items object.data is reserved for dropped metadata (handled below).
	if info.TypeID != constt.DroppedItemTypeID {
		if internalState, hasState := ecs.GetComponent[components.ObjectInternalState](w, h); hasState && internalState.State != nil {
			stateJSON, err := json.Marshal(internalState.State)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal object state for %d: %w", externalID.ID, err)
			}
			obj.Data = pqtype.NullRawMessage{RawMessage: stateJSON, Valid: true}
		}
	}

	// Serialize dropped item data
	if info.TypeID == constt.DroppedItemTypeID {
		if droppedItem, ok := ecs.GetComponent[components.DroppedItem](w, h); ok {
			data := DroppedItemData{
				HasInventory:    true,
				ContainedItemID: uint64(droppedItem.ContainedItemID),
				DropTime:        droppedItem.DropTime,
				DropperID:       uint64(droppedItem.DropperID),
			}
			dataJSON, err := json.Marshal(data)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal dropped item data: %w", err)
			}
			obj.Data = pqtype.NullRawMessage{RawMessage: dataJSON, Valid: true}
		}
	}

	return obj, nil
}

// DeserializeObjectState restores serialized runtime state for regular objects.
// Flags are computed by behavior systems and are not persisted.
func (f *ObjectFactory) DeserializeObjectState(raw *repository.Object) (any, error) {
	if raw == nil || raw.TypeID == constt.DroppedItemTypeID {
		return nil, nil
	}
	if !raw.Data.Valid || len(raw.Data.RawMessage) == 0 {
		return nil, nil
	}

	var state any
	if err := json.Unmarshal(raw.Data.RawMessage, &state); err != nil {
		return nil, err
	}
	return state, nil
}

// SerializeObjectInventories serializes root inventories owned by object entity.
// Nested containers remain embedded in root JSON (current format).
func (f *ObjectFactory) SerializeObjectInventories(w *ecs.World, h types.Handle) ([]repository.Inventory, error) {
	externalID, ok := ecs.GetComponent[ecs.ExternalID](w, h)
	if !ok {
		return nil, ErrEntityNotFound
	}

	entityID := externalID.ID
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)

	// Root object-owned container: kind=Grid, key=0
	rootHandle, found := refIndex.Lookup(constt.InventoryGrid, entityID, 0)
	if !found || !w.Alive(rootHandle) {
		// Dropped items store inventory in kind=DroppedItem.
		droppedHandle, droppedFound := refIndex.Lookup(constt.InventoryDroppedItem, entityID, 0)
		if !droppedFound || !w.Alive(droppedHandle) {
			return nil, nil
		}
		rootHandle = droppedHandle
	}

	container, hasContainer := ecs.GetComponent[components.InventoryContainer](w, rootHandle)
	if !hasContainer {
		return nil, nil
	}

	data, err := f.serializeContainerData(w, container)
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return []repository.Inventory{
		{
			OwnerID:      int64(container.OwnerID),
			Kind:         int16(container.Kind),
			InventoryKey: int16(container.Key),
			Data:         payload,
			Version:      int(container.Version),
		},
	}, nil
}

func (f *ObjectFactory) serializeContainerData(
	w *ecs.World,
	container components.InventoryContainer,
) (objectInventoryDataV1, error) {
	data := objectInventoryDataV1{
		Kind:    uint8(container.Kind),
		Key:     container.Key,
		Width:   container.Width,
		Height:  container.Height,
		Version: int(container.Version),
		Items:   make([]objectInventoryItem, 0, len(container.Items)),
	}

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	for _, item := range container.Items {
		dbItem := objectInventoryItem{
			ItemID:    uint64(item.ItemID),
			TypeID:    item.TypeID,
			Quality:   item.Quality,
			Quantity:  item.Quantity,
			X:         item.X,
			Y:         item.Y,
			EquipSlot: equipSlotToString(item.EquipSlot),
		}

		if nestedHandle, found := refIndex.Lookup(constt.InventoryGrid, item.ItemID, 0); found && w.Alive(nestedHandle) {
			nested, ok := ecs.GetComponent[components.InventoryContainer](w, nestedHandle)
			if ok {
				nestedData, err := f.serializeContainerDataLevel1(nested)
				if err != nil {
					return objectInventoryDataV1{}, err
				}
				dbItem.NestedInventory = &nestedData
			}
		}

		data.Items = append(data.Items, dbItem)
	}

	return data, nil
}

// serializeContainerDataLevel1 serializes nested depth=1 only.
func (f *ObjectFactory) serializeContainerDataLevel1(
	container components.InventoryContainer,
) (objectInventoryDataV1, error) {
	data := objectInventoryDataV1{
		Kind:    uint8(container.Kind),
		Key:     container.Key,
		Width:   container.Width,
		Height:  container.Height,
		Version: int(container.Version),
		Items:   make([]objectInventoryItem, 0, len(container.Items)),
	}

	for _, item := range container.Items {
		data.Items = append(data.Items, objectInventoryItem{
			ItemID:    uint64(item.ItemID),
			TypeID:    item.TypeID,
			Quality:   item.Quality,
			Quantity:  item.Quantity,
			X:         item.X,
			Y:         item.Y,
			EquipSlot: equipSlotToString(item.EquipSlot),
		})
	}

	return data, nil
}

// getPlayerTypeID returns the TypeID for player entities
func (f *ObjectFactory) getPlayerTypeID() uint32 {
	playerDef, _ := objectdefs.Global().GetByKey("player")
	if playerDef != nil {
		return uint32(playerDef.DefID)
	}
	return 0
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
