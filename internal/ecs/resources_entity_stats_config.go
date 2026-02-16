package ecs

const (
	defaultPlayerStatsTTLms uint32 = 1000
)

// EntityStatsRuntimeConfig contains runtime tuning for entity stats mechanics.
type EntityStatsRuntimeConfig struct {
	PlayerStatsTTLms uint32
}

func ResolvePlayerStatsTTLms(w *World) uint32 {
	if w == nil {
		return defaultPlayerStatsTTLms
	}
	cfg, ok := TryGetResource[EntityStatsRuntimeConfig](w)
	if !ok || cfg == nil || cfg.PlayerStatsTTLms == 0 {
		return defaultPlayerStatsTTLms
	}
	return cfg.PlayerStatsTTLms
}
