package objectdefs

import "origin/internal/game/behaviors/contracts"

// SetTreeBehaviorConfig applies validated tree behavior config onto object def.
func (d *ObjectDef) SetTreeBehaviorConfig(cfg contracts.TreeBehaviorConfig) {
	if d == nil {
		return
	}
	stages := make([]TreeStageConfig, 0, len(cfg.Stages))
	for _, stage := range cfg.Stages {
		take := make([]TakeConfig, 0, len(stage.Take))
		for _, entry := range stage.Take {
			take = append(take, TakeConfig{
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

// SetTakeBehaviorConfig applies validated take behavior config onto object def.
func (d *ObjectDef) SetTakeBehaviorConfig(cfg contracts.TakeBehaviorConfig) {
	if d == nil {
		return
	}
	items := make([]TakeConfig, 0, len(cfg.Items))
	for _, item := range cfg.Items {
		items = append(items, TakeConfig{
			ID:         item.ID,
			Name:       item.Name,
			ItemDefKey: item.ItemDefKey,
			Count:      item.Count,
		})
	}
	d.TakeConfig = &TakeBehaviorConfig{
		Priority: cfg.Priority,
		Items:    items,
	}
}
