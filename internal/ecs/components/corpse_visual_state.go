package components

import "origin/internal/ecs"

// CorpseVisualState preserves the public pose revision after EntityHealth is
// removed. Its presence also marks a restored player corpse as lying.
type CorpseVisualState struct {
	LyingRevision uint64 // Runtime only; a restored entity has a new generation.
}

const CorpseVisualStateComponentID ecs.ComponentID = 39

func init() {
	ecs.RegisterComponent[CorpseVisualState](CorpseVisualStateComponentID)
}
