package objectdefs

import "origin/internal/game/behaviors/contracts"

// SetTreeBehaviorConfig applies validated tree behavior config onto object def.
func (d *ObjectDef) SetTreeBehaviorConfig(cfg contracts.TreeBehaviorConfig) {
	if d == nil {
		return
	}
	stages := make([]TreeStageConfig, 0, len(cfg.Stages))
	for _, stage := range cfg.Stages {
		take := make([]TreeTakeConfig, 0, len(stage.Take))
		for _, entry := range stage.Take {
			take = append(take, TreeTakeConfig{
				ID:         entry.ID,
				Name:       entry.Name,
				ItemDefKey: entry.ItemDefKey,
				Count:      entry.Count,
			})
		}
		stages = append(stages, TreeStageConfig{
			ChopPointsTotal:   stage.ChopPointsTotal,
			StageDuration:     stage.StageDuration,
			AllowChop:         stage.AllowChop,
			SpawnChopObject:   append([]string(nil), stage.SpawnChopObject...),
			SpawnChopItem:     append([]string(nil), stage.SpawnChopItem...),
			Take:              take,
			TransformToDefKey: stage.TransformToDefKey,
		})
	}
	d.TreeConfig = &TreeBehaviorConfig{
		Priority: cfg.Priority,
		Stages:   stages,
	}
}
