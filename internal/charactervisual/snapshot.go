package charactervisual

import (
	"fmt"
	"sort"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

// Snapshot must run under the world lock. It owns its output, including the item
// list, so callers can serialize it without retaining mutable inventory slices.
func Snapshot(w *ecs.World, handle types.Handle) (*netproto.CharacterVisualState, error) {
	if !w.Alive(handle) {
		return nil, nil
	}
	appearance, ok := ecs.GetComponent[components.Appearance](w, handle)
	if !ok || appearance.Resource != "player" {
		return nil, nil
	}
	ownerID, ok := w.GetExternalID(handle)
	if !ok {
		return nil, fmt.Errorf("character visual has no external ID: %d", handle)
	}
	state := &netproto.CharacterVisualState{Generation: fmt.Sprintf("%d:%d", w.Layer, handle)}
	equipmentHandle, found := ecs.GetResource[ecs.InventoryRefIndex](w).Lookup(constt.InventoryEquipment, ownerID, 0)
	if !found {
		return state, nil
	}
	container, ok := ecs.GetComponent[components.InventoryContainer](w, equipmentHandle)
	if !ok || !w.Alive(equipmentHandle) || container.OwnerID != ownerID || container.Kind != constt.InventoryEquipment || container.Key != 0 {
		return nil, fmt.Errorf("invalid equipment container for character %d", ownerID)
	}
	state.Revision = container.Version
	seen := make(map[netproto.EquipSlot]bool, len(container.Items))
	for _, item := range container.Items {
		if item.EquipSlot == netproto.EquipSlot_EQUIP_SLOT_NONE || netproto.EquipSlot_name[int32(item.EquipSlot)] == "" || seen[item.EquipSlot] {
			return nil, fmt.Errorf("invalid or duplicate equipment slot %d for character %d", item.EquipSlot, ownerID)
		}
		def, ok := itemdefs.Global().GetByID(int(item.TypeID))
		if !ok {
			return nil, fmt.Errorf("unknown equipped item definition %d for character %d", item.TypeID, ownerID)
		}
		seen[item.EquipSlot] = true
		state.Equipment = append(state.Equipment, &netproto.CharacterEquipmentVisual{Slot: item.EquipSlot, VisualKey: def.Key})
	}
	sort.Slice(state.Equipment, func(i, j int) bool { return state.Equipment[i].Slot < state.Equipment[j].Slot })
	return state, nil
}
