package behaviors

import (
	"testing"

	"origin/internal/types"
)

type testBehaviorProviderNoExecutor struct{}

func (testBehaviorProviderNoExecutor) Key() string { return "invalid_provider_only" }
func (testBehaviorProviderNoExecutor) ValidateAndApplyDefConfig(*types.BehaviorDefConfigContext) (int, error) {
	return 100, nil
}
func (testBehaviorProviderNoExecutor) DeclaredActions() []types.BehaviorActionSpec {
	return []types.BehaviorActionSpec{{ActionID: "x"}}
}
func (testBehaviorProviderNoExecutor) ProvideActions(*types.BehaviorActionListContext) []types.ContextAction {
	return []types.ContextAction{{ActionID: "x", Title: "X"}}
}

type testBehaviorDeclaredNoExecutor struct{}

func (testBehaviorDeclaredNoExecutor) Key() string { return "invalid_declared_only" }
func (testBehaviorDeclaredNoExecutor) ValidateAndApplyDefConfig(*types.BehaviorDefConfigContext) (int, error) {
	return 100, nil
}
func (testBehaviorDeclaredNoExecutor) DeclaredActions() []types.BehaviorActionSpec {
	return []types.BehaviorActionSpec{{ActionID: "x"}}
}

type testBehaviorCyclicDeclaredNoHandler struct{}

func (testBehaviorCyclicDeclaredNoHandler) Key() string { return "invalid_cyclic_declared" }
func (testBehaviorCyclicDeclaredNoHandler) ValidateAndApplyDefConfig(*types.BehaviorDefConfigContext) (int, error) {
	return 100, nil
}
func (testBehaviorCyclicDeclaredNoHandler) DeclaredActions() []types.BehaviorActionSpec {
	return []types.BehaviorActionSpec{{ActionID: "x", StartsCyclic: true}}
}
func (testBehaviorCyclicDeclaredNoHandler) ExecuteAction(*types.BehaviorActionExecuteContext) types.BehaviorResult {
	return types.BehaviorResult{OK: true}
}

type testBehaviorValidActionAndCycle struct{}

func (testBehaviorValidActionAndCycle) Key() string { return "valid_action_cyclic" }
func (testBehaviorValidActionAndCycle) ValidateAndApplyDefConfig(*types.BehaviorDefConfigContext) (int, error) {
	return 100, nil
}
func (testBehaviorValidActionAndCycle) DeclaredActions() []types.BehaviorActionSpec {
	return []types.BehaviorActionSpec{{ActionID: "x", StartsCyclic: true}}
}
func (testBehaviorValidActionAndCycle) ExecuteAction(*types.BehaviorActionExecuteContext) types.BehaviorResult {
	return types.BehaviorResult{OK: true}
}
func (testBehaviorValidActionAndCycle) OnCycleComplete(*types.BehaviorCycleContext) types.BehaviorCycleDecision {
	return types.BehaviorCycleDecisionComplete
}

func TestNewRegistry_FailFast_WhenProviderMissingExecutor(t *testing.T) {
	_, err := NewRegistry(testBehaviorProviderNoExecutor{})
	if err == nil {
		t.Fatalf("expected fail-fast error when provider has no executor")
	}
}

func TestNewRegistry_FailFast_WhenDeclaredActionMissingExecutor(t *testing.T) {
	_, err := NewRegistry(testBehaviorDeclaredNoExecutor{})
	if err == nil {
		t.Fatalf("expected fail-fast error when declared action has no executor")
	}
}

func TestNewRegistry_FailFast_WhenCyclicActionMissingHandler(t *testing.T) {
	_, err := NewRegistry(testBehaviorCyclicDeclaredNoHandler{})
	if err == nil {
		t.Fatalf("expected fail-fast error when cyclic action has no cyclic handler")
	}
}

func TestNewRegistry_Success_WhenActionAndCyclicCapabilitiesMatch(t *testing.T) {
	_, err := NewRegistry(testBehaviorValidActionAndCycle{})
	if err != nil {
		t.Fatalf("expected valid registry, got error: %v", err)
	}
}
