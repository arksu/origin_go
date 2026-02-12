package game

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
)

const cyclicActionSystemPriority = 315

type cyclicActionProgressSender interface {
	SendCyclicActionProgress(entityID types.EntityID, progress *netproto.S2C_CyclicActionProgress)
}

type CyclicActionSystem struct {
	ecs.BaseSystem
	contextActions *ContextActionService
	progressSender cyclicActionProgressSender
	logger         *zap.Logger
	query          *ecs.PreparedQuery
	handles        []types.Handle
}

func NewCyclicActionSystem(
	contextActions *ContextActionService,
	progressSender cyclicActionProgressSender,
	logger *zap.Logger,
) *CyclicActionSystem {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CyclicActionSystem{
		BaseSystem:     ecs.NewBaseSystem("CyclicActionSystem", cyclicActionSystemPriority),
		contextActions: contextActions,
		progressSender: progressSender,
		logger:         logger,
		handles:        make([]types.Handle, 0, 128),
	}
}

func (s *CyclicActionSystem) Update(w *ecs.World, dt float64) {
	_ = dt
	if s.contextActions == nil {
		return
	}

	if s.query == nil {
		s.query = ecs.NewPreparedQuery(
			w,
			(1<<ecs.ExternalIDComponentID)|
				(1<<components.ActiveCyclicActionComponentID),
			0,
		)
	}

	s.handles = s.handles[:0]
	s.query.ForEach(func(h types.Handle) {
		s.handles = append(s.handles, h)
	})
	if len(s.handles) == 0 {
		return
	}

	linkState := ecs.GetResource[ecs.LinkState](w)
	nowTick := ecs.GetResource[ecs.TimeState](w).Tick

	for _, playerHandle := range s.handles {
		if playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
			continue
		}

		playerExternalID, hasExternalID := ecs.GetComponent[ecs.ExternalID](w, playerHandle)
		action, hasAction := ecs.GetComponent[components.ActiveCyclicAction](w, playerHandle)
		if !hasExternalID || !hasAction {
			continue
		}
		playerID := playerExternalID.ID

		if action.CycleDurationTicks == 0 {
			s.contextActions.cancelActiveCyclicAction(playerID, playerHandle, "invalid_action_state")
			continue
		}

		if action.TargetKind == components.CyclicActionTargetObject {
			if !isLinkedToTarget(linkState, playerID, action.TargetID) {
				s.contextActions.cancelActiveCyclicAction(playerID, playerHandle, "link_broken")
				continue
			}
		}

		action.CycleElapsedTicks++
		if action.CycleElapsedTicks > action.CycleDurationTicks {
			action.CycleElapsedTicks = action.CycleDurationTicks
		}

		s.sendProgress(playerID, action)

		if action.CycleElapsedTicks < action.CycleDurationTicks {
			ecs.WithComponent(w, playerHandle, func(active *components.ActiveCyclicAction) {
				active.CycleElapsedTicks = action.CycleElapsedTicks
			})
			continue
		}

		ecs.WithComponent(w, playerHandle, func(active *components.ActiveCyclicAction) {
			active.CycleElapsedTicks = action.CycleDurationTicks
		})
		s.contextActions.emitCycleSound(action)

		decision := s.contextActions.handleCyclicCycleComplete(w, playerID, playerHandle, action)
		switch decision {
		case cyclicActionDecisionContinue:
			ecs.WithComponent(w, playerHandle, func(active *components.ActiveCyclicAction) {
				active.CycleElapsedTicks = 0
				active.CycleIndex++
				active.StartedTick = nowTick
			})
		case cyclicActionDecisionComplete, cyclicActionDecisionCanceled:
			if decision == cyclicActionDecisionComplete {
				s.contextActions.completeActiveCyclicAction(playerID, playerHandle)
				continue
			}
			s.contextActions.cancelActiveCyclicAction(playerID, playerHandle, "")
		default:
			s.contextActions.cancelActiveCyclicAction(playerID, playerHandle, "invalid_action_state")
		}
	}
}

func (s *CyclicActionSystem) sendProgress(playerID types.EntityID, action components.ActiveCyclicAction) {
	if s.progressSender == nil || playerID == 0 || action.CycleDurationTicks == 0 {
		return
	}

	progress := &netproto.S2C_CyclicActionProgress{
		ActionId:       action.ActionID,
		TargetEntityId: uint64(action.TargetID),
		CycleIndex:     action.CycleIndex,
		ElapsedTicks:   action.CycleElapsedTicks,
		TotalTicks:     action.CycleDurationTicks,
	}
	s.progressSender.SendCyclicActionProgress(playerID, progress)
}

func isLinkedToTarget(linkState *ecs.LinkState, playerID, targetID types.EntityID) bool {
	if linkState == nil || playerID == 0 || targetID == 0 {
		return false
	}
	link, hasLink := linkState.GetLink(playerID)
	if !hasLink {
		return false
	}
	return link.TargetID == targetID
}
