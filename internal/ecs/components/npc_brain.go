package components

import "origin/internal/ecs"

type NPCActivity uint8

const (
	NPCIdle NPCActivity = iota
	NPCWander
)

// NPCBrain is transient, per-entity decision state shared by all NPC profiles.
// Position, velocity and the current movement target belong to Movement/Transform.
// Species parameters belong to immutable object definitions, not ECS components.
type NPCBrain struct {
	Activity     NPCActivity
	HomeX, HomeY float64
	DeadlineTick uint64
	RandomState  uint64
}

const NPCBrainComponentID ecs.ComponentID = 40

func init() { ecs.RegisterComponent[NPCBrain](NPCBrainComponentID) }
