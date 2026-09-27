package systems

import (
	"go.uber.org/zap"
	"origin/internal/cyclicaction"
	"origin/internal/ecs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

type ActionAnimationSender interface {
	SendActionAnimation(observerID, entityID types.EntityID, state *netproto.CharacterActionAnimationState)
}

type ActionAnimationSystem struct {
	ecs.BaseSystem
	sender ActionAnimationSender
	logger *zap.Logger
	batch  []types.Handle
}

func NewActionAnimationSystem(sender ActionAnimationSender, logger *zap.Logger) *ActionAnimationSystem {
	return &ActionAnimationSystem{BaseSystem: ecs.NewBaseSystem("ActionAnimationSystem", 491), sender: sender, logger: logger}
}

func (system *ActionAnimationSystem) Update(w *ecs.World, _ float64) {
	system.batch = ecs.GetResource[ecs.ActionAnimationDirtyQueue](w).Drain(0, system.batch[:0])
	for _, handle := range system.batch {
		state, err := cyclicaction.Snapshot(w, handle)
		if err != nil {
			system.logger.Error("Unable to build action animation state", zap.Uint64("handle", uint64(handle)), zap.Error(err))
			continue
		}
		if state == nil {
			continue
		}
		entityID, ok := w.GetExternalID(handle)
		if !ok {
			continue
		}
		system.sender.SendActionAnimation(entityID, entityID, state)
		visibility := ecs.GetResource[ecs.VisibilityState](w)
		visibility.Mu.RLock()
		for observer := range visibility.ObserversByVisibleTarget[handle] {
			if observer == handle || !w.Alive(observer) {
				continue
			}
			if observerID, ok := w.GetExternalID(observer); ok {
				system.sender.SendActionAnimation(observerID, entityID, state)
			}
		}
		visibility.Mu.RUnlock()
	}
}
