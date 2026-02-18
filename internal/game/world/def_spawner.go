package world

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/objectdefs"
	"origin/internal/types"
)

type DefSpawnParams struct {
	EntityID         types.EntityID
	X                float64
	Y                float64
	Direction        float64
	Quality          uint32
	Region           int
	Layer            int
	InitReason       contracts.ObjectBehaviorInitReason
	PreviousTypeID   uint32
	BehaviorRegistry contracts.BehaviorRegistry
}

func SpawnEntityFromDef(w *ecs.World, def *objectdefs.ObjectDef, params DefSpawnParams) types.Handle {
	if w == nil || def == nil {
		return types.InvalidHandle
	}

	handle := w.Spawn(params.EntityID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{
			X:         params.X,
			Y:         params.Y,
			Direction: params.Direction,
		})

		ecs.AddComponent(w, h, components.EntityInfo{
			TypeID:    uint32(def.DefID),
			Behaviors: def.CopyBehaviorOrder(),
			IsStatic:  def.IsStatic,
			Quality:   params.Quality,
			Region:    params.Region,
			Layer:     params.Layer,
		})

		if def.Components != nil && def.Components.Collider != nil {
			ecs.AddComponent(w, h, objectdefs.BuildColliderComponent(def.Components.Collider))
		}

		resource := objectdefs.ResolveAppearanceResource(def, nil)
		if resource == "" {
			resource = def.Resource
		}
		ecs.AddComponent(w, h, components.Appearance{Resource: resource})
		ecs.AddComponent(w, h, components.ObjectInternalState{
			IsDirty: true,
		})
	})
	if handle == types.InvalidHandle || params.InitReason == "" || params.BehaviorRegistry == nil {
		return handle
	}

	info, hasInfo := ecs.GetComponent[components.EntityInfo](w, handle)
	if !hasInfo || len(info.Behaviors) == 0 {
		return handle
	}
	if initErr := params.BehaviorRegistry.InitObjectBehaviors(&contracts.BehaviorObjectInitContext{
		World:        w,
		Handle:       handle,
		EntityID:     params.EntityID,
		EntityType:   info.TypeID,
		Reason:       params.InitReason,
		PreviousType: params.PreviousTypeID,
	}, info.Behaviors); initErr != nil {
		// Fail fast: do not leave partially initialized behavior objects alive.
		w.Despawn(handle)
		return types.InvalidHandle
	}
	return handle
}
