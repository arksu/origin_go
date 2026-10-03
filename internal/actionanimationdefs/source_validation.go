package actionanimationdefs

import "fmt"

// Menu definitions own their concrete target contract. Resolving it at activation
// keeps animation definitions independent of action/game package dependencies.
func (registry *Registry) ValidateMenuSoundTargets(resolveTargetKind func(actionID string) (string, bool)) error {
	for _, binding := range registry.all {
		if binding.Source.Kind != "menu" {
			continue
		}
		for _, cue := range binding.SoundCues {
			if cue.Source != "target" {
				continue
			}
			if resolveTargetKind == nil {
				return fmt.Errorf("%s: binding %s cue %s: menu target resolver is required", binding.SourceFile, binding.Key, cue.ID)
			}
			kind, exists := resolveTargetKind(binding.Source.ID)
			if !exists {
				return fmt.Errorf("%s: binding %s cue %s: menu action %s is missing", binding.SourceFile, binding.Key, cue.ID, binding.Source.ID)
			}
			if kind != "object" && kind != "tile" {
				return fmt.Errorf("%s: binding %s cue %s: menu action %s target kind %q has no target sound source", binding.SourceFile, binding.Key, cue.ID, binding.Source.ID, kind)
			}
			break
		}
	}
	return nil
}
