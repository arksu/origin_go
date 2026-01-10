package components

import "origin/internal/ecs"

// Transform represents an entity's position and orientation in 2D space
type Transform struct {
	// исходные координаты на начало тика
	X int
	Y int
	// то куда передвигаемся на текущем тике
	IntentX int
	IntentY int
	// направление вращения в градусах
	Direction float32
}

const TransformComponentID ecs.ComponentID = 10

func init() {
	ecs.RegisterComponent[Transform](TransformComponentID)
}
