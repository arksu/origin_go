package components

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/types"
)

// Movement represents an entity's movement capabilities and state
type Movement struct {
	// updates by movement system based on Speed
	VelocityX float64
	VelocityY float64

	Mode  constt.MoveMode
	State constt.MoveState
	Speed float64

	TargetType   constt.TargetType
	TargetX      float64
	TargetY      float64
	TargetHandle types.Handle

	InteractionRange float64

	// Steering/obstacle avoidance
	StuckCounter       int     // How many ticks we've been stuck
	LastCollisionNormX float64 // Last collision normal X
	LastCollisionNormY float64 // Last collision normal Y
	SteeringMode       bool    // Currently steering around obstacle
	RetryDirectCounter int     // Ticks until retry direct path

	// Movement sequence number (monotonically increasing per entity, wrap ok)
	MoveSeq uint32
}

const MovementComponentID ecs.ComponentID = 12

func init() {
	ecs.RegisterComponent[Movement](MovementComponentID)
}

func (m *Movement) HasReachedTarget(currentX, currentY float64) bool {
	if m.TargetType == constt.TargetNone {
		return true
	}

	dx := m.TargetX - currentX
	dy := m.TargetY - currentY
	distSq := dx*dx + dy*dy
	stopDistSq := constt.StopDistance * constt.StopDistance

	return distSq <= stopDistSq
}

func (m *Movement) ClearTarget() {
	m.TargetType = constt.TargetNone
	m.TargetHandle = types.InvalidHandle
	m.VelocityX = 0
	m.VelocityY = 0
	m.State = constt.StateIdle
}

func (m *Movement) SetTargetPoint(x, y int) {
	m.TargetType = constt.TargetPoint
	m.TargetX = float64(x)
	m.TargetY = float64(y)
	m.TargetHandle = types.InvalidHandle
	m.State = constt.StateMoving
}

func (m *Movement) SetTargetHandle(handle types.Handle, x, y int) {
	m.TargetType = constt.TargetEntity
	m.TargetHandle = handle
	m.TargetX = float64(x)
	m.TargetY = float64(y)
	m.State = constt.StateMoving
}

func (m *Movement) GetCurrentSpeed() float64 {
	switch m.Mode {
	case constt.Walk:
		return m.Speed
	case constt.Run:
		return m.Speed * 1.5
	case constt.FastRun:
		return m.Speed * 2.0
	case constt.Swim:
		return m.Speed * 0.7
	default:
		return m.Speed
	}
}
