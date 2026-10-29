package world

import (
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/objectdefs"
	"origin/internal/playerstate"
	"origin/internal/types"

	"go.uber.org/zap"
)

type ObjectTransformChunkProvider interface {
	GetChunkFast(coord types.ChunkCoord) *core.Chunk
}

type ObjectInventoryInitializer interface {
	EnsureObjectInventoriesForDef(
		w *ecs.World,
		objectHandle types.Handle,
		ownerID types.EntityID,
		def *objectdefs.ObjectDef,
	) bool
}

type TransformObjectInPlaceOptions struct {
	DeleteBehaviorStateKeys []string
	ClearFlags              bool
	QualityOverride         *uint32
	BehaviorRegistry        contracts.BehaviorRegistry
	Chunks                  ObjectTransformChunkProvider
	EventBus                *eventbus.EventBus
	Logger                  *zap.Logger
	InventoryInitializer    ObjectInventoryInitializer
}

// TransformObjectToDefInPlace updates a live object entity to a new object definition while
// preserving entity identity and world position. It centralizes transform semantics used by
// gameplay systems (e.g. tree -> stump, build-site -> final object).
func TransformObjectToDefInPlace(
	w *ecs.World,
	targetID types.EntityID,
	targetHandle types.Handle,
	newDef *objectdefs.ObjectDef,
	opts TransformObjectInPlaceOptions,
) bool {
	if w == nil || targetID == 0 || targetHandle == types.InvalidHandle || !w.Alive(targetHandle) || newDef == nil || newDef.Key == "player" || newDef.HP <= 0 {
		return false
	}
	if ecs.ObjectDestructionPending(w, targetHandle) || ecs.InventoryHandleReserved(w, targetHandle) {
		return false
	}
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	targetInfo, hasInfo := ecs.GetComponent[components.EntityInfo](w, targetHandle)
	if !hasInfo {
		return false
	}
	// Dropping a persistent identity would require a durable row deletion.
	// This helper only supports transformations that can be persisted by upsert.
	if !targetInfo.Transient && newDef.Transient {
		return false
	}
	if !ecs.HasComponent[components.ObjectInternalState](w, targetHandle) {
		return false
	}
	var spatial *core.SpatialHashGrid
	var position components.Transform
	if targetInfo.IsStatic != newDef.IsStatic && opts.Chunks != nil {
		if ref, hasRef := ecs.GetComponent[components.ChunkRef](w, targetHandle); hasRef {
			chunk := opts.Chunks.GetChunkFast(types.ChunkCoord{X: ref.CurrentChunkX, Y: ref.CurrentChunkY})
			// An unavailable chunk has no local spatial index to migrate.
			if chunk != nil {
				var hasPosition bool
				position, hasPosition = ecs.GetComponent[components.Transform](w, targetHandle)
				if !hasPosition {
					return false
				}
				spatial = chunk.Spatial()
			}
		}
	}
	previousTypeID := targetInfo.TypeID
	if previousTypeID != uint32(newDef.DefID) && ecs.HasComponent[components.NPCBrain](w, targetHandle) {
		// Retire the old execution before changing its definition or removing
		// runtime components. The contract avoids a world -> behaviors cycle.
		if lifecycle, exists := ecs.TryGetResource[contracts.NPCLifecycle](w); exists && lifecycle.Controller != nil {
			lifecycle.Controller.Retire(targetHandle, contracts.NPCRetireTransform)
		}
	}
	if newDef.NPCConfig == nil && ecs.HasComponent[components.NPCBrain](w, targetHandle) {
		playerstate.StopMovement(w, targetHandle)
		ecs.RemoveComponent[components.NPCBrain](w, targetHandle)
		ecs.RemoveComponent[components.Movement](w, targetHandle)
		ecs.RemoveComponent[components.CollisionResult](w, targetHandle)
	}
	if spatial != nil {
		x, y := int(position.X), int(position.Y)
		if targetInfo.IsStatic {
			spatial.RemoveStatic(targetHandle, x, y)
			spatial.AddDynamic(targetHandle, x, y)
		} else {
			spatial.RemoveDynamic(targetHandle, x, y)
			spatial.AddStatic(targetHandle, x, y)
		}
	}

	ecs.WithComponent(w, targetHandle, func(info *components.EntityInfo) {
		info.TypeID = uint32(newDef.DefID)
		info.Behaviors = newDef.CopyBehaviorOrder()
		info.IsStatic = newDef.IsStatic
		info.Indestructible = newDef.Indestructible
		info.Transient = newDef.Transient
		if opts.QualityOverride != nil {
			info.Quality = *opts.QualityOverride
		}
	})

	if opts.InventoryInitializer != nil {
		opts.InventoryInitializer.EnsureObjectInventoriesForDef(w, targetHandle, targetID, newDef)
	}

	resource := objectdefs.ResolveAppearanceResource(newDef, nil)
	if resource == "" {
		resource = newDef.Resource
	}
	if _, hasAppearance := ecs.GetComponent[components.Appearance](w, targetHandle); hasAppearance {
		ecs.WithComponent(w, targetHandle, func(appearance *components.Appearance) {
			appearance.Resource = resource
		})
	}

	if newDef.Components != nil && newDef.Components.Collider != nil {
		nextCollider := objectdefs.BuildColliderComponent(newDef.Components.Collider)
		if _, hasCollider := ecs.GetComponent[components.Collider](w, targetHandle); hasCollider {
			ecs.WithComponent(w, targetHandle, func(c *components.Collider) {
				c.HalfWidth = nextCollider.HalfWidth
				c.HalfHeight = nextCollider.HalfHeight
				c.Layer = nextCollider.Layer
				c.Mask = nextCollider.Mask
			})
		} else {
			ecs.AddComponent(w, targetHandle, nextCollider)
		}
	} else {
		ecs.RemoveComponent[components.Collider](w, targetHandle)
	}

	ecs.WithComponent(w, targetHandle, func(state *components.ObjectInternalState) {
		if previousTypeID != uint32(newDef.DefID) {
			// Only a real type transition creates the destination's full HP.
			state.HP = float64(newDef.HP)
			state.HasHP = true
		}
		for _, key := range opts.DeleteBehaviorStateKeys {
			if key == "" {
				continue
			}
			components.DeleteBehaviorState(state, key)
		}
		if opts.ClearFlags {
			state.Flags = nil
		}
		state.IsDirty = true
	})

	ecs.CancelBehaviorTicksByEntityID(w, targetID)
	ecs.CancelBehaviorRuntimeByHandle(w, targetHandle)

	// Lifecycle hooks must see the destination's station state, just as on spawn.
	if newDef.Station != nil {
		ecs.AddComponent(w, targetHandle, newStationState(newDef.Station))
	} else {
		ecs.RemoveComponent[components.StationState](w, targetHandle)
	}
	ecs.WithComponent(w, targetHandle, func(state *components.ObjectInternalState) {
		if runtime, ok := components.GetRuntimeObjectState(*state); ok {
			runtime.Station = nil
		}
	})

	if opts.BehaviorRegistry != nil {
		currentInfo, hasCurrentInfo := ecs.GetComponent[components.EntityInfo](w, targetHandle)
		if hasCurrentInfo {
			if err := opts.BehaviorRegistry.InitObjectBehaviors(&contracts.BehaviorObjectInitContext{
				World:        w,
				Handle:       targetHandle,
				EntityID:     targetID,
				EntityType:   currentInfo.TypeID,
				PreviousType: previousTypeID,
				Reason:       contracts.ObjectBehaviorInitReasonTransform,
			}, currentInfo.Behaviors); err != nil {
				logger.Error("object transform: failed to init transformed object behaviors",
					zap.Uint64("target_id", uint64(targetID)),
					zap.Uint32("from_type", previousTypeID),
					zap.Uint32("to_type", currentInfo.TypeID),
					zap.Error(err))
			}
		}
	}

	if chunkRef, hasChunkRef := ecs.GetComponent[components.ChunkRef](w, targetHandle); hasChunkRef && opts.Chunks != nil {
		chunk := opts.Chunks.GetChunkFast(types.ChunkCoord{
			X: chunkRef.CurrentChunkX,
			Y: chunkRef.CurrentChunkY,
		})
		if chunk != nil && !newDef.Transient {
			chunk.MarkRawDataDirty()
		}
	}

	ecs.PublishEntityAppearanceChanged(w, opts.EventBus, targetID, targetHandle)

	ecs.MarkObjectBehaviorDirty(w, targetHandle)
	return true
}
