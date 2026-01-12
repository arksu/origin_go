package components

import (
	"origin/internal/ecs"
	"origin/internal/types"
)

// CollisionResult represents the result of a collision check
type CollisionResult struct {
	FinalX, FinalY                     float64          // Финальная позиция
	CollisionNormalX, CollisionNormalY float64          // Нормаль коллизии
	CollidedWith                       []types.EntityID // С кем столкнулись
	WasBlocked                         bool             // Было ли движение заблокировано
}

const CollisionResultComponentID ecs.ComponentID = 15

func init() {
	ecs.RegisterComponent[CollisionResult](CollisionResultComponentID)
}
