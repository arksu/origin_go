package components

import (
	"origin/internal/ecs"
	"origin/internal/types"
)

// CollisionResult represents the result of a collision check
type CollisionResult struct {
	FinalX, FinalY                     float64 // Финальная позиция
	CollisionNormalX, CollisionNormalY float64 // Нормаль коллизии
	IsPhantom                          bool
	CollidedWith                       types.EntityID // С кем столкнулись (0 если нет коллизии)
	HasCollision                       bool
	PerpendicularOscillation           bool           // движение дальше невозможно, скольжение невозможно
	PrevFinalX, PrevFinalY             float64        // Позиция с предыдущего кадра для обнаружения осцилляции
	PrevCollidedWith                   types.EntityID // Объект коллизии с предыдущего кадра
}

const CollisionResultComponentID ecs.ComponentID = 15

func init() {
	ecs.RegisterComponent[CollisionResult](CollisionResultComponentID)
}
