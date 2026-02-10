package systems

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/types"

	"go.uber.org/zap"
)

const linkMovementEpsilon = 0.001

// LinkSystem maintains player<->object links.
//
// Rules:
// - link is created only by explicit intent + confirmed collision with target
// - one player can have only one active link
// - link breaks strictly on movement, collision switch, relink, or despawn
type breakCandidate struct {
	playerID types.EntityID
	reason   ecs.LinkBreakReason
}

type LinkSystem struct {
	ecs.BaseSystem
	eventBus  *eventbus.EventBus
	logger    *zap.Logger
	epsilonSq float64
	toBreak   []breakCandidate
}

func NewLinkSystem(eventBus *eventbus.EventBus, logger *zap.Logger) *LinkSystem {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &LinkSystem{
		BaseSystem: ecs.NewBaseSystem("LinkSystem", 310),
		eventBus:   eventBus,
		logger:     logger,
		epsilonSq:  linkMovementEpsilon * linkMovementEpsilon,
	}
}

func (s *LinkSystem) Update(w *ecs.World, dt float64) {
	linkState := ecs.GetResource[ecs.LinkState](w)

	s.processIntents(w, linkState)
	s.validateActiveLinks(w, linkState)
}

func (s *LinkSystem) processIntents(w *ecs.World, linkState *ecs.LinkState) {
	for playerID, intent := range linkState.IntentByPlayer {
		playerHandle := w.GetHandleByEntityID(playerID)
		if playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
			linkState.ClearIntent(playerID)
			continue
		}

		targetHandle := intent.TargetHandle
		if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
			targetHandle = w.GetHandleByEntityID(intent.TargetID)
			if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
				linkState.ClearIntent(playerID)
				continue
			}
			intent.TargetHandle = targetHandle
			linkState.IntentByPlayer[playerID] = intent
		}

		if s.lastCollidedWith(w, playerHandle) != intent.TargetID {
			continue
		}

		playerTransform, hasPlayerTransform := ecs.GetComponent[components.Transform](w, playerHandle)
		targetTransform, hasTargetTransform := ecs.GetComponent[components.Transform](w, targetHandle)
		if !hasPlayerTransform || !hasTargetTransform {
			linkState.ClearIntent(playerID)
			continue
		}

		s.linkPlayerToTarget(
			w,
			linkState,
			playerID,
			playerHandle,
			intent.TargetID,
			targetHandle,
			playerTransform,
			targetTransform,
		)
		linkState.ClearIntent(playerID)
	}
}

func (s *LinkSystem) validateActiveLinks(w *ecs.World, linkState *ecs.LinkState) {
	s.toBreak = s.toBreak[:0]

	for playerID, link := range linkState.LinkedByPlayer {
		playerHandle := w.GetHandleByEntityID(playerID)
		if playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
			s.toBreak = append(s.toBreak, breakCandidate{playerID: playerID, reason: ecs.LinkBreakDespawn})
			continue
		}
		// Handle changed means despawn/respawn happened under same EntityID.
		if link.PlayerHandle != types.InvalidHandle && playerHandle != link.PlayerHandle {
			s.toBreak = append(s.toBreak, breakCandidate{playerID: playerID, reason: ecs.LinkBreakDespawn})
			continue
		}

		targetHandle := w.GetHandleByEntityID(link.TargetID)
		if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
			s.toBreak = append(s.toBreak, breakCandidate{playerID: playerID, reason: ecs.LinkBreakDespawn})
			continue
		}
		if link.TargetHandle != types.InvalidHandle && targetHandle != link.TargetHandle {
			s.toBreak = append(s.toBreak, breakCandidate{playerID: playerID, reason: ecs.LinkBreakDespawn})
			continue
		}

		playerTransform, hasPlayerTransform := ecs.GetComponent[components.Transform](w, playerHandle)
		targetTransform, hasTargetTransform := ecs.GetComponent[components.Transform](w, targetHandle)
		if !hasPlayerTransform || !hasTargetTransform {
			s.toBreak = append(s.toBreak, breakCandidate{playerID: playerID, reason: ecs.LinkBreakDespawn})
			continue
		}

		// Any factual movement breaks the link.
		if movedBeyond(link.PlayerX, link.PlayerY, playerTransform.X, playerTransform.Y, s.epsilonSq) ||
			movedBeyond(link.TargetX, link.TargetY, targetTransform.X, targetTransform.Y, s.epsilonSq) {
			s.toBreak = append(s.toBreak, breakCandidate{playerID: playerID, reason: ecs.LinkBreakMoved})
			continue
		}

		// Collision target must still be the linked target.
		if s.lastCollidedWith(w, playerHandle) != link.TargetID {
			s.toBreak = append(s.toBreak, breakCandidate{playerID: playerID, reason: ecs.LinkBreakMoved})
		}
	}

	for _, candidate := range s.toBreak {
		s.breakLink(w, linkState, candidate.playerID, candidate.reason)
	}
}

func movedBeyond(prevX, prevY, currentX, currentY, epsilonSq float64) bool {
	dx := currentX - prevX
	dy := currentY - prevY
	return dx*dx+dy*dy > epsilonSq
}

func (s *LinkSystem) lastCollidedWith(w *ecs.World, playerHandle types.Handle) types.EntityID {
	cr, ok := ecs.GetComponent[components.CollisionResult](w, playerHandle)
	if !ok {
		return 0
	}
	if cr.PrevCollidedWith != 0 {
		return cr.PrevCollidedWith
	}
	if cr.HasCollision && cr.CollidedWith != 0 {
		return cr.CollidedWith
	}
	return 0
}

func (s *LinkSystem) linkPlayerToTarget(
	w *ecs.World,
	linkState *ecs.LinkState,
	playerID types.EntityID,
	playerHandle types.Handle,
	targetID types.EntityID,
	targetHandle types.Handle,
	playerTransform components.Transform,
	targetTransform components.Transform,
) {
	if existingLink, hasLink := linkState.GetLink(playerID); hasLink {
		if existingLink.TargetID == targetID {
			// Refresh snapshots if linking again to the same target.
			linkState.SetLink(ecs.PlayerLink{
				PlayerID:     playerID,
				PlayerHandle: playerHandle,
				TargetID:     targetID,
				TargetHandle: targetHandle,
				PlayerX:      playerTransform.X,
				PlayerY:      playerTransform.Y,
				TargetX:      targetTransform.X,
				TargetY:      targetTransform.Y,
				CreatedAt:    ecs.GetResource[ecs.TimeState](w).Now,
			})
			return
		}
		s.breakLink(w, linkState, playerID, ecs.LinkBreakRelink)
	}

	linkState.SetLink(ecs.PlayerLink{
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     targetID,
		TargetHandle: targetHandle,
		PlayerX:      playerTransform.X,
		PlayerY:      playerTransform.Y,
		TargetX:      targetTransform.X,
		TargetY:      targetTransform.Y,
		CreatedAt:    ecs.GetResource[ecs.TimeState](w).Now,
	})

	// Player must stop at contact point; otherwise continued path-following
	// immediately breaks link on movement delta in subsequent ticks.
	var stopMoveEntry *ecs.MoveBatchEntry
	ecs.MutateComponent[components.Movement](w, playerHandle, func(m *components.Movement) bool {
		stopMoveEntry = &ecs.MoveBatchEntry{
			EntityID:     playerID,
			Handle:       playerHandle,
			X:            int(playerTransform.X),
			Y:            int(playerTransform.Y),
			Heading:      playerTransform.Direction,
			VelocityX:    0,
			VelocityY:    0,
			MoveMode:     m.Mode,
			IsMoving:     false,
			ServerTimeMs: ecs.GetResource[ecs.TimeState](w).UnixMs,
			MoveSeq:      m.MoveSeq,
			IsTeleport:   false,
		}
		m.ClearTarget()
		m.MoveSeq++
		return true
	})

	if stopMoveEntry != nil {
		s.eventBus.PublishAsync(
			ecs.NewObjectMoveBatchEvent(w.Layer, []ecs.MoveBatchEntry{*stopMoveEntry}),
			eventbus.PriorityMedium,
		)
	}

	s.publishSync(w, ecs.NewLinkCreatedEvent(w.Layer, playerID, targetID))
}

func (s *LinkSystem) breakLink(w *ecs.World, linkState *ecs.LinkState, playerID types.EntityID, reason ecs.LinkBreakReason) {
	link, removed := linkState.RemoveLink(playerID)
	if !removed {
		return
	}

	linkState.ClearIntent(playerID)
	s.publishSync(w, ecs.NewLinkBrokenEvent(w.Layer, playerID, link.TargetID, reason))
}

func (s *LinkSystem) publishSync(w *ecs.World, event eventbus.Event) {
	if s.eventBus == nil {
		return
	}
	if err := s.eventBus.PublishSync(event); err != nil {
		s.logger.Warn("LinkSystem PublishSync failed",
			zap.Error(err),
			zap.Int("layer", w.Layer),
			zap.String("topic", event.Topic()),
		)
	}
}
