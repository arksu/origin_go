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
		ChopStaminaCost:        cfg.ChopStaminaCost,
		ActionSound:            cfg.ActionSound,
		FinishSound:            cfg.FinishSound,
		LogsSpawnDefKey:        cfg.LogsSpawnDefKey,
		LogsSpawnCount:         cfg.LogsSpawnCount,
		LogsSpawnInitialOffset: cfg.LogsSpawnInitialOffset,
		LogsSpawnStepOffset:    cfg.LogsSpawnStepOffset,
		TransformToDefKey:      cfg.TransformToDefKey,
		GrowthStageMax:         cfg.GrowthStageMax,
		GrowthStartStage:       cfg.GrowthStartStage,
		GrowthStageDurations:   append([]int(nil), cfg.GrowthStageDurations...),
		AllowedChopStages:      append([]int(nil), cfg.AllowedChopStages...),
	}
}
