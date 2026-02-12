package world

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/objectdefs"
	"origin/internal/types"
)

type DefSpawnParams struct {
	EntityID  types.EntityID
	X         float64
	Y         float64
	Direction float64
	Region    int
	Layer     int
}

func SpawnEntityFromDef(w *ecs.World, def *objectdefs.ObjectDef, params DefSpawnParams) types.Handle {
	if w == nil || def == nil {
		return types.InvalidHandle
	}

	return w.Spawn(params.EntityID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{
			X:         params.X,
			Y:         params.Y,
			Direction: params.Direction,
		})

		ecs.AddComponent(w, h, components.EntityInfo{
			TypeID:    uint32(def.DefID),
			Behaviors: def.CopyBehaviorOrder(),
			IsStatic:  def.IsStatic,
			Region:    params.Region,
			Layer:     params.Layer,
		})

		if def.Components != nil && def.Components.Collider != nil {
			ecs.AddComponent(w, h, objectdefs.BuildColliderComponent(def.Components.Collider))
		}

		ecs.AddComponent(w, h, components.Appearance{
			Resource: def.Resource,
		})
	})
}
