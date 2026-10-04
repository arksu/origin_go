package components

import "origin/internal/ecs"

// EntityHealth stores runtime SHP/HHP pools and KO/death runtime state.
type EntityHealth struct {
	SHP           float64
	HHP           float64
	KOUntilUnixMs int64 // Runtime only; zero means KO has been completed.
	IsLying       bool
	LyingRevision uint64 // Runtime visual revision; never persisted.
}

const EntityHealthComponentID ecs.ComponentID = 33

func init() {
	ecs.RegisterComponent[EntityHealth](EntityHealthComponentID)
}
