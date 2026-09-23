package actiondefs

import (
	"fmt"
	"sync"
)

type Registry struct {
	byID map[string]*Definition
	all  []*Definition
}

var (
	globalRegistry *Registry
	registryOnce   sync.Once
)

func SetGlobal(registry *Registry) {
	registryOnce.Do(func() { globalRegistry = registry })
}

func SetGlobalForTesting(registry *Registry) {
	globalRegistry = registry
}

func Global() *Registry {
	return globalRegistry
}

func NewRegistry(definitions []Definition) *Registry {
	registry := &Registry{
		byID: make(map[string]*Definition, len(definitions)),
		all:  make([]*Definition, 0, len(definitions)),
	}
	for index := range definitions {
		definition := &definitions[index]
		registry.byID[definition.ID] = definition
		registry.all = append(registry.all, definition)
	}
	return registry
}

func (registry *Registry) Get(id string) (*Definition, bool) {
	if registry == nil {
		return nil, false
	}
	definition, exists := registry.byID[id]
	return definition, exists
}

func (registry *Registry) All() []*Definition {
	if registry == nil {
		return nil
	}
	definitions := make([]*Definition, len(registry.all))
	copy(definitions, registry.all)
	return definitions
}

// ValidateHandlers prevents a definition from being visible without executable behavior.
func (registry *Registry) ValidateHandlers(handlerIDs []string) error {
	if registry == nil {
		return fmt.Errorf("action definitions registry is nil")
	}
	handlers := make(map[string]struct{}, len(handlerIDs))
	for _, id := range handlerIDs {
		if _, duplicate := handlers[id]; duplicate {
			return fmt.Errorf("duplicate action handler %q", id)
		}
		handlers[id] = struct{}{}
		if _, exists := registry.byID[id]; !exists {
			return fmt.Errorf("action handler %q has no definition", id)
		}
	}
	for _, definition := range registry.all {
		if _, exists := handlers[definition.ID]; !exists {
			return fmt.Errorf("%s: action %q has no handler", definition.SourceFile, definition.ID)
		}
	}
	return nil
}
