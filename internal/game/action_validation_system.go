package game

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

const actionValidationSystemPriority = 314

type ActionValidationSystem struct {
	ecs.BaseSystem
	service *ActionService
	query   *ecs.PreparedQuery
}

func NewActionValidationSystem(world *ecs.World, service *ActionService) *ActionValidationSystem {
	return &ActionValidationSystem{
		BaseSystem: ecs.NewBaseSystem("ActionValidationSystem", actionValidationSystemPriority),
		service:    service,
		query:      ecs.NewPreparedQuery(world, (1<<ecs.ExternalIDComponentID)|(1<<components.CharacterProfileComponentID), 0),
	}
}

func (system *ActionValidationSystem) Update(world *ecs.World, _ float64) {
	if system.service == nil {
		return
	}
	detached := ecs.GetResource[ecs.DetachedEntities](world)
	system.query.ForEach(func(handle types.Handle) {
		playerID, exists := world.GetExternalID(handle)
		if exists && !detached.IsDetached(playerID) {
			system.service.Recheck(world, playerID, handle)
		}
	})
}
