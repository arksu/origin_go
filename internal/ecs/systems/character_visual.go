package systems

import (
	"origin/internal/charactervisual"
	"origin/internal/ecs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
)

type CharacterVisualSender interface {
	SendCharacterVisual(observerID, entityID types.EntityID, state *netproto.CharacterVisualState)
}

type CharacterVisualSystem struct {
	ecs.BaseSystem
	sender CharacterVisualSender
	logger *zap.Logger
	batch  []types.Handle
}

func NewCharacterVisualSystem(sender CharacterVisualSender, logger *zap.Logger) *CharacterVisualSystem {
	return &CharacterVisualSystem{BaseSystem: ecs.NewBaseSystem("CharacterVisualSystem", 490), sender: sender, logger: logger}
}

func (s *CharacterVisualSystem) Update(w *ecs.World, dt float64) {
	s.batch = ecs.GetResource[ecs.CharacterVisualDirtyQueue](w).Drain(0, s.batch[:0])
	for _, handle := range s.batch {
		state, err := charactervisual.Snapshot(w, handle)
		if err != nil {
			s.logger.Error("Unable to build character visual", zap.Uint64("handle", uint64(handle)), zap.Error(err))
			continue
		}
		if state != nil {
			entityID, _ := w.GetExternalID(handle)
			visibility := ecs.GetResource[ecs.VisibilityState](w)
			visibility.Mu.RLock()
			for observer := range visibility.ObserversByVisibleTarget[handle] {
				if observerID, ok := w.GetExternalID(observer); ok && w.Alive(observer) {
					s.sender.SendCharacterVisual(observerID, entityID, state)
				}
			}
			visibility.Mu.RUnlock()
		}
	}
}
