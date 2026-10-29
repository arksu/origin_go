package contracts

import (
	"encoding/json"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

// NPCBehaviorConfig is immutable definition content. Params is profile-specific
// JSON validated at load time; it is not an interpreted behavior program.
type NPCProfileParameters interface{ NPCProfileParameters() }

type NPCBehaviorConfig struct {
	Priority        int                  `json:"priority,omitempty"`
	Profile         string               `json:"profile"`
	Capabilities    []string             `json:"capabilities,omitempty"`
	Abilities       []string             `json:"abilities,omitempty"`
	Parameters      NPCProfileParameters `json:"-"`
	Params          json.RawMessage      `json:"params,omitempty"`
	Speed           float64              `json:"speed"`
	IdleMinSeconds  float64              `json:"idleMinSeconds"`
	IdleMaxSeconds  float64              `json:"idleMaxSeconds"`
	WanderMinTiles  float64              `json:"wanderMinTiles"`
	WanderMaxTiles  float64              `json:"wanderMaxTiles"`
	HomeRadiusTiles float64              `json:"homeRadiusTiles"`
	MaxWalkSeconds  float64              `json:"maxWalkSeconds"`
}
type EntityRef = components.NPCEntityRef
type NPCPoint = components.NPCPoint
type NPCTarget = components.NPCTarget
type NPCTargetKind = components.NPCTargetKind

const (
	NPCTargetNone      = components.NPCTargetNone
	NPCTargetEntity    = components.NPCTargetEntity
	NPCTargetPoint     = components.NPCTargetPoint
	NPCTargetDirection = components.NPCTargetDirection
)

type NPCExecutionStatus = components.NPCExecutionStatus

const (
	NPCExecutionNone      = components.NPCExecutionNone
	NPCExecutionAccepted  = components.NPCExecutionAccepted
	NPCExecutionBlocked   = components.NPCExecutionBlocked
	NPCExecutionCompleted = components.NPCExecutionCompleted
	NPCExecutionTimeout   = components.NPCExecutionTimeout
	NPCExecutionBusy      = components.NPCExecutionBusy
	NPCExecutionRejected  = components.NPCExecutionRejected
	NPCExecutionCanceled  = components.NPCExecutionCanceled
)

type NPCIntentKind uint8

const (
	NPCIntentContinue NPCIntentKind = iota
	NPCIntentWait
	NPCIntentMove
	NPCIntentStop
	NPCIntentUseAbility
	NPCIntentCancelAbility
)
const NPCCandidateLimit = 4

type NPCMoveIntent struct {
	Candidates   [NPCCandidateLimit]NPCPoint
	Count        uint8
	Target       EntityRef
	DeadlineTick uint64
	HomeRadius   float64
}
type NPCIntent struct {
	Kind    NPCIntentKind
	Move    NPCMoveIntent
	Ability NPCAbilityRequest
	// CancelGeneration identifies the observed execution to interrupt.
	CancelGeneration uint64
}
type NPCExecutionResult struct {
	Status     NPCExecutionStatus
	Generation uint64
}

type NPCAbilityPhase uint8

const (
	NPCAbilityNone NPCAbilityPhase = iota
	NPCAbilityWindup
	NPCAbilityActive
	NPCAbilityRecovery
	NPCAbilityFinished
)

type NPCAbilityDefinition struct {
	ID           string
	TargetKind   NPCTargetKind
	OwnsMovement bool
}
type NPCAbilityRequest struct {
	Actor      EntityRef
	AbilityID  string
	Target     NPCTarget
	Generation uint64
}

// NPCAbilityState is an owned value; observing it never advances an execution.
type NPCAbilityState struct {
	Actor      EntityRef
	AbilityID  string
	Generation uint64
	Phase      NPCAbilityPhase
	Status     NPCExecutionStatus
}

func (s NPCAbilityState) Running() bool {
	return s.Phase >= NPCAbilityWindup && s.Phase <= NPCAbilityRecovery
}

// NPCAbilityExecutor is shared by actor controllers. Its owning system advances
// phases/cooldowns independently of AI ticks, under the owning world's lock.
// State matches actor identity and execution generation; retained terminal
// results remain observable after the target has been destroyed. Starting and
// each delayed effect must revalidate the relevant target as well as the actor.
// Cancel with NPCRetireRequested may refuse interruption by returning a running
// state with Busy. All other retirement reasons require releasing the execution
// and its movement before returning. Cancel always returns the resulting state.
type NPCAbilityExecutor interface {
	TryStart(NPCAbilityRequest) NPCAbilityState
	Cancel(EntityRef, uint64, NPCRetireReason) NPCAbilityState
	State(EntityRef, uint64) NPCAbilityState
}
type NPCRetireReason uint8

const (
	NPCRetireDespawn NPCRetireReason = iota
	NPCRetireTransform
	NPCRetireQuarantine
	NPCRetireInvalidTarget
	NPCRetireMovementBlocked
	NPCRetireTimeout
	NPCRetireRequested
)

type NPCActorLifecycle interface {
	Retire(types.Handle, NPCRetireReason)
}

// NPCLifecycle exposes the controller to lifecycle code without a game package cycle.
type NPCLifecycle struct{ Controller NPCActorLifecycle }
