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
	if raw.Quality < 0 {
		return types.InvalidHandle, fmt.Errorf("object %d has invalid quality %d", raw.ID, raw.Quality)
	}

	h := SpawnEntityFromDef(w, def, DefSpawnParams{
		EntityID:  types.EntityID(raw.ID),
		X:         float64(raw.X),
		Y:         float64(raw.Y),
		Direction: headingDegreesToRadians(raw.Heading),
		Quality:   uint32(raw.Quality),
		Region:    raw.Region,
		Layer:     raw.Layer,
		// Restored state is applied in chunk activation after deserialization.
		// Init hook runs there to avoid clobbering persisted behavior state.
		InitReason: "",
	})
	if h == types.InvalidHandle {
		return types.InvalidHandle, ErrEntitySpawnFailed
	}

	// Container object inventory is instantiated only when:
	// - behavior includes "container"
	// - definition has components.inventory
	if f.isContainerDefinition(def) {
		links := f.spawnObjectInventories(w, types.EntityID(raw.ID), def, inventories)
		ecs.AddComponent(w, h, components.InventoryOwner{
			Inventories: links,
		})
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
	nowRuntimeSeconds := ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal
	if data.DropTime+constt.DroppedDespawnSeconds <= nowRuntimeSeconds {
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
	droppedQuality := uint32(0)
	container, hasContainer := ecs.GetComponent[components.InventoryContainer](w, containerHandle)
	if hasContainer && len(container.Items) > 0 {
		resource = container.Items[0].Resource
		droppedQuality = container.Items[0].Quality
	}

	entityID := types.EntityID(raw.ID)
	containedItemID := types.EntityID(data.ContainedItemID)

	h := w.Spawn(entityID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.CreateTransform(raw.X, raw.Y, 0))

		ecs.AddComponent(w, h, components.EntityInfo{
			TypeID:   constt.DroppedItemTypeID,
			IsStatic: true,
			Quality:  droppedQuality,
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
	return def.HasBehavior("container")
}

// HasPersistentInventories returns true for object types that can own persistent
// inventories in ECS/DB (dropped items or defs with components.inventory).
// Behaviors are used as a conservative fallback for legacy defs.
func (f *ObjectFactory) HasPersistentInventories(typeID uint32, behaviors []string) bool {
	if typeID == constt.DroppedItemTypeID {
		return true
	}

	if def, ok := objectdefs.Global().GetByID(int(typeID)); ok {
		if def.Components != nil && len(def.Components.Inventory) > 0 {
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

func clampQualityToInt16(value uint32) int16 {
	if value > math.MaxInt16 {
		return math.MaxInt16
	}
	return int16(value)
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
	case "left_hand":
		return netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND
	case "right_hand":
		return netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND
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
	case netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND:
		return "left_hand"
	case netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND:
		return "right_hand"
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
		Quality: clampQualityToInt16(info.Quality),
		Heading: sql.NullInt16{
			Int16: radiansToHeadingDegrees(transform.Direction),
			Valid: true,
		},
	}

	// Serialize runtime object state for regular world objects.
	// For dropped items object.data is reserved for dropped metadata (handled below).
	if info.TypeID != constt.DroppedItemTypeID {
		if internalState, hasState := ecs.GetComponent[components.ObjectInternalState](w, h); hasState {
			stateJSON, hasPayload, err := serializePersistentObjectState(internalState)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal object state for %d: %w", externalID.ID, err)
			}
			if hasPayload {
				obj.Data = pqtype.NullRawMessage{RawMessage: stateJSON, Valid: true}
			} else {
				obj.Data = pqtype.NullRawMessage{}
			}
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
func (f *ObjectFactory) DeserializeObjectState(raw *repository.Object) (any, error) {
	if raw == nil || raw.TypeID == constt.DroppedItemTypeID {
		return nil, nil
	}
	if !raw.Data.Valid || len(raw.Data.RawMessage) == 0 {
		return nil, nil
	}

	var envelope components.ObjectStateEnvelope
	if err := json.Unmarshal(raw.Data.RawMessage, &envelope); err != nil {
		return nil, err
	}
	if envelope.Version != 1 {
		return nil, fmt.Errorf("unsupported object state version %d", envelope.Version)
	}

	if len(envelope.Behaviors) == 0 {
		return nil, nil
	}

	runtimeState := &components.RuntimeObjectState{
		Behaviors: make(map[string]any, len(envelope.Behaviors)),
	}

	for behaviorKey, rawBehaviorState := range envelope.Behaviors {
		switch behaviorKey {
		case "tree":
			var treeState components.TreeBehaviorState
			if err := json.Unmarshal(rawBehaviorState, &treeState); err != nil {
				return nil, fmt.Errorf("failed to decode tree state: %w", err)
			}
			runtimeState.Behaviors[behaviorKey] = &treeState
		case "take":
			var takeState components.TakeBehaviorState
			if err := json.Unmarshal(rawBehaviorState, &takeState); err != nil {
				return nil, fmt.Errorf("failed to decode take state: %w", err)
			}
			runtimeState.Behaviors[behaviorKey] = &takeState
		default:
			cloned := append([]byte(nil), rawBehaviorState...)
			runtimeState.Behaviors[behaviorKey] = json.RawMessage(cloned)
		}
	}

	return runtimeState, nil
}

func serializePersistentObjectState(internalState components.ObjectInternalState) ([]byte, bool, error) {
	runtimeState, ok := components.GetRuntimeObjectState(internalState)
	if !ok || len(runtimeState.Behaviors) == 0 {
		return nil, false, nil
	}

	envelope := components.ObjectStateEnvelope{
		Version:   1,
		Behaviors: make(map[string]json.RawMessage, len(runtimeState.Behaviors)),
	}

	for behaviorKey, rawState := range runtimeState.Behaviors {
		if behaviorKey == "" || rawState == nil {
			continue
		}

		var (
			payload []byte
			err     error
		)

		switch state := rawState.(type) {
		case json.RawMessage:
			payload = append([]byte(nil), state...)
		default:
			payload, err = json.Marshal(state)
		}
		if err != nil {
			return nil, false, fmt.Errorf("failed to marshal behavior %s state: %w", behaviorKey, err)
		}
		envelope.Behaviors[behaviorKey] = payload
	}

	if len(envelope.Behaviors) == 0 {
		return nil, false, nil
	}

	stateJSON, err := json.Marshal(envelope)
	if err != nil {
		return nil, false, err
	}
	return stateJSON, true, nil
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
