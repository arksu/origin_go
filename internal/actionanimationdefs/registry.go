package actionanimationdefs

import (
	"fmt"
	"origin/internal/sounddefs"
	"sync"
)

type Registry struct {
	byKey    map[string]*Definition
	bySource map[Source]*Definition
	all      []*Definition
}

var globalRegistry *Registry
var registryOnce sync.Once

func SetGlobal(registry *Registry)           { registryOnce.Do(func() { globalRegistry = registry }) }
func Global() *Registry                      { return globalRegistry }
func SetGlobalForTesting(registry *Registry) { globalRegistry = registry }

func NewRegistry(definitions []Definition) (*Registry, error) {
	registry := &Registry{byKey: map[string]*Definition{}, bySource: map[Source]*Definition{}}
	for _, definition := range definitions {
		if err := validate(&definition); err != nil {
			return nil, fmt.Errorf("%s: %s: %w", definition.SourceFile, definition.Key, err)
		}
		if previous, exists := registry.byKey[definition.Key]; exists {
			return nil, fmt.Errorf("%s: duplicate key %q (previously %s)", definition.SourceFile, definition.Key, previous.SourceFile)
		}
		if previous, exists := registry.bySource[definition.Source]; exists {
			return nil, fmt.Errorf("%s: duplicate source selector (previously %s key %q)", definition.SourceFile, previous.SourceFile, previous.Key)
		}
		registry.byKey[definition.Key] = &definition
		registry.bySource[definition.Source] = &definition
		registry.all = append(registry.all, &definition)
	}
	return registry, nil
}

func (registry *Registry) Resolve(source Source) (*Definition, bool) {
	if registry == nil {
		return nil, false
	}
	binding, exists := registry.bySource[normalizeSource(source)]
	return binding, exists
}

func (registry *Registry) Get(key string) (*Definition, bool) {
	if registry == nil {
		return nil, false
	}
	binding, exists := registry.byKey[key]
	return binding, exists
}

func (registry *Registry) All() []*Definition {
	if registry == nil {
		return nil
	}
	return append([]*Definition(nil), registry.all...)
}

// PrepareSoundCues resolves ownership before cycles begin, avoiding registry work per tick.
func (registry *Registry) PrepareSoundCues(sounds *sounddefs.Registry) error {
	for _, binding := range registry.all {
		worldCues, localCues := []SoundCue{}, []SoundCue{}
		for _, cue := range binding.SoundCues {
			profile, exists := sounds.Get(cue.SoundKey)
			if !exists {
				return fmt.Errorf("%s: binding %s cue %s references missing sound %s", binding.SourceFile, binding.Key, cue.ID, cue.SoundKey)
			}
			if profile.Mode == sounddefs.ModeWorld {
				worldCues = append(worldCues, cue)
			} else {
				localCues = append(localCues, cue)
			}
		}
		binding.WorldSoundCues, binding.LocalSoundCues = worldCues, localCues
	}
	return nil
}
