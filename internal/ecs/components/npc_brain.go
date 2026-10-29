package components

import (
	"origin/internal/ecs"
	"origin/internal/types"
)

type NPCActivity uint8

const (
	NPCIdle NPCActivity = iota
	NPCWander
	NPCWatch
	NPCRetreat
	NPCDefend
	NPCReturnHome
)

type NPCExecutionStatus uint8

const (
	NPCExecutionNone NPCExecutionStatus = iota
	NPCExecutionAccepted
	NPCExecutionBlocked
	NPCExecutionCompleted
	NPCExecutionTimeout
	NPCExecutionBusy
	NPCExecutionRejected
	NPCExecutionCanceled
)

type NPCMovementOwner uint8

const (
	NPCMovementUnowned NPCMovementOwner = iota
	NPCMovementAI
	NPCMovementAbility
)

// NPCEntityRef protects both recycled handles and persistent identity boundaries.
type NPCEntityRef struct {
	Handle   types.Handle
	EntityID types.EntityID
}
type NPCPoint struct{ X, Y float64 }
type NPCTargetKind uint8

const (
	NPCTargetNone NPCTargetKind = iota
	NPCTargetEntity
	NPCTargetPoint
	NPCTargetDirection
)

type NPCTarget struct {
	Kind      NPCTargetKind
	Entity    NPCEntityRef
	Point     NPCPoint
	Direction NPCPoint
}

// NPCPolicyState is the bounded memory a pure decision policy may update.
type NPCPolicyState struct {
	Activity    NPCActivity
	RandomState uint64
	Step        uint32
	Cautious    NPCCautiousMemory
}

// NPCCautiousMemory is value-only encounter memory shared by cautious defenders.
// It retains one encounter and one damage revision; it never grows with players.
type NPCCautiousMemory struct {
	Encounter                    NPCEntityRef
	DamageRevision               uint64
	IdleUntilTick                uint64
	WatchUntilTick               uint64
	AbsentSinceTick              uint64
	ChaseFailedSinceTick         uint64
	LastChasePosition            NPCPoint
	LastEncounterDistanceSquared float64
	ReturnRetryTick              uint64
	RetreatPending               bool
	RetreatChosen                bool
	Habituated                   bool
	AbsenceTracking              bool
	ChaseTracking                bool
}

// NPCMovementLease describes the current task's movement rules. A zero home radius
// permits movement away from home; terrain and active-chunk checks always apply.
type NPCMovementLease struct {
	// Target is the identity dependency of this accepted route, independent of
	// the remembered target that later stimuli may replace.
	Target       NPCEntityRef
	Owner        NPCMovementOwner
	Generation   uint64
	DeadlineTick uint64
	HomeRadius   float64
}
type NPCAbilityRef struct {
	ID         string
	Generation uint64
	Target     NPCTarget
}

// NPCBrain is transient and uses the same component ID for every NPC profile.
// Ability phases/cooldowns belong to the executor, never this component.
type NPCBrain struct {
	NPCPolicyState
	HomeX, HomeY         float64
	NextDecisionTick     uint64
	ActivityDeadlineTick uint64
	// Target is remembered perception state, changed only by controller stimuli.
	Target NPCEntityRef
	// DamageSource is set only by confirmed positive HP damage, never perception.
	DamageSource   NPCEntityRef
	DamageRevision uint64
	MovementLease  NPCMovementLease
	Ability        NPCAbilityRef
	LastResult     NPCExecutionStatus
}

const NPCBrainComponentID ecs.ComponentID = 40

func init() { ecs.RegisterComponent[NPCBrain](NPCBrainComponentID) }
