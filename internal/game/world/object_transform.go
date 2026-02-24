package world

import (
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/objectdefs"
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
	if w == nil || targetID == 0 || targetHandle == types.InvalidHandle || !w.Alive(targetHandle) || newDef == nil {
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
	previousTypeID := targetInfo.TypeID

	ecs.WithComponent(w, targetHandle, func(info *components.EntityInfo) {
		info.TypeID = uint32(newDef.DefID)
		info.Behaviors = newDef.CopyBehaviorOrder()
		info.IsStatic = newDef.IsStatic
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
		if chunk != nil {
			chunk.MarkRawDataDirty()
		}
	}

	if opts.EventBus != nil {
		opts.EventBus.PublishAsync(
			ecs.NewEntityAppearanceChangedEvent(targetInfo.Layer, targetID, targetHandle),
			eventbus.PriorityMedium,
		)
	}

	ecs.MarkObjectBehaviorDirty(w, targetHandle)
	return true
}
