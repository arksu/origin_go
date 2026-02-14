package objectdefs

import "origin/internal/game/behaviors/contracts"

// SetTreeBehaviorConfig applies validated tree behavior config onto object def.
func (d *ObjectDef) SetTreeBehaviorConfig(cfg contracts.TreeBehaviorConfig) {
	if d == nil {
		return
	}
	d.TreeConfig = &TreeBehaviorConfig{
		Priority:               cfg.Priority,
		ChopPointsTotal:        cfg.ChopPointsTotal,
		ChopCycleDurationTicks: cfg.ChopCycleDurationTicks,
		ActionSound:            cfg.ActionSound,
		FinishSound:            cfg.FinishSound,
		LogsSpawnDefKey:        cfg.LogsSpawnDefKey,
		LogsSpawnCount:         cfg.LogsSpawnCount,
		LogsSpawnInitialOffset: cfg.LogsSpawnInitialOffset,
		LogsSpawnStepOffset:    cfg.LogsSpawnStepOffset,
		TransformToDefKey:      cfg.TransformToDefKey,
	}
}
