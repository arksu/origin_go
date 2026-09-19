package game

import (
	"fmt"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/inventory"
	"origin/internal/game/world"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"go.uber.org/zap"
)

type burnerExhaustionHandler struct {
	idAllocator          inventory.EntityIDAllocator
	replacementPersister inventory.AtomicDroppedObjectReplacementPersister
	despawnPersister     world.ObjectDespawnPersistence
	chunkManager         burnerChunkManager
	visionForcer         inventory.VisionUpdateForcer
	logger               *zap.Logger
}

type burnerChunkManager interface {
	GetChunkFast(coord types.ChunkCoord) *core.Chunk
	AddStaticToChunkSpatial(handle types.Handle, chunkX, chunkY, x, y int)
}

func (h *burnerExhaustionHandler) handle(w *ecs.World, handle types.Handle, config *objectdefs.BurnerBehaviorConfig) bool {
	if h == nil || h.idAllocator == nil || h.replacementPersister == nil || h.chunkManager == nil || config == nil {
		return false
	}
	if config.DropItem == "" {
		return true
	}
	if config.Despawn && (h.despawnPersister == nil || !h.canRemoveSource(w, handle)) {
		return false
	}

	params, err := h.outcomeParams(w, handle, config.DropItem)
	if err != nil {
		h.logFailure("build burner exhaustion outcome", err)
		return false
	}
	result, err := inventory.SpawnDroppedEntity(w, params)
	if err != nil {
		h.logFailure("spawn burner exhaustion outcome", err)
		return false
	}
	if err := inventory.PersistDroppedEntityReplacement(h.replacementPersister, params, params.Region, sourceID(w, handle)); err != nil {
		h.logFailure("persist burner exhaustion replacement", err)
		h.removeUnpersistedOutcome(w, result, params)
		return false
	}
	h.chunkManager.AddStaticToChunkSpatial(result.DroppedHandle, params.ChunkX, params.ChunkY, params.DropX, params.DropY)

	if !config.Despawn {
		return true
	}
	h.removeSource(w, handle)
	return true
}

func sourceID(w *ecs.World, handle types.Handle) types.EntityID {
	entityID, _ := w.GetExternalID(handle)
	return entityID
}

func (h *burnerExhaustionHandler) removeUnpersistedOutcome(w *ecs.World, result inventory.SpawnDroppedEntityResult, params inventory.SpawnDroppedEntityParams) {
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	refIndex.Remove(constt.InventoryDroppedItem, params.DroppedEntityID, 0)
	if w.Alive(result.ContainerHandle) {
		w.Despawn(result.ContainerHandle)
	}
	if w.Alive(result.DroppedHandle) {
		w.Despawn(result.DroppedHandle)
	}
}

func (h *burnerExhaustionHandler) canRemoveSource(w *ecs.World, handle types.Handle) bool {
	_, hasSourceID := w.GetExternalID(handle)
	_, hasInfo := ecs.GetComponent[components.EntityInfo](w, handle)
	_, hasTransform := ecs.GetComponent[components.Transform](w, handle)
	chunkRef, hasChunk := ecs.GetComponent[components.ChunkRef](w, handle)
	if !hasSourceID || !hasInfo || !hasTransform || !hasChunk || h.chunkManager.GetChunkFast(types.ChunkCoord{X: chunkRef.CurrentChunkX, Y: chunkRef.CurrentChunkY}) == nil {
		h.logFailure("remove exhausted burner", fmt.Errorf("burner source is missing identity, position, or active chunk"))
		return false
	}
	return true
}

func (h *burnerExhaustionHandler) outcomeParams(w *ecs.World, handle types.Handle, itemKey string) (inventory.SpawnDroppedEntityParams, error) {
	info, hasInfo := ecs.GetComponent[components.EntityInfo](w, handle)
	transform, hasTransform := ecs.GetComponent[components.Transform](w, handle)
	chunkRef, hasChunk := ecs.GetComponent[components.ChunkRef](w, handle)
	itemDef, hasItemDef := itemdefs.Global().GetByKey(itemKey)
	if !hasInfo || !hasTransform || !hasChunk || !hasItemDef {
		return inventory.SpawnDroppedEntityParams{}, fmt.Errorf("burner outcome is missing source or item definition")
	}
	droppedItemID := h.idAllocator.GetFreeID()
	return inventory.SpawnDroppedEntityParams{
		DroppedEntityID:   droppedItemID,
		ItemID:            droppedItemID,
		TypeID:            uint32(itemDef.DefID),
		Resource:          itemDef.ResolveResource(false),
		Quality:           info.Quality,
		Quantity:          1,
		W:                 uint8(itemDef.Size.W),
		H:                 uint8(itemDef.Size.H),
		DropX:             int(transform.X),
		DropY:             int(transform.Y),
		Region:            info.Region,
		Layer:             info.Layer,
		ChunkX:            chunkRef.CurrentChunkX,
		ChunkY:            chunkRef.CurrentChunkY,
		NowRuntimeSeconds: ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal,
	}, nil
}

func (h *burnerExhaustionHandler) removeSource(w *ecs.World, handle types.Handle) {
	sourceID, _ := w.GetExternalID(handle)
	info, _ := ecs.GetComponent[components.EntityInfo](w, handle)
	transform, _ := ecs.GetComponent[components.Transform](w, handle)
	chunkRef, _ := ecs.GetComponent[components.ChunkRef](w, handle)
	chunk := h.chunkManager.GetChunkFast(types.ChunkCoord{X: chunkRef.CurrentChunkX, Y: chunkRef.CurrentChunkY})
	if info.IsStatic {
		chunk.Spatial().RemoveStatic(handle, int(transform.X), int(transform.Y))
	} else {
		chunk.Spatial().RemoveDynamic(handle, int(transform.X), int(transform.Y))
	}
	h.despawnPersister.RecordChunkObjectDespawn(chunk, sourceID)
	w.Despawn(handle)
	h.forceVisionRefreshAll(w)
}

func (h *burnerExhaustionHandler) forceVisionRefreshAll(w *ecs.World) {
	if h.visionForcer == nil {
		return
	}
	for _, character := range ecs.GetResource[ecs.CharacterEntities](w).Map {
		if character.Handle != types.InvalidHandle && w.Alive(character.Handle) {
			h.visionForcer.ForceUpdateForObserver(w, character.Handle)
		}
	}
}

func (h *burnerExhaustionHandler) logFailure(operation string, err error) {
	if h.logger != nil {
		h.logger.Warn("Burner exhaustion failed", zap.String("operation", operation), zap.Error(err))
	}
}

func (h *burnerExhaustionHandler) systemHandler() systems.BurnerExhaustionHandler {
	return h.handle
}

func (h *burnerExhaustionHandler) ReconcileRestoredObject(w *ecs.World, handle types.Handle) bool {
	info, hasInfo := ecs.GetComponent[components.EntityInfo](w, handle)
	if !hasInfo {
		return true
	}
	def, found := objectdefs.Global().GetByID(int(info.TypeID))
	if !found || def.BurnerConfig == nil || def.BurnerConfig.DropItem == "" {
		return true
	}
	state, hasState := ecs.GetComponent[components.ObjectInternalState](w, handle)
	if !hasState {
		return true
	}
	burner, found := components.GetBehaviorState[components.BurnerBehaviorState](state, "burner")
	if !found || burner == nil || burner.Fuel > 0 || burner.OutcomeCreated {
		return true
	}
	return !h.handle(w, handle, def.BurnerConfig)
}
