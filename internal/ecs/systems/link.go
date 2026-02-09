package systems

import (
	"math"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/types"
)

const linkMoveEpsilon = 0.0001

// LinkSystem manages player↔object links based on collision and movement.
// Priority 250: runs after CollisionSystem (200) and before TransformUpdateSystem (300).
type LinkSystem struct {
	ecs.BaseSystem
	eventBus *eventbus.EventBus
}

func NewLinkSystem(eventBus *eventbus.EventBus) *LinkSystem {
	return &LinkSystem{
		BaseSystem: ecs.NewBaseSystem("LinkSystem", 250),
		eventBus:   eventBus,
	}
}

func (s *LinkSystem) Update(w *ecs.World, dt float64) {
	links := ecs.GetResource[ecs.LinkedObjects](w)
	moved := ecs.GetResource[ecs.MovedEntities](w)
	charEntities := ecs.GetResource[ecs.CharacterEntities](w)

	s.cleanupDead(w, links)
	s.breakOnMovement(w, links, moved)
	s.linkOnCollision(w, links, moved, charEntities)
}

func (s *LinkSystem) cleanupDead(w *ecs.World, links *ecs.LinkedObjects) {
	for playerHandle, entry := range links.PlayerToObject {
		if !w.Alive(playerHandle) || !w.Alive(entry.ObjectHandle) {
			links.Unlink(playerHandle)
			s.publishBroken(w, entry, ecs.LinkBreakDespawn)
		}
	}
}

func (s *LinkSystem) breakOnMovement(w *ecs.World, links *ecs.LinkedObjects, moved *ecs.MovedEntities) {
	for i := 0; i < moved.Count; i++ {
		h := moved.Handles[i]
		if !w.Alive(h) {
			continue
		}

		if !s.movedThisTick(w, h, moved.IntentX[i], moved.IntentY[i]) {
			continue
		}

		if entry, ok := links.PlayerToObject[h]; ok {
			links.Unlink(h)
			s.publishBroken(w, entry, ecs.LinkBreakMoved)
		}

		if _, ok := links.ObjectToPlayers[h]; ok {
			entries := links.UnlinkAllFromObject(h)
			for _, entry := range entries {
				s.publishBroken(w, entry, ecs.LinkBreakMoved)
			}
		}
	}
}

func (s *LinkSystem) linkOnCollision(w *ecs.World, links *ecs.LinkedObjects, moved *ecs.MovedEntities, charEntities *ecs.CharacterEntities) {
	for i := 0; i < moved.Count; i++ {
		h := moved.Handles[i]
		if !w.Alive(h) {
			continue
		}

		playerID, ok := w.GetExternalID(h)
		if !ok {
			continue
		}
		if _, isPlayer := charEntities.Map[playerID]; !isPlayer {
			continue
		}

		collision, ok := ecs.GetComponent[components.CollisionResult](w, h)
		if !ok || !collision.HasCollision || collision.CollidedWith == 0 {
			continue
		}
		if collision.CollidedWith == playerID {
			continue
		}
		if _, isPlayer := charEntities.Map[collision.CollidedWith]; isPlayer {
			continue
		}

		objectHandle := w.GetHandleByEntityID(collision.CollidedWith)
		if objectHandle == types.InvalidHandle || !w.Alive(objectHandle) {
			continue
		}

		if current, ok := links.PlayerToObject[h]; ok {
			if current.ObjectHandle == objectHandle {
				continue
			}
			links.Unlink(h)
			s.publishBroken(w, current, ecs.LinkBreakRelink)
		}

		if links.Link(h, playerID, objectHandle, collision.CollidedWith) {
			s.publishCreated(w, h, playerID, objectHandle, collision.CollidedWith)
		}
	}
}

func (s *LinkSystem) movedThisTick(w *ecs.World, h types.Handle, intentX, intentY float64) bool {
	transform, hasTransform := ecs.GetComponent[components.Transform](w, h)
	if !hasTransform {
		return true
	}
	if collision, ok := ecs.GetComponent[components.CollisionResult](w, h); ok {
		return math.Abs(collision.FinalX-transform.X) > linkMoveEpsilon ||
			math.Abs(collision.FinalY-transform.Y) > linkMoveEpsilon
	}
	return math.Abs(intentX-transform.X) > linkMoveEpsilon ||
		math.Abs(intentY-transform.Y) > linkMoveEpsilon
}

func (s *LinkSystem) publishCreated(w *ecs.World, playerHandle types.Handle, playerID types.EntityID, objectHandle types.Handle, objectID types.EntityID) {
	if s.eventBus == nil {
		return
	}
	_ = s.eventBus.PublishSync(
		ecs.NewLinkCreatedEvent(playerID, playerHandle, objectID, objectHandle, w.Layer),
	)
}

func (s *LinkSystem) publishBroken(w *ecs.World, entry ecs.LinkEntry, reason ecs.LinkBreakReason) {
	if s.eventBus == nil {
		return
	}
	_ = s.eventBus.PublishSync(
		ecs.NewLinkBrokenEvent(entry.PlayerID, entry.PlayerHandle, entry.ObjectID, entry.ObjectHandle, w.Layer, reason),
	)
}
