package types

import "fmt"

// ContextAction describes one selectable object action for context menu.
type ContextAction struct {
	ActionID string
	Title    string
}

// Behavior is the base contract for every object behavior.
type Behavior interface {
	Key() string
}

// BehaviorDefConfigContext is object-definition behavior config input.
type BehaviorDefConfigContext struct {
	BehaviorKey string
	RawConfig   []byte
	Def         any
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

// BehaviorActionDeclarer returns static action contracts for fail-fast checks.
type BehaviorActionDeclarer interface {
	DeclaredActions() []BehaviorActionSpec
}

// BehaviorRuntimeResult is a runtime recompute output for object state/flags.
type BehaviorRuntimeResult struct {
	State    any
	HasState bool
	Flags    []string
}

// BehaviorRuntimeContext contains runtime recompute data.
type BehaviorRuntimeContext struct {
	World      any
	Handle     Handle
	EntityID   EntityID
	EntityType uint32
	PrevState  any
	PrevFlags  []string
}

// RuntimeBehavior recomputes behavior runtime flags/state.
type RuntimeBehavior interface {
	ApplyRuntime(ctx *BehaviorRuntimeContext) BehaviorRuntimeResult
}

// BehaviorActionListContext is used to compute context actions.
type BehaviorActionListContext struct {
	World        any
	PlayerID     EntityID
	PlayerHandle Handle
	TargetID     EntityID
	TargetHandle Handle
	Extra        any
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

// BehaviorActionValidateContext is used before action execution.
type BehaviorActionValidateContext struct {
	World        any
	PlayerID     EntityID
	PlayerHandle Handle
	TargetID     EntityID
	TargetHandle Handle
	ActionID     string
	Phase        BehaviorValidationPhase
	Extra        any
}

// ContextActionValidator validates one action id for preview/execute phase.
type ContextActionValidator interface {
	ValidateAction(ctx *BehaviorActionValidateContext) BehaviorResult
}

// BehaviorActionExecuteContext is used to execute one action id.
type BehaviorActionExecuteContext struct {
	World        any
	PlayerID     EntityID
	PlayerHandle Handle
	TargetID     EntityID
	TargetHandle Handle
	ActionID     string
	Extra        any
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
	World        any
	PlayerID     EntityID
	PlayerHandle Handle
	TargetID     EntityID
	TargetHandle Handle
	ActionID     string
	Action       any
	Extra        any
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
	World        any
	Handle       Handle
	EntityID     EntityID
	EntityType   uint32
	Reason       ObjectBehaviorInitReason
	PreviousType uint32
	Extra        any
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

// ValidateActionSpecs validates declared behavior actions.
func ValidateActionSpecs(specs []BehaviorActionSpec) error {
	if len(specs) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		if spec.ActionID == "" {
			return fmt.Errorf("declared action id must not be empty")
		}
		if _, exists := seen[spec.ActionID]; exists {
			return fmt.Errorf("duplicate declared action id %q", spec.ActionID)
		}
		seen[spec.ActionID] = struct{}{}
	}
	return nil
}
