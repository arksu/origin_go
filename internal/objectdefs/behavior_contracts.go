package objectdefs

import "origin/internal/game/behaviors/contracts"

// SetTreeBehaviorConfig applies validated tree behavior config onto object def.
func (d *ObjectDef) SetTreeBehaviorConfig(cfg contracts.TreeBehaviorConfig) {
	if d == nil {
		return
	}
	stages := make([]TreeStageConfig, 0, len(cfg.Stages))
	for _, stage := range cfg.Stages {
		stages = append(stages, TreeStageConfig{
			ChopPointsTotal:   stage.ChopPointsTotal,
			StageDuration:     stage.StageDuration,
			AllowChop:         stage.AllowChop,
			SpawnChopObject:   append([]string(nil), stage.SpawnChopObject...),
			SpawnChopItem:     append([]string(nil), stage.SpawnChopItem...),
			TransformToDefKey: stage.TransformToDefKey,
		})
	}
	d.TreeConfig = &TreeBehaviorConfig{
		Priority: cfg.Priority,
		Stages:   stages,
	}
}
