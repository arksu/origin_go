package objectdefs

import "origin/internal/game/behaviors/contracts"

type CropBehaviorConfig = contracts.CropBehaviorConfig
type CropStageConfig = contracts.CropStageConfig
type CropHarvestProduct = contracts.CropHarvestProduct

func (d *ObjectDef) SetCropBehaviorConfig(cfg contracts.CropBehaviorConfig) {
	if d == nil {
		return
	}
	d.CropConfig = &cfg
	// Generic collider defaults make unspecified layers solid. Crops retain a
	// damage-target collider while remaining passable at every growth stage.
	if d.Components != nil && d.Components.Collider != nil {
		d.Components.Collider.Layer = 0
		d.Components.Collider.Mask = 0
	}
	d.ContextMenuEvenForOneItemValue = false
}
