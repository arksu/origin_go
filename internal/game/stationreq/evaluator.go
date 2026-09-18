package stationreq

import (
	"math"

	"origin/internal/craftdefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

const (
	FailureStationMissing      = "station_missing"
	FailureCapabilityMissing   = "station_capability_missing"
	FailureStateMismatch       = "station_state_mismatch"
	FailureConditionFailed     = "station_condition_failed"
	FailureResourceMissing     = "station_resource_missing"
	FailureUnsupportedProvider = "station_requirement_provider_unsupported"
	FailureDependencyCycle     = "station_requirement_dependency_cycle"
	FailureDepthExceeded       = "station_requirement_depth_exceeded"
)

type Context struct {
	World           *ecs.World
	ActorID         types.EntityID
	StationID       types.EntityID
	VisitedEntities map[types.EntityID]struct{}
	Depth           uint8
	MaxDepth        uint8
}

type Evaluation struct {
	Passed       bool
	FailureCode  string
	Consumptions map[string]uint32
}

type Provider interface {
	Source() string
	Evaluate(Context, craftdefs.StationCondition) Evaluation
}

type Evaluator struct {
	providers map[string]Provider
}

func NewEvaluator(providers ...Provider) *Evaluator {
	evaluator := &Evaluator{providers: make(map[string]Provider, len(providers)+1)}
	evaluator.Register(stationProvider{})
	for _, provider := range providers {
		evaluator.Register(provider)
	}
	return evaluator
}

func (e *Evaluator) Register(provider Provider) {
	if e == nil || provider == nil || provider.Source() == "" {
		return
	}
	e.providers[provider.Source()] = provider
}

func (e *Evaluator) Evaluate(ctx Context, requirements []craftdefs.StationRequirement) Evaluation {
	preparedContext, failure := prepareContext(ctx)
	if failure != "" {
		return failed(failure)
	}
	ctx = preparedContext

	result := Evaluation{Passed: true, Consumptions: make(map[string]uint32)}
	for _, requirement := range requirements {
		if requirement.Capability != "" || requirement.State != "" || len(requirement.Consume) > 0 {
			station, ok := stationFromContext(ctx)
			if !ok {
				return failed(FailureStationMissing)
			}
			if requirement.Capability != "" && !station.HasCapability(requirement.Capability) {
				return failed(FailureCapabilityMissing)
			}
			if requirement.State != "" && station.CurrentState != requirement.State {
				return failed(FailureStateMismatch)
			}
			for _, consumption := range requirement.Consume {
				planned := result.Consumptions[consumption.ResourceKey]
				if math.MaxUint32-planned < consumption.Amount {
					return failed(FailureResourceMissing)
				}
				nextPlanned := planned + consumption.Amount
				if station.Resources[consumption.ResourceKey] < nextPlanned {
					return failed(FailureResourceMissing)
				}
				result.Consumptions[consumption.ResourceKey] = nextPlanned
			}
		}
		for _, condition := range requirement.Conditions {
			provider, exists := e.providers[condition.Source]
			if !exists {
				return failed(FailureUnsupportedProvider)
			}
			conditionResult := provider.Evaluate(ctx, condition)
			if !conditionResult.Passed {
				return conditionResult
			}
		}
	}
	return result
}

func prepareContext(ctx Context) (Context, string) {
	if ctx.MaxDepth > 0 && ctx.Depth > ctx.MaxDepth {
		return Context{}, FailureDepthExceeded
	}
	if ctx.StationID == 0 {
		return ctx, ""
	}
	if _, alreadyVisited := ctx.VisitedEntities[ctx.StationID]; alreadyVisited {
		return Context{}, FailureDependencyCycle
	}
	visited := make(map[types.EntityID]struct{}, len(ctx.VisitedEntities)+1)
	for entityID := range ctx.VisitedEntities {
		visited[entityID] = struct{}{}
	}
	visited[ctx.StationID] = struct{}{}
	ctx.VisitedEntities = visited
	return ctx, ""
}

func failed(code string) Evaluation {
	return Evaluation{FailureCode: code, Consumptions: make(map[string]uint32)}
}

func stationFromContext(ctx Context) (components.StationState, bool) {
	if ctx.World == nil || ctx.StationID == 0 {
		return components.StationState{}, false
	}
	handle := ctx.World.GetHandleByEntityID(ctx.StationID)
	if handle == types.InvalidHandle || !ctx.World.Alive(handle) {
		return components.StationState{}, false
	}
	return ecs.GetComponent[components.StationState](ctx.World, handle)
}

type stationProvider struct{}

func (stationProvider) Source() string { return "station" }

func (stationProvider) Evaluate(ctx Context, condition craftdefs.StationCondition) Evaluation {
	station, ok := stationFromContext(ctx)
	if !ok {
		return failed(FailureStationMissing)
	}
	if condition.Kind != "value" {
		return failed(FailureConditionFailed)
	}
	actual, exists := station.Values[condition.Key]
	if !exists {
		return failed(FailureConditionFailed)
	}
	passed := false
	switch condition.Operator {
	case "eq":
		passed = actual == condition.Value
	case "gte":
		passed = actual >= condition.Value
	case "lte":
		passed = actual <= condition.Value
	}
	if !passed {
		return failed(FailureConditionFailed)
	}
	return Evaluation{Passed: true, Consumptions: make(map[string]uint32)}
}
