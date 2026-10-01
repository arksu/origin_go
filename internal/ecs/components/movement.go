package components

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/types"
	"time"
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

	// A point stop is confirmed only after collision resolution applies the position.
	PointStopPending bool
	PointStopX       float64
	PointStopY       float64

	// Direction is ephemeral input ownership, never part of character persistence.
	Direction DirectionalInput
}

type DirectionalInput struct {
	X, Y           float64
	InputX, InputY float32
	Revision       uint32
	ClientID       uint64
	StreamEpoch    uint32
	ExpiresAt      time.Time
	UpdatePending  bool
	Blocked        bool
	BlockedMode    constt.MoveMode
}

const MovementComponentID ecs.ComponentID = 12

func init() {
	ecs.RegisterComponent[Movement](MovementComponentID)
}

func (m *Movement) HasReachedTarget(currentX, currentY float64) bool {
	if m.TargetType == constt.TargetDirection {
		return false
	}
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
	m.retireDirection()
	m.PointStopPending = false
	m.TargetType = constt.TargetNone
	m.TargetHandle = types.InvalidHandle
	m.VelocityX = 0
	m.VelocityY = 0
	if m.State != constt.StateStunned {
		m.State = constt.StateIdle
	}
}

func (m *Movement) StopAtPointTarget() {
	pointTarget := m.TargetType == constt.TargetPoint
	targetX, targetY := m.TargetX, m.TargetY
	m.ClearTarget()
	m.PointStopPending = pointTarget
	m.PointStopX, m.PointStopY = targetX, targetY
}

func (m *Movement) SetTargetPoint(x, y int) {
	m.retireDirection()
	m.PointStopPending = false
	m.TargetType = constt.TargetPoint
	m.TargetX = float64(x)
	m.TargetY = float64(y)
	m.TargetHandle = types.InvalidHandle
	m.State = constt.StateMoving
}

func (m *Movement) SetTargetHandle(handle types.Handle, x, y int) {
	m.retireDirection()
	m.PointStopPending = false
	m.TargetType = constt.TargetEntity
	m.TargetHandle = handle
	m.TargetX = float64(x)
	m.TargetY = float64(y)
	m.State = constt.StateMoving
}

func (m *Movement) retireDirection() {
	if m.TargetType == constt.TargetDirection {
		m.Direction.UpdatePending = true
	}
	m.Direction.X, m.Direction.Y = 0, 0
	m.Direction.InputX, m.Direction.InputY = 0, 0
	m.Direction.ExpiresAt = time.Time{}
	m.Direction.Blocked = false
}

// ReleaseDirection cannot stop a newer point or entity route.
func (m *Movement) ReleaseDirection() {
	if m.TargetType == constt.TargetDirection {
		m.ClearTarget()
	}
}

// ResetDirectionSession is called only after the active connection/epoch is validated.
func (m *Movement) ResetDirectionSession(clientID uint64, epoch uint32) {
	m.ReleaseDirection()
	pending := m.Direction.UpdatePending
	m.Direction = DirectionalInput{ClientID: clientID, StreamEpoch: epoch, UpdatePending: pending}
}

func (m *Movement) SetDirection(x, y float64, revision uint32, expiresAt time.Time) {
	m.PointStopPending = false
	m.TargetType = constt.TargetDirection
	m.TargetHandle = types.InvalidHandle
	m.TargetX, m.TargetY = 0, 0
	m.Direction.X, m.Direction.Y = x, y
	m.Direction.Revision = revision
	m.Direction.ExpiresAt = expiresAt
	m.Direction.UpdatePending = true
	m.Direction.Blocked = false
	m.State = constt.StateMoving
}

func (m *Movement) GetCurrentSpeed() float64 {
	switch m.Mode {
	case constt.Crawl:
		return m.Speed * 0.5
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
