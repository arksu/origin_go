package game

import (
	"fmt"

	"origin/internal/actiondefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	"origin/internal/types"
)

// CombatDefinitions is prepared once before shard startup. Its selectors and
// catalog are immutable and can be shared by worlds with independent equipment.
type CombatDefinitions struct {
	catalog *CombatEquipmentCatalog
	actions map[string]*PreparedMeleeAction
}

func NewCombatDefinitions(items *itemdefs.Registry, actions *actiondefs.Registry) (*CombatDefinitions, error) {
	if actions == nil {
		return nil, fmt.Errorf("combat definitions require an action registry")
	}
	catalog, err := NewCombatEquipmentCatalog(items)
	if err != nil {
		return nil, err
	}
	definitions := &CombatDefinitions{catalog: catalog, actions: make(map[string]*PreparedMeleeAction)}
	for _, definition := range actions.All() {
		if definition.Combat == nil {
			continue
		}
		prepared, err := catalog.PrepareMeleeAction(definition)
		if err != nil {
			return nil, fmt.Errorf("prepare combat action %q: %w", definition.ID, err)
		}
		definitions.actions[definition.ID] = prepared
	}
	return definitions, nil
}

// prepareCreatureCombatTarget runs at successful setup/attachment, before a
// body is exposed to combat. Death continues through the existing lifecycle.
func (s *Shard) prepareCreatureCombatTarget(target types.Handle) error {
	if s.creatureDamage == nil {
		return nil // Lightweight test/tool shards may not run combat.
	}
	health, exists := ecs.GetComponent[components.EntityHealth](s.world, target)
	if !exists {
		return ErrInvalidCreatureTarget
	}
	if health.HHP == 0 {
		_, _, err := s.creatureDamage.readTarget(target)
		if err == ErrCreatureTargetDead {
			return nil
		}
		return err
	}
	return s.creatureDamage.PrepareTarget(target)
}
