package behaviors

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

// BuildStateSnapshotForTarget builds the current build-site UI snapshot for a target handle.
func BuildStateSnapshotForTarget(world *ecs.World, targetID types.EntityID, targetHandle types.Handle) (*netproto.S2C_BuildState, bool) {
	if world == nil || targetID == 0 || targetHandle == types.InvalidHandle || !world.Alive(targetHandle) {
		return nil, false
	}
	internalState, hasInternalState := ecs.GetComponent[components.ObjectInternalState](world, targetHandle)
	if !hasInternalState {
		return nil, false
	}
	buildState, ok := components.GetBehaviorState[components.BuildBehaviorState](internalState, buildBehaviorStateKey)
	if !ok || buildState == nil {
		return nil, false
	}

	rows := make([]*netproto.BuildStateItem, 0, len(buildState.Items))
	for i := range buildState.Items {
		slot := &buildState.Items[i]
		row := &netproto.BuildStateItem{
			Resource:      resolveBuildStateItemResource(slot.ItemKey),
			RequiredCount: slot.RequiredCount,
			PutCount:      slot.PutCount(),
			BuildCount:    slot.BuildCount,
		}
		if slot.ItemKey != "" {
			itemKey := slot.ItemKey
			row.ItemKey = &itemKey
		}
		if slot.ItemTag != "" {
			itemTag := slot.ItemTag
			row.ItemTag = &itemTag
		}
		rows = append(rows, row)
	}
	return &netproto.S2C_BuildState{
		EntityId: uint64(targetID),
		List:     rows,
	}, true
}

// BuildStateSnapshotForEntityID resolves the handle and builds a snapshot by entity id.
func BuildStateSnapshotForEntityID(world *ecs.World, targetID types.EntityID) (*netproto.S2C_BuildState, bool) {
	if world == nil || targetID == 0 {
		return nil, false
	}
	targetHandle := world.GetHandleByEntityID(targetID)
	if targetHandle == types.InvalidHandle {
		return nil, false
	}
	return BuildStateSnapshotForTarget(world, targetID, targetHandle)
}

func resolveBuildStateItemResource(itemKey string) string {
	if itemKey == "" {
		return ""
	}
	reg := itemdefs.Global()
	if reg == nil {
		return ""
	}
	def, ok := reg.GetByKey(itemKey)
	if !ok || def == nil {
		return ""
	}
	return def.ResolveResource(false)
}
