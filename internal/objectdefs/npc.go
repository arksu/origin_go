package objectdefs

import "origin/internal/game/behaviors/contracts"

type NPCBehaviorConfig = contracts.NPCBehaviorConfig

func (d *ObjectDef) SetNPCBehaviorConfig(cfg contracts.NPCBehaviorConfig) {
	d.NPCConfig = &cfg
}
