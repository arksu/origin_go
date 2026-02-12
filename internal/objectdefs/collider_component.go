package objectdefs

import "origin/internal/ecs/components"

func BuildColliderComponent(def *ColliderDef) components.Collider {
	if def == nil {
		return components.Collider{}
	}

	return components.Collider{
		HalfWidth:  def.W / 2.0,
		HalfHeight: def.H / 2.0,
		Layer:      def.Layer,
		Mask:       def.Mask,
	}
}
