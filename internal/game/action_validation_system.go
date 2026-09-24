package game

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

const actionValidationSystemPriority = 314

type ActionValidationSystem struct {
	ecs.BaseSystem
	service       *ActionService
	query         *ecs.PreparedQuery
	activeHandles []types.Handle
}

func NewActionValidationSystem(world *ecs.World, service *ActionService) *ActionValidationSystem {
	return &ActionValidationSystem{
		BaseSystem: ecs.NewBaseSystem("ActionValidationSystem", actionValidationSystemPriority),
		service:    service,
		query:      ecs.NewPreparedQuery(world, (1<<ecs.ExternalIDComponentID)|(1<<components.ActiveGameActionComponentID), 0),
	}
}

func (system *ActionValidationSystem) Update(world *ecs.World, _ float64) {
	if system.service == nil {
		return
	}
	detached := ecs.GetResource[ecs.DetachedEntities](world)
	system.activeHandles = system.activeHandles[:0]
	// Recheck can remove ActiveGameAction, so avoid mutating query storage during iteration.
	system.query.ForEach(func(handle types.Handle) {
		system.activeHandles = append(system.activeHandles, handle)
	})
	for _, handle := range system.activeHandles {
		playerID, exists := world.GetExternalID(handle)
		if exists && world.Alive(handle) && !detached.IsDetached(playerID) {
			system.service.Recheck(world, playerID, handle)
		}
	}
}
