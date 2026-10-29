// Package combatpresentation publishes accepted attack warnings independently of
// sprite animation bindings. All calls require the owning shard's world lock.
package combatpresentation

import (
	"fmt"
	"math"
	"time"

	"origin/internal/actiondefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

type Sender interface {
	SendCombatTelegraph(types.Handle)
}

type record struct {
	identity                     types.EntityID
	generation                   string
	revision, execution, started uint64
	total                        uint32
	aim                          float64
	sector                       actiondefs.Sector
	active                       bool
}

// State holds one retained revision per admitted NPC. It is bounded by live
// entities and retired synchronously; it consumes no ECS component ID.
type State struct {
	records map[types.Handle]record
	sender  Sender
}

func Attach(w *ecs.World, sender Sender) {
	if _, exists := ecs.TryGetResource[*State](w); exists {
		panic("combat presentation already attached")
	}
	s := &State{records: make(map[types.Handle]record, 64), sender: sender}
	ecs.SetResource(w, s)
	prepare := func(h types.Handle) {
		if !ecs.HasComponent[components.NPCBrain](w, h) {
			return
		}
		if _, exists := s.records[h]; exists {
			return
		}
		if id, ok := w.GetExternalID(h); ok && w.Alive(h) {
			s.records[h] = record{identity: id, generation: fmt.Sprintf("%d:%d", w.Layer, h)}
		}
	}
	w.AddComponentObserver(components.NPCBrainComponentID, prepare)
	w.AddDespawnObserver(func(h types.Handle) { delete(s.records, h) })
	w.Query().With(components.NPCBrainComponentID).ForEach(prepare)
}

// Start does not replace a warning for the same or an older execution. Reserve
// two revisions so an accepted warning can always publish its terminal clear.
func Start(w *ecs.World, actor types.Handle, execution uint64, aim float64, sector actiondefs.Sector, total uint32) bool {
	resource, ok := ecs.TryGetResource[*State](w)
	if !ok || execution == 0 || total == 0 || !w.Alive(actor) || math.IsNaN(aim) || aim < 0 || aim >= 2*math.Pi ||
		!finitePositive(sector.Range) || sector.Range > math.MaxFloat32 || !finitePositive(sector.Angle) || sector.Angle > 2*math.Pi {
		return false
	}
	s := *resource
	r, exists := s.records[actor]
	id, identified := w.GetExternalID(actor)
	if !exists || !identified || id != r.identity || execution <= r.execution || r.revision > math.MaxUint64-2 {
		return false
	}
	r.revision++
	r.execution, r.started, r.total = execution, ecs.GetResource[ecs.TimeState](w).Tick, total
	r.aim, r.sector, r.active = aim, sector, true
	s.records[actor] = r
	if s.sender != nil {
		s.sender.SendCombatTelegraph(actor)
	}
	return true
}

func Stop(w *ecs.World, actor types.Handle, execution uint64) {
	resource, ok := ecs.TryGetResource[*State](w)
	if !ok {
		return
	}
	s := *resource
	r, exists := s.records[actor]
	if !exists || !r.active || execution != r.execution {
		return
	}
	r.revision++
	r.active = false
	s.records[actor] = r
	if s.sender != nil {
		s.sender.SendCombatTelegraph(actor)
	}
}

// Snapshot returns owned data, including an inactive incarnation for every NPC.
// The caller may serialize it after releasing the world read lock.
func Snapshot(w *ecs.World, actor types.Handle) *netproto.CombatTelegraphState {
	if !w.Alive(actor) || !ecs.HasComponent[components.NPCBrain](w, actor) {
		return nil
	}
	ts := ecs.GetResource[ecs.TimeState](w)
	state := &netproto.CombatTelegraphState{Generation: fmt.Sprintf("%d:%d", w.Layer, actor), ServerTimeMs: ts.UnixMs}
	resource, ok := ecs.TryGetResource[*State](w)
	if !ok {
		return state
	}
	r, exists := (*resource).records[actor]
	if !exists {
		return state
	}
	state.Generation, state.Revision, state.ExecutionGeneration = r.generation, r.revision, r.execution
	if !r.active || ts.TickPeriod <= 0 {
		return state
	}
	elapsed := uint64(0)
	if ts.Tick >= r.started {
		elapsed = ts.Tick - r.started
	}
	// The warning expires even if a delayed visibility snapshot arrives before
	// the terminal packet. The executor remains the authority for damage timing.
	if elapsed >= uint64(r.total) {
		return state
	}
	state.Active, state.TotalTicks, state.ElapsedTicks = true, r.total, uint32(elapsed)
	state.TickDurationMs = float64(ts.TickPeriod) / float64(time.Millisecond)
	state.FacingAngle, state.Range, state.Angle = float32(r.aim), float32(r.sector.Range), float32(r.sector.Angle)
	return state
}

func finitePositive(v float64) bool {
	return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0)
}
