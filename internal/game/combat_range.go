package game

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"origin/internal/combat"
	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/game/inventory"
	"origin/internal/types"
	"os"
	"time"
)

const combatRangeLimit = 8
const combatRangeRadius = 48.0

type combatRangePreset struct {
	Name       string  `json:"name"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	HalfWidth  float64 `json:"halfWidth"`
	HalfHeight float64 `json:"halfHeight"`
	HP         float64 `json:"hp"`
	AmplitudeY float64 `json:"amplitudeY"`
	PeriodMs   int64   `json:"periodMs"`
}
type combatRangeFixture struct {
	handle  types.Handle
	chunk   *core.Chunk
	preset  combatRangePreset
	x, y    float64
	moveSeq uint32
}
type combatRange struct {
	owner    types.Handle
	origin   combat.Point
	started  time.Time
	fixtures []*combatRangeFixture
	strength *float64
}

type CombatRangeService struct {
	ecs.BaseSystem
	world    *ecs.World
	combat   *CombatService
	chunks   combatChunks
	ids      inventory.EntityIDAllocator
	presets  map[string][]combatRangePreset
	ranges   map[types.Handle]*combatRange
	bus      *eventbus.EventBus
	OnTarget func(types.Handle)
	OnRemove func(types.Handle)
}

func NewCombatRangeService(w *ecs.World, service *CombatService, chunks combatChunks, ids inventory.EntityIDAllocator, path string, bus *eventbus.EventBus) (*CombatRangeService, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var presets map[string][]combatRangePreset
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&presets); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("%s: unexpected trailing JSON", path)
	}
	if len(presets) == 0 || len(presets) > 16 {
		return nil, fmt.Errorf("%s: expected 1..16 presets", path)
	}
	for key, entries := range presets {
		if key == "" || len(entries) == 0 || len(entries) > 8 {
			return nil, fmt.Errorf("%s: invalid preset %q", path, key)
		}
		for _, entry := range entries {
			if entry.Name == "" || len(entry.Name) > 48 || entry.HalfWidth <= 0 || entry.HalfHeight <= 0 || entry.HP < 0 || entry.HP > 10000 || entry.AmplitudeY < 0 || (entry.AmplitudeY > 0 && (entry.PeriodMs < 400 || entry.PeriodMs > 60000)) {
				return nil, fmt.Errorf("%s: invalid fixture in %q", path, key)
			}
			for _, value := range []float64{entry.X, entry.Y, entry.HalfWidth, entry.HalfHeight, entry.HP, entry.AmplitudeY} {
				if math.IsNaN(value) || math.IsInf(value, 0) {
					return nil, fmt.Errorf("%s: nonfinite fixture in %q", path, key)
				}
			}
			if math.Abs(entry.X)+entry.HalfWidth > combatRangeRadius || math.Abs(entry.Y)+entry.HalfHeight+entry.AmplitudeY > combatRangeRadius {
				return nil, fmt.Errorf("%s: fixture exceeds range bounds", path)
			}
		}
	}
	return &CombatRangeService{BaseSystem: ecs.NewBaseSystem("CombatRangeMotion", 305), world: w, combat: service, chunks: chunks, ids: ids, presets: presets, ranges: make(map[types.Handle]*combatRange), bus: bus}, nil
}

func rangeChunk(x, y float64) types.ChunkCoord {
	return types.ChunkCoord{X: int(math.Floor(x / constt.ChunkWorldSize)), Y: int(math.Floor(y / constt.ChunkWorldSize))}
}
func (service *CombatRangeService) available(owner types.Handle) error {
	if service == nil || service.combat == nil || !service.combat.enabled {
		return fmt.Errorf("combat testing is disabled")
	}
	if !service.world.Alive(owner) {
		return fmt.Errorf("player is unavailable")
	}
	return nil
}
func (service *CombatRangeService) Create(owner types.Handle, preset string) error {
	if err := service.available(owner); err != nil {
		return err
	}
	if _, exists := service.ranges[owner]; exists {
		return fmt.Errorf("remove your existing range first")
	}
	if len(service.ranges) >= combatRangeLimit {
		return fmt.Errorf("range limit reached (%d per shard)", combatRangeLimit)
	}
	entries, exists := service.presets[preset]
	if !exists {
		return fmt.Errorf("unknown preset: use small, large, tied, moving, or blocker")
	}
	transform, exists := ecs.GetComponent[components.Transform](service.world, owner)
	if !exists || math.IsNaN(transform.X) || math.IsNaN(transform.Y) || math.IsInf(transform.X, 0) || math.IsInf(transform.Y, 0) {
		return fmt.Errorf("player position unavailable")
	}
	if service.ids == nil {
		return fmt.Errorf("entity ID allocator unavailable")
	}
	if service.world.EntityCapacity()-service.world.EntityCount() < len(entries) {
		return fmt.Errorf("entity capacity exhausted")
	}
	layout := &combatRange{owner: owner, origin: combat.Point{X: transform.X, Y: transform.Y}, started: ecs.GetResource[ecs.TimeState](service.world).Now}
	min, max := rangeChunk(transform.X-combatRangeRadius, transform.Y-combatRangeRadius), rangeChunk(transform.X+combatRangeRadius, transform.Y+combatRangeRadius)
	for cy := min.Y; cy <= max.Y; cy++ {
		for cx := min.X; cx <= max.X; cx++ {
			chunk := service.chunks.GetChunk(types.ChunkCoord{X: cx, Y: cy})
			if chunk == nil || chunk.GetState() != types.ChunkStateActive {
				return fmt.Errorf("range patch must be in loaded active chunks")
			}
		}
	}
	// A rare setup command can scan actual colliders. A center-only grid query
	// would miss a large neighboring object whose bounds extend into this patch.
	colliders := ecs.NewQuery(service.world).With(ecs.GetComponentID[components.Transform]()).With(ecs.GetComponentID[components.Collider]())
	for _, other := range colliders.Handles() {
		if other == owner {
			continue
		}
		position, _ := ecs.GetComponent[components.Transform](service.world, other)
		collider, _ := ecs.GetComponent[components.Collider](service.world, other)
		if math.Abs(position.X-transform.X) <= combatRangeRadius+collider.HalfWidth && math.Abs(position.Y-transform.Y) <= combatRangeRadius+collider.HalfHeight {
			return fmt.Errorf("range patch overlaps an existing entity; move to clear ground")
		}
	}
	service.ranges[owner] = layout
	for _, entry := range entries {
		id := service.ids.GetFreeID()
		if id == 0 || service.world.Alive(service.world.GetHandleByEntityID(id)) {
			service.Remove(owner)
			return fmt.Errorf("invalid fixture entity ID")
		}
		x, y := transform.X+entry.X, transform.Y+entry.Y
		coord := rangeChunk(x, y)
		name := entry.Name
		handle := service.world.Spawn(id, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.Transform{X: x, Y: y})
			ecs.AddComponent(w, h, components.ChunkRef{CurrentChunkX: coord.X, CurrentChunkY: coord.Y, PrevChunkX: coord.X, PrevChunkY: coord.Y})
			ecs.AddComponent(w, h, components.EntityInfo{TypeID: 13, IsStatic: true, Layer: w.Layer})
			ecs.AddComponent(w, h, components.Appearance{Resource: "boulder", Name: &name})
			ecs.AddComponent(w, h, components.Collider{HalfWidth: entry.HalfWidth, HalfHeight: entry.HalfHeight, Layer: 1, Mask: 1})
			ecs.AddComponent(w, h, components.CombatTestTarget{HP: entry.HP, MaxHP: entry.HP, Revision: 1, Receiver: entry.HP > 0})
		})
		if handle == types.InvalidHandle {
			service.Remove(owner)
			return fmt.Errorf("entity capacity exhausted")
		}
		fixture := &combatRangeFixture{handle: handle, chunk: service.chunks.GetChunk(coord), preset: entry, x: x, y: y}
		layout.fixtures = append(layout.fixtures, fixture)
		fixture.chunk.Spatial().AddStatic(handle, int(x), int(y))
		if entry.HP > 0 {
			if err := service.combat.receivers.Register(handle, &combatRangeReceiver{service: service, handle: handle}); err != nil {
				service.Remove(owner)
				return err
			}
		}
	}
	return nil
}

func (service *CombatRangeService) Remove(owner types.Handle) {
	if service == nil {
		return
	}
	layout := service.ranges[owner]
	if layout == nil {
		return
	}
	for _, fixture := range layout.fixtures {
		service.combat.receivers.Unregister(fixture.handle)
		fixture.chunk.Spatial().RemoveStatic(fixture.handle, int(fixture.x), int(fixture.y))
		if service.OnRemove != nil {
			service.OnRemove(fixture.handle)
		}
		service.world.Despawn(fixture.handle)
	}
	delete(service.ranges, owner)
}

func (service *CombatRangeService) Reset(owner types.Handle) error {
	if err := service.available(owner); err != nil {
		return err
	}
	layout := service.ranges[owner]
	if layout == nil {
		return fmt.Errorf("create a range first")
	}
	layout.started = ecs.GetResource[ecs.TimeState](service.world).Now
	for _, fixture := range layout.fixtures {
		if !service.world.Alive(fixture.handle) {
			service.Remove(owner)
			return fmt.Errorf("range was unloaded; create it again")
		}
		if !service.move(fixture, layout.origin.X+fixture.preset.X, layout.origin.Y+fixture.preset.Y) {
			service.Remove(owner)
			return fmt.Errorf("range chunk was unloaded; create it again")
		}
		ecs.WithComponent(service.world, fixture.handle, func(target *components.CombatTestTarget) { target.HP = target.MaxHP; target.Revision++ })
		if service.OnTarget != nil {
			service.OnTarget(fixture.handle)
		}
	}
	return nil
}
func (service *CombatRangeService) Strength(w *ecs.World, actor types.Handle) (float64, error) {
	if layout := service.ranges[actor]; layout != nil && layout.strength != nil {
		return *layout.strength, nil
	}
	return baseCombatStrength(w, actor)
}
func (service *CombatRangeService) move(fixture *combatRangeFixture, x, y float64) bool {
	chunk := service.chunks.GetChunk(rangeChunk(x, y))
	if chunk == nil || chunk.GetState() != types.ChunkStateActive {
		return false
	}
	fixture.chunk.Spatial().RemoveStatic(fixture.handle, int(fixture.x), int(fixture.y))
	chunk.Spatial().AddStatic(fixture.handle, int(x), int(y))
	fixture.chunk, fixture.x, fixture.y = chunk, x, y
	ecs.WithComponent(service.world, fixture.handle, func(position *components.Transform) { position.X = x; position.Y = y })
	ecs.WithComponent(service.world, fixture.handle, func(ref *components.ChunkRef) {
		ref.CurrentChunkX = chunk.Coord.X
		ref.CurrentChunkY = chunk.Coord.Y
		ref.PrevChunkX = chunk.Coord.X
		ref.PrevChunkY = chunk.Coord.Y
	})
	fixture.moveSeq++
	if service.bus != nil {
		id, _ := service.world.GetExternalID(fixture.handle)
		service.bus.PublishAsync(ecs.NewObjectMoveBatchEvent(service.world.Layer, []ecs.MoveBatchEntry{{EntityID: id, Handle: fixture.handle, X: int(x), Y: int(y), MoveSeq: fixture.moveSeq, IsMoving: fixture.preset.AmplitudeY > 0, ServerTimeMs: ecs.GetResource[ecs.TimeState](service.world).UnixMs}}), eventbus.PriorityMedium)
	}
	return true
}
func (service *CombatRangeService) Update(w *ecs.World, _ float64) {
	for owner, layout := range service.ranges {
		if !w.Alive(owner) {
			service.Remove(owner)
			continue
		}
		elapsed := ecs.GetResource[ecs.TimeState](w).Now.Sub(layout.started).Seconds()
		for _, fixture := range layout.fixtures {
			if !w.Alive(fixture.handle) {
				service.Remove(owner)
				break
			}
			if fixture.preset.AmplitudeY == 0 {
				continue
			}
			y := layout.origin.Y + fixture.preset.Y + fixture.preset.AmplitudeY*math.Sin(2*math.Pi*elapsed/(float64(fixture.preset.PeriodMs)/1000))
			if !service.move(fixture, fixture.x, y) {
				service.Remove(owner)
				break
			}
		}
	}
}

type combatRangeReceiver struct {
	service *CombatRangeService
	handle  types.Handle
}

func (receiver *combatRangeReceiver) Eligible() bool {
	target, ok := ecs.GetComponent[components.CombatTestTarget](receiver.service.world, receiver.handle)
	return ok && target.Receiver && target.HP > 0
}
func (*combatRangeReceiver) Armor() (float64, error) { return 0, nil }
func (receiver *combatRangeReceiver) ApplyHit(hit CombatHit) error {
	if math.IsNaN(hit.Damage) || math.IsInf(hit.Damage, 0) || hit.Damage < 0 || !receiver.Eligible() {
		return fmt.Errorf("invalid range hit")
	}
	ecs.WithComponent(receiver.service.world, receiver.handle, func(target *components.CombatTestTarget) {
		target.HP = math.Max(0, target.HP-hit.Damage)
		target.Revision++
	})
	if receiver.service.OnTarget != nil {
		receiver.service.OnTarget(receiver.handle)
	}
	return nil
}
