package ecs

import _const "origin/internal/const"

const (
	defaultPlayerStatsTTLms          uint32 = 1000
	defaultStaminaRegenIntervalTicks uint64 = _const.DefaultStaminaRegenIntervalTicks
)

// EntityStatsRuntimeConfig contains runtime tuning for entity stats mechanics.
type EntityStatsRuntimeConfig struct {
	PlayerStatsTTLms          uint32
	StaminaRegenIntervalTicks uint64
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

func ResolveStaminaRegenIntervalTicks(w *World) uint64 {
	if w == nil {
		return defaultStaminaRegenIntervalTicks
	}
	cfg, ok := TryGetResource[EntityStatsRuntimeConfig](w)
	if !ok || cfg == nil || cfg.StaminaRegenIntervalTicks == 0 {
		return defaultStaminaRegenIntervalTicks
	}
	return cfg.StaminaRegenIntervalTicks
}
