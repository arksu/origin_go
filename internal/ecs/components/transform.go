package components

import "origin/internal/ecs"

// Transform represents an entity's position and orientation in 2D space
type Transform struct {
	// исходные координаты на начало тика
	X float64
	Y float64
	// направление вращения в градусах
	Direction float64
}

func CreateTransform(x, y int, direction int) Transform {
	return Transform{
		X:         float64(x),
		Y:         float64(y),
		Direction: float64(direction),
	}
}

const TransformComponentID ecs.ComponentID = 10

func init() {
	ecs.RegisterComponent[Transform](TransformComponentID)
}
