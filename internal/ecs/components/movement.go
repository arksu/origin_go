package components

import "origin/internal/ecs"

const StopDistance float32 = 0.2

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
	VelocityX float32
	VelocityY float32
	Mode      MoveMode
	State     MoveState
	Speed     float32

	TargetType   TargetType
	TargetX      float32
	TargetY      float32
	TargetHandle ecs.Handle

	InteractionRange float32
}

const MovementComponentID ecs.ComponentID = 12

func init() {
	ecs.RegisterComponent[Movement](MovementComponentID)
}

func (m *Movement) HasReachedTarget(currentX, currentY float32) bool {
	if m.TargetType == TargetNone {
		return true
	}

	dx := m.TargetX - currentX
	dy := m.TargetY - currentY
	distSq := dx*dx + dy*dy
	stopDistSq := StopDistance * StopDistance

	return distSq <= stopDistSq
}

func (m *Movement) ClearTarget() {
	m.TargetType = TargetNone
	m.TargetHandle = ecs.InvalidHandle
	m.VelocityX = 0
	m.VelocityY = 0
	m.State = StateIdle
}

func (m *Movement) SetTargetPoint(x, y float32) {
	m.TargetType = TargetPoint
	m.TargetX = x
	m.TargetY = y
	m.TargetHandle = ecs.InvalidHandle
	m.State = StateMoving
}

func (m *Movement) SetTargetHandle(handle ecs.Handle, x, y float32) {
	m.TargetType = TargetEntity
	m.TargetHandle = handle
	m.TargetX = x
	m.TargetY = y
	m.State = StateMoving
}

func (m *Movement) GetCurrentSpeed() float32 {
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
