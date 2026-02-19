package behaviors

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"origin/internal/game/behaviors/contracts"
)

// Registry is a single runtime registry for object behavior contracts/execution.
type Registry struct {
	byKey map[string]contracts.Behavior
	keys  []string
}

// NewRegistry creates a behavior registry and validates contracts in fail-fast mode.
func NewRegistry(behaviors ...contracts.Behavior) (*Registry, error) {
	registry := &Registry{
		byKey: make(map[string]contracts.Behavior, len(behaviors)),
	}

	for _, behavior := range behaviors {
		if behavior == nil {
			return nil, fmt.Errorf("behavior must not be nil")
		}

		key := strings.TrimSpace(behavior.Key())
		if key == "" {
			return nil, fmt.Errorf("behavior key must not be empty")
		}
		if _, exists := registry.byKey[key]; exists {
			return nil, fmt.Errorf("duplicate behavior key %q", key)
		}

		if err := validateBehaviorContract(behavior); err != nil {
			return nil, fmt.Errorf("behavior %q: %w", key, err)
		}

		registry.byKey[key] = behavior
		registry.keys = append(registry.keys, key)
	}

	sort.Strings(registry.keys)
	return registry, nil
}

func (r *Registry) GetBehavior(key string) (contracts.Behavior, bool) {
	if r == nil {
		return nil, false
	}
	behavior, ok := r.byKey[key]
	return behavior, ok
}

func (r *Registry) Keys() []string {
	if r == nil || len(r.keys) == 0 {
		return nil
	}
	keys := make([]string, len(r.keys))
	copy(keys, r.keys)
	return keys
}

func (r *Registry) IsRegisteredBehaviorKey(key string) bool {
	if r == nil {
		return false
	}
	_, ok := r.byKey[key]
	return ok
}

func (r *Registry) ValidateBehaviorKeys(keys []string) error {
	for _, key := range keys {
		if key == "" {
			return fmt.Errorf("behavior key must not be empty")
		}
		if !r.IsRegisteredBehaviorKey(key) {
			return fmt.Errorf("unknown behavior %q", key)
		}
	}
	return nil
}

// InitObjectBehaviors runs lifecycle init hook for behavior keys in object order.
func (r *Registry) InitObjectBehaviors(ctx *contracts.BehaviorObjectInitContext, behaviorKeys []string) error {
	if r == nil || len(behaviorKeys) == 0 || ctx == nil {
		return nil
	}
	for _, behaviorKey := range behaviorKeys {
		behavior, found := r.GetBehavior(behaviorKey)
		if !found {
			continue
		}
		initializer, ok := behavior.(contracts.ObjectLifecycleInitializer)
		if !ok {
			continue
		}
		if err := initializer.InitObject(ctx); err != nil {
			return fmt.Errorf("behavior %q init failed: %w", behaviorKey, err)
		}
	}
	return nil
}

func validateBehaviorContract(behavior contracts.Behavior) error {
	_, hasDefConfigValidator := behavior.(contracts.BehaviorDefConfigValidator)
	_, hasRuntime := behavior.(contracts.RuntimeBehavior)
	_, hasProvider := behavior.(contracts.ContextActionProvider)
	_, hasValidator := behavior.(contracts.ContextActionValidator)
	_, hasExecutor := behavior.(contracts.ContextActionExecutor)
	_, hasScheduledTick := behavior.(contracts.ScheduledTickBehavior)

	if !hasDefConfigValidator {
		return fmt.Errorf("missing def config validator capability")
	}
	if hasScheduledTick && !hasRuntime {
		return fmt.Errorf("scheduled tick capability requires runtime capability")
	}

	if (hasProvider || hasValidator) && !hasExecutor {
		return fmt.Errorf("action provider/validator requires execute capability")
	}

	return nil
}

var (
	defaultRegistryOnce sync.Once
	defaultRegistry     *Registry
	defaultRegistryErr  error
)

// DefaultRegistry returns a singleton with all built-in behavior implementations.
func DefaultRegistry() (*Registry, error) {
	defaultRegistryOnce.Do(func() {
		defaultRegistry, defaultRegistryErr = NewRegistry(
			containerBehavior{},
			treeBehavior{},
			takeBehavior{},
			playerBehavior{},
		)
	})
	return defaultRegistry, defaultRegistryErr
}

// MustDefaultRegistry is a panic-on-error helper for wiring code.
func MustDefaultRegistry() *Registry {
	registry, err := DefaultRegistry()
	if err != nil {
		panic(err)
	}
	return registry
}
