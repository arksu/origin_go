package components

import (
	"origin/internal/ecs"
	"origin/internal/types"
)

// Collider represents an entity's collision box dimensions
type Collider struct {
	HalfWidth  int
	HalfHeight int
	Layer      uint64 // Collision layer (битовая маска)
	Mask       uint64 // С какими слоями проверяем

	Phantom *PhantomCollider
}

type PhantomCollider struct {
	// Абсолютные мировые координаты
	WorldX float64
	WorldY float64

	HalfWidth  int
	HalfHeight int

	BuildingType types.ObjectType
}

const ColliderComponentID ecs.ComponentID = 14

func init() {
	ecs.RegisterComponent[Collider](ColliderComponentID)
}
