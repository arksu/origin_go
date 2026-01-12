package components

import (
	"origin/internal/ecs"
	"origin/internal/types"
)

const StopDistance float64 = 0.2

// MoveMode represents different movement states
type MoveMode uint8

const (
	Walk MoveMode = iota
	Run
	FastRun
	Swim
)

// MoveState представляет текущее состояние движения
type MoveState uint8

const (
	StateIdle MoveState = iota
	StateMoving
	StateInteracting
	StateStunned // не может двигаться
)

type TargetType uint8

const (
	TargetNone TargetType = iota
	TargetPoint
	TargetEntity
)

// Movement represents an entity's movement capabilities and state
type Movement struct {
	// updates by movement system based on Speed
	VelocityX float64
	VelocityY float64

	Mode  MoveMode
	State MoveState
	Speed float64

	TargetType   TargetType
	TargetX      int
	TargetY      int
	TargetHandle types.Handle

	InteractionRange float64
}

const MovementComponentID ecs.ComponentID = 12

func init() {
	ecs.RegisterComponent[Movement](MovementComponentID)
}

func (m *Movement) HasReachedTarget(currentX, currentY int) bool {
	if m.TargetType == TargetNone {
		return true
	}

	dx := m.TargetX - currentX
	dy := m.TargetY - currentY
	distSq := float64(dx*dx) + float64(dy*dy)
	stopDistSq := StopDistance * StopDistance

	return distSq <= stopDistSq
}

func (m *Movement) ClearTarget() {
	m.TargetType = TargetNone
	m.TargetHandle = types.InvalidHandle
	m.VelocityX = 0
	m.VelocityY = 0
	m.State = StateIdle
}

func (m *Movement) SetTargetPoint(x, y int) {
	m.TargetType = TargetPoint
	m.TargetX = x
	m.TargetY = y
	m.TargetHandle = types.InvalidHandle
	m.State = StateMoving
}

func (m *Movement) SetTargetHandle(handle types.Handle, x, y int) {
	m.TargetType = TargetEntity
	m.TargetHandle = handle
	m.TargetX = x
	m.TargetY = y
	m.State = StateMoving
}

func (m *Movement) GetCurrentSpeed() float64 {
	switch m.Mode {
	case Walk:
		return m.Speed
	case Run:
		return m.Speed * 1.5
	case FastRun:
		return m.Speed * 2.0
	case Swim:
		return m.Speed * 0.7
	default:
		return m.Speed
	}
}
