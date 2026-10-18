package objectdefs

import "origin/internal/game/behaviors/contracts"

type WildWeedBehaviorConfig = contracts.WildWeedBehaviorConfig

func (d *ObjectDef) SetWildWeedBehaviorConfig(cfg contracts.WildWeedBehaviorConfig) {
	d.WildWeedConfig = &cfg
}
