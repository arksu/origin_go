package game

import (
	"fmt"
	"math"
	"origin/internal/combat"
	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

type CombatHit struct {
	EventSequence       uint64
	ExecutionID         uint64
	AttackerID          types.EntityID
	AttackerIncarnation types.Handle
	TargetID            types.EntityID
	TargetIncarnation   types.Handle
	RawDamage           float64
	Damage              float64
	TimeMs              int64
}

// CombatReceiver implementations must validate before mutation and never partially
// apply a failed hit. Their health rules are independent of targeting and sessions.
type CombatReceiver interface {
	Eligible() bool
	Armor() (float64, error)
	ApplyHit(CombatHit) error
}

type combatReceiverEntry struct {
	receiver  CombatReceiver
	lastEvent uint64
}

type combatChunks interface {
	GetChunk(types.ChunkCoord) *core.Chunk
}

type CombatReceivers struct {
	world         *ecs.World
	chunks        combatChunks
	entries       map[types.Handle]*combatReceiverEntry
	maxHalfWidth  float64
	maxHalfHeight float64
}

func NewCombatReceivers(world *ecs.World, chunks combatChunks) *CombatReceivers {
	return &CombatReceivers{world: world, chunks: chunks, entries: make(map[types.Handle]*combatReceiverEntry)}
}

func (receivers *CombatReceivers) Register(handle types.Handle, receiver CombatReceiver) error {
	if receiver == nil || receivers.world == nil || !receivers.world.Alive(handle) {
		return fmt.Errorf("combat receiver requires a live entity and adapter")
	}
	if _, exists := receivers.entries[handle]; exists {
		return fmt.Errorf("combat receiver already registered")
	}
	if err := receivers.UpdateExtent(handle); err != nil {
		return err
	}
	receivers.entries[handle] = &combatReceiverEntry{receiver: receiver}
	return nil
}

// Call before making a larger collider hittable. Retaining old maxima is conservative.
func (receivers *CombatReceivers) UpdateExtent(handle types.Handle) error {
	collider, exists := ecs.GetComponent[components.Collider](receivers.world, handle)
	if !exists || math.IsNaN(collider.HalfWidth) || math.IsNaN(collider.HalfHeight) || math.IsInf(collider.HalfWidth, 0) || math.IsInf(collider.HalfHeight, 0) || collider.HalfWidth < 0 || collider.HalfHeight < 0 {
		return fmt.Errorf("combat receiver requires finite non-negative collider extents")
	}
	receivers.maxHalfWidth = math.Max(receivers.maxHalfWidth, collider.HalfWidth)
	receivers.maxHalfHeight = math.Max(receivers.maxHalfHeight, collider.HalfHeight)
	return nil
}

func (receivers *CombatReceivers) Unregister(handle types.Handle) { delete(receivers.entries, handle) }

func (receivers *CombatReceivers) eligible(candidate combat.Candidate) bool {
	entry := receivers.entries[candidate.Incarnation]
	id, alive := receivers.world.GetExternalID(candidate.Incarnation)
	return entry != nil && alive && id == candidate.ID && entry.receiver.Eligible()
}

func (receivers *CombatReceivers) Contacts(actorID types.EntityID, sector combat.Sector, nearest bool) ([]combat.Contact, error) {
	if err := sector.Validate(); err != nil {
		return nil, err
	}
	if receivers.chunks == nil {
		return nil, fmt.Errorf("combat spatial queries unavailable")
	}
	bounds := sector.Bounds()
	minX := math.Floor(bounds.Min.X - receivers.maxHalfWidth - combat.GeometryTolerance)
	minY := math.Floor(bounds.Min.Y - receivers.maxHalfHeight - combat.GeometryTolerance)
	maxX := math.Ceil(bounds.Max.X + receivers.maxHalfWidth + combat.GeometryTolerance)
	maxY := math.Ceil(bounds.Max.Y + receivers.maxHalfHeight + combat.GeometryTolerance)
	// Chunk coordinates and grid queries use ints. Reject invalid bounds before conversion.
	for _, coordinate := range []float64{minX, minY, maxX, maxY} {
		if math.IsInf(coordinate, 0) || coordinate <= float64(math.MinInt) || coordinate >= float64(math.MaxInt) {
			return nil, fmt.Errorf("combat query bounds overflow")
		}
	}
	minChunkX, maxChunkX := int(math.Floor(minX/constt.ChunkWorldSize)), int(math.Floor(maxX/constt.ChunkWorldSize))
	minChunkY, maxChunkY := int(math.Floor(minY/constt.ChunkWorldSize)), int(math.Floor(maxY/constt.ChunkWorldSize))
	handles := make([]types.Handle, 0)
	for chunkY := minChunkY; chunkY <= maxChunkY; chunkY++ {
		for chunkX := minChunkX; chunkX <= maxChunkX; chunkX++ {
			chunk := receivers.chunks.GetChunk(types.ChunkCoord{X: chunkX, Y: chunkY})
			if chunk != nil {
				chunk.Spatial().QueryAABB(int(minX), int(minY), int(maxX), int(maxY), &handles)
			}
		}
	}
	candidates := make([]combat.Candidate, 0, len(handles))
	seen := make(map[types.Handle]struct{}, len(handles))
	for _, handle := range handles {
		if _, exists := seen[handle]; exists {
			continue
		}
		seen[handle] = struct{}{}
		id, _ := receivers.world.GetExternalID(handle)
		candidate := combat.Candidate{ID: id, Incarnation: handle}
		if candidate.ID == actorID || !receivers.eligible(candidate) {
			continue
		}
		transform, hasTransform := ecs.GetComponent[components.Transform](receivers.world, handle)
		collider, hasCollider := ecs.GetComponent[components.Collider](receivers.world, handle)
		if !hasTransform || !hasCollider {
			continue
		}
		candidate.Bounds = combat.AABB{Min: combat.Point{X: transform.X - collider.HalfWidth, Y: transform.Y - collider.HalfHeight}, Max: combat.Point{X: transform.X + collider.HalfWidth, Y: transform.Y + collider.HalfHeight}}
		candidates = append(candidates, candidate)
	}
	return combat.Select(actorID, sector, candidates, nearest, receivers.eligible)
}

func (receivers *CombatReceivers) Apply(hit CombatHit) (CombatHit, bool, error) {
	if hit.EventSequence == 0 || hit.ExecutionID == 0 || hit.AttackerID == 0 || hit.AttackerIncarnation == types.InvalidHandle || hit.AttackerID == hit.TargetID {
		return hit, false, fmt.Errorf("invalid combat hit identity")
	}
	candidate := combat.Candidate{ID: hit.TargetID, Incarnation: hit.TargetIncarnation}
	if !receivers.eligible(candidate) {
		return hit, false, nil
	}
	entry := receivers.entries[hit.TargetIncarnation]
	if hit.EventSequence <= entry.lastEvent {
		return hit, false, nil
	}
	armor, err := entry.receiver.Armor()
	if err != nil {
		return hit, false, err
	}
	hit.Damage, err = combat.ArmorDamage(hit.RawDamage, armor)
	if err != nil {
		return hit, false, err
	}
	if err := entry.receiver.ApplyHit(hit); err != nil {
		return hit, false, err
	}
	entry.lastEvent = hit.EventSequence
	return hit, true, nil
}
