package contracts

import (
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
)

// ContextAction describes one selectable object action for context menu.
type ContextAction struct {
	ActionID string
	Title    string
}

// Behavior is the base contract for every object behavior.
type Behavior interface {
	Key() string
}

// TreeBehaviorConfig contains tree-specific validated behavior config data.
type TreeBehaviorConfig struct {
	Priority int               `json:"priority,omitempty"`
	Stages   []TreeStageConfig `json:"stages"`
}

type TreeStageConfig struct {
	ChopPointsTotal   int          `json:"chopPointsTotal"`
	StageDuration     int          `json:"stageDurationTicks"`
	AllowChop         bool         `json:"allowChop"`
	SpawnChopObject   []string     `json:"spawnChopObject,omitempty"`
	SpawnChopItem     []string     `json:"spawnChopItem,omitempty"`
	Take              []TakeConfig `json:"take,omitempty"`
	TransformToDefKey string       `json:"transformToDefKey,omitempty"`
}

type TakeConfig struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ItemDefKey string `json:"itemDefKey"`
	Count      int    `json:"count"`
}

type TakeBehaviorConfig struct {
	Priority int          `json:"priority,omitempty"`
	Items    []TakeConfig `json:"items"`
}

// BehaviorDefConfigTarget receives validated behavior config mutations.
type BehaviorDefConfigTarget interface {
	SetTreeBehaviorConfig(cfg TreeBehaviorConfig)
	SetTakeBehaviorConfig(cfg TakeBehaviorConfig)
}

// BehaviorDefConfigContext is object-definition behavior config input.
type BehaviorDefConfigContext struct {
	BehaviorKey string
	RawConfig   []byte
	Def         BehaviorDefConfigTarget
}

// BehaviorDefConfigValidator validates behavior config in object definitions,
// applies resolved data to def, and returns behavior priority.
type BehaviorDefConfigValidator interface {
	ValidateAndApplyDefConfig(ctx *BehaviorDefConfigContext) (priority int, err error)
}

// BehaviorActionSpec declares one action supported by behavior.
// It is used for startup validation (fail fast).
type BehaviorActionSpec struct {
	ActionID     string
	StartsCyclic bool
}

// BehaviorRuntimeResult is a runtime recompute output for object state/flags.
type BehaviorRuntimeResult struct {
	State    *components.RuntimeObjectState
	HasState bool
	Flags    []string
}

// BehaviorRuntimeContext contains runtime recompute data.
type BehaviorRuntimeContext struct {
	World      *ecs.World
	Handle     types.Handle
	EntityID   types.EntityID
	EntityType uint32
	PrevState  *components.RuntimeObjectState
	PrevFlags  []string
}

// RuntimeBehavior recomputes behavior runtime flags/state.
type RuntimeBehavior interface {
	ApplyRuntime(ctx *BehaviorRuntimeContext) BehaviorRuntimeResult
}

// OpenContainerError is a behavior-layer container open failure payload.
type OpenContainerError struct {
	Code    netproto.ErrorCode
	Message string
}

// OpenContainerFn opens a container for the given player and ref.
type OpenContainerFn func(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	ref *netproto.InventoryRef,
) *OpenContainerError

// TreeChunkProvider resolves chunks for tree behavior spawn/transform flow.
type TreeChunkProvider interface {
	GetChunkFast(coord types.ChunkCoord) *core.Chunk
}

// EntityIDAllocator allocates new entity IDs.
type EntityIDAllocator interface {
	GetFreeID() types.EntityID
}

// VisionUpdateForcer forces observer vision refresh.
type VisionUpdateForcer interface {
	ForceUpdateForObserver(w *ecs.World, observerHandle types.Handle)
}

type MiniAlertSender interface {
	SendMiniAlert(entityID types.EntityID, alert *netproto.S2C_MiniAlert)
}

type BuildStateSender interface {
	SendBuildState(entityID types.EntityID, state *netproto.S2C_BuildState)
}

type GiveItemOutcome struct {
	Success      bool
	AnyDropped   bool
	PlacedInHand bool
	GrantedCount uint32
	Message      string
}

type GiveItemFn func(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	itemKey string,
	count uint32,
	quality uint32,
) GiveItemOutcome

type LiftObjectFn func(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	targetID types.EntityID,
	targetHandle types.Handle,
) BehaviorResult

// ExecutionDeps contains shared dependencies for context action execution.
type ExecutionDeps struct {
	OpenContainer    OpenContainerFn
	GiveItem         GiveItemFn
	LiftObject       LiftObjectFn
	EventBus         *eventbus.EventBus
	Chunks           TreeChunkProvider
	IDAllocator      EntityIDAllocator
	VisionForcer     VisionUpdateForcer
	Alerts           MiniAlertSender
	BuildState       BuildStateSender
	BehaviorRegistry BehaviorRegistry
	Logger           *zap.Logger
}

// BehaviorActionListContext is used to compute context actions.
type BehaviorActionListContext struct {
	World        *ecs.World
	PlayerID     types.EntityID
	PlayerHandle types.Handle
	TargetID     types.EntityID
	TargetHandle types.Handle
	Deps         *ExecutionDeps
}

// ContextActionProvider provides context menu actions.
type ContextActionProvider interface {
	ProvideActions(ctx *BehaviorActionListContext) []ContextAction
}

// BehaviorValidationPhase defines when action validation runs.
type BehaviorValidationPhase uint8

const (
	BehaviorValidationPhasePreview BehaviorValidationPhase = iota + 1
	BehaviorValidationPhaseExecute
)

// BehaviorAlertSeverity is behavior-layer alert severity.
type BehaviorAlertSeverity uint8

const (
	BehaviorAlertSeverityInfo BehaviorAlertSeverity = iota + 1
	BehaviorAlertSeverityWarning
	BehaviorAlertSeverityError
)

// BehaviorResult is the standardized behavior output.
// It is used by both validate and execute phases.
type BehaviorResult struct {
	OK          bool
	UserVisible bool
	ReasonCode  string
	Severity    BehaviorAlertSeverity
}

// BehaviorTickContext contains scheduled behavior tick execution data.
type BehaviorTickContext struct {
	World        *ecs.World
	Handle       types.Handle
	EntityID     types.EntityID
	EntityType   uint32
	BehaviorKey  string
	CurrentTick  uint64
	CurrentState *components.RuntimeObjectState
}

// BehaviorTickResult is a scheduled behavior tick result.
type BehaviorTickResult struct {
	StateChanged bool
}

// ScheduledTickBehavior handles due ticks from the unified behavior tick scheduler.
type ScheduledTickBehavior interface {
	OnScheduledTick(ctx *BehaviorTickContext) (BehaviorTickResult, error)
}

// BehaviorActionValidateContext is used before action execution.
type BehaviorActionValidateContext struct {
	World        *ecs.World
	PlayerID     types.EntityID
	PlayerHandle types.Handle
	TargetID     types.EntityID
	TargetHandle types.Handle
	ActionID     string
	Phase        BehaviorValidationPhase
	Deps         *ExecutionDeps
}

// ContextActionValidator validates one action id for preview/execute phase.
type ContextActionValidator interface {
	ValidateAction(ctx *BehaviorActionValidateContext) BehaviorResult
}

// BehaviorActionExecuteContext is used to execute one action id.
type BehaviorActionExecuteContext struct {
	World        *ecs.World
	PlayerID     types.EntityID
	PlayerHandle types.Handle
	TargetID     types.EntityID
	TargetHandle types.Handle
	ActionID     string
	Deps         *ExecutionDeps
}

// ContextActionExecutor executes one context action.
type ContextActionExecutor interface {
	ExecuteAction(ctx *BehaviorActionExecuteContext) BehaviorResult
}

// BehaviorCycleDecision is a behavior decision after one cyclic action cycle.
type BehaviorCycleDecision uint8

const (
	BehaviorCycleDecisionContinue BehaviorCycleDecision = iota + 1
	BehaviorCycleDecisionComplete
	BehaviorCycleDecisionCanceled
)

// BehaviorCycleContext contains cyclic action cycle completion data.
type BehaviorCycleContext struct {
	World        *ecs.World
	PlayerID     types.EntityID
	PlayerHandle types.Handle
	TargetID     types.EntityID
	TargetHandle types.Handle
	ActionID     string
	Action       components.ActiveCyclicAction
	Deps         *ExecutionDeps
}

// CyclicActionHandler handles cyclic cycle completion for behavior.
type CyclicActionHandler interface {
	OnCycleComplete(ctx *BehaviorCycleContext) BehaviorCycleDecision
}

// ObjectBehaviorInitReason describes why object behavior init is triggered.
type ObjectBehaviorInitReason string

const (
	ObjectBehaviorInitReasonSpawn     ObjectBehaviorInitReason = "spawn"
	ObjectBehaviorInitReasonRestore   ObjectBehaviorInitReason = "restore"
	ObjectBehaviorInitReasonTransform ObjectBehaviorInitReason = "transform"
)

// BehaviorObjectInitContext contains data for behavior state initialization.
type BehaviorObjectInitContext struct {
	World        *ecs.World
	Handle       types.Handle
	EntityID     types.EntityID
	EntityType   uint32
	Reason       ObjectBehaviorInitReason
	PreviousType uint32
}

// ObjectLifecycleInitializer initializes behavior state for object lifecycle.
type ObjectLifecycleInitializer interface {
	InitObject(ctx *BehaviorObjectInitContext) error
}

// BehaviorRegistry is the single runtime registry for all behaviors.
type BehaviorRegistry interface {
	GetBehavior(key string) (Behavior, bool)
	Keys() []string
	IsRegisteredBehaviorKey(key string) bool
	ValidateBehaviorKeys(keys []string) error
	InitObjectBehaviors(ctx *BehaviorObjectInitContext, behaviorKeys []string) error
}
