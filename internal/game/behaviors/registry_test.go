package behaviors

import (
	"testing"

	"origin/internal/game/behaviors/contracts"
)

type testBehaviorProviderNoExecutor struct{}

func (testBehaviorProviderNoExecutor) Key() string { return "invalid_provider_only" }
func (testBehaviorProviderNoExecutor) ValidateAndApplyDefConfig(*contracts.BehaviorDefConfigContext) (int, error) {
	return 100, nil
}
func (testBehaviorProviderNoExecutor) DeclaredActions() []contracts.BehaviorActionSpec {
	return []contracts.BehaviorActionSpec{{ActionID: "x"}}
}
func (testBehaviorProviderNoExecutor) ProvideActions(*contracts.BehaviorActionListContext) []contracts.ContextAction {
	return []contracts.ContextAction{{ActionID: "x", Title: "X"}}
}

type testBehaviorDeclaredNoExecutor struct{}

func (testBehaviorDeclaredNoExecutor) Key() string { return "invalid_declared_only" }
func (testBehaviorDeclaredNoExecutor) ValidateAndApplyDefConfig(*contracts.BehaviorDefConfigContext) (int, error) {
	return 100, nil
}
func (testBehaviorDeclaredNoExecutor) DeclaredActions() []contracts.BehaviorActionSpec {
	return []contracts.BehaviorActionSpec{{ActionID: "x"}}
}

type testBehaviorCyclicDeclaredNoHandler struct{}

func (testBehaviorCyclicDeclaredNoHandler) Key() string { return "invalid_cyclic_declared" }
func (testBehaviorCyclicDeclaredNoHandler) ValidateAndApplyDefConfig(*contracts.BehaviorDefConfigContext) (int, error) {
	return 100, nil
}
func (testBehaviorCyclicDeclaredNoHandler) DeclaredActions() []contracts.BehaviorActionSpec {
	return []contracts.BehaviorActionSpec{{ActionID: "x", StartsCyclic: true}}
}
func (testBehaviorCyclicDeclaredNoHandler) ExecuteAction(*contracts.BehaviorActionExecuteContext) contracts.BehaviorResult {
	return contracts.BehaviorResult{OK: true}
}

type testBehaviorValidActionAndCycle struct{}

func (testBehaviorValidActionAndCycle) Key() string { return "valid_action_cyclic" }
func (testBehaviorValidActionAndCycle) ValidateAndApplyDefConfig(*contracts.BehaviorDefConfigContext) (int, error) {
	return 100, nil
}
func (testBehaviorValidActionAndCycle) DeclaredActions() []contracts.BehaviorActionSpec {
	return []contracts.BehaviorActionSpec{{ActionID: "x", StartsCyclic: true}}
}
func (testBehaviorValidActionAndCycle) ExecuteAction(*contracts.BehaviorActionExecuteContext) contracts.BehaviorResult {
	return contracts.BehaviorResult{OK: true}
}
func (testBehaviorValidActionAndCycle) OnCycleComplete(*contracts.BehaviorCycleContext) contracts.BehaviorCycleDecision {
	return contracts.BehaviorCycleDecisionComplete
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

func TestNewRegistry_AllowsCyclicDeclaredWithoutHandler(t *testing.T) {
	_, err := NewRegistry(testBehaviorCyclicDeclaredNoHandler{})
	if err != nil {
		t.Fatalf("expected registry to allow cyclic declaration without handler, got: %v", err)
	}
}

func TestNewRegistry_Success_WhenActionAndCyclicCapabilitiesMatch(t *testing.T) {
	_, err := NewRegistry(testBehaviorValidActionAndCycle{})
	if err != nil {
		t.Fatalf("expected valid registry, got error: %v", err)
	}
}
