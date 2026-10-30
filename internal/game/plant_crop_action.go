package game

import (
	"fmt"
	"math"
	"math/rand/v2"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/behaviors/contracts"
	gameworld "origin/internal/game/world"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/playerstate"
	"origin/internal/types"

	netproto "origin/internal/network/proto"
)

const plantCropActionID = "plant"

type cropPlantingTerrain interface {
	GetTileID(int, int) (byte, bool)
	GetChunkFast(types.ChunkCoord) *core.Chunk
}

type plantingSeed struct {
	hand       types.Handle
	version    uint64
	item       components.InvItem
	generation uint64
}

type preparedPlanting struct {
	playerID types.EntityID
	player   types.Handle
	seed     plantingSeed
	crop     types.Handle
	chunk    *core.Chunk
	x, y     float64
	missed   bool
}

// One handler belongs to one World. Preparation and commit run in the same
// shard-locked call, so no inventory/world mutation can race the cost commit.
type plantCropActionHandler struct {
	world       *ecs.World
	terrain     cropPlantingTerrain
	ids         contracts.EntityIDAllocator
	registry    contracts.BehaviorRegistry
	bySeed      map[uint32]*objectdefs.ObjectDef
	variance    int64
	actions     *ActionService
	notify      func(*ecs.World, types.EntityID, types.Handle)
	alerts      contracts.MiniAlertSender
	seeds       map[types.Handle]plantingSeed
	candidates  []types.Handle
	random      func() float64
	randomInt64 func(int64) int64
	prepared    *preparedPlanting
}

func newPlantCropActionHandler(w *ecs.World, terrain cropPlantingTerrain, ids contracts.EntityIDAllocator,
	registry contracts.BehaviorRegistry, objects *objectdefs.Registry, items *itemdefs.Registry, variance int64,
	notify func(*ecs.World, types.EntityID, types.Handle), alerts contracts.MiniAlertSender,
) (*plantCropActionHandler, error) {
	if w == nil || terrain == nil || ids == nil || registry == nil || objects == nil || items == nil || variance < 0 || variance > math.MaxUint32 {
		return nil, fmt.Errorf("invalid crop planting dependencies or quality variance")
	}
	h := &plantCropActionHandler{
		world: w, terrain: terrain, ids: ids, registry: registry, variance: variance,
		notify: notify, alerts: alerts, bySeed: make(map[uint32]*objectdefs.ObjectDef),
		seeds: make(map[types.Handle]plantingSeed), candidates: make([]types.Handle, 0, placementMaxCandidates),
		random: rand.Float64, randomInt64: rand.Int64N,
	}
	for _, def := range objects.All() {
		if def.CropConfig == nil {
			continue
		}
		seed, found := items.GetByKey(def.CropConfig.SeedItemKey)
		if !found {
			return nil, fmt.Errorf("crop %q has unknown seed %q", def.Key, def.CropConfig.SeedItemKey)
		}
		if previous := h.bySeed[uint32(seed.DefID)]; previous != nil {
			return nil, fmt.Errorf("crops %q and %q share seed %q", previous.Key, def.Key, seed.Key)
		}
		h.bySeed[uint32(seed.DefID)] = def
	}
	w.AddDespawnObserver(func(handle types.Handle) { delete(h.seeds, handle) })
	return h, nil
}

func (*plantCropActionHandler) RequiresItemMutation() bool { return true }

func (h *plantCropActionHandler) CanPlantFromHand(w *ecs.World, playerID types.EntityID, player types.Handle) bool {
	_, _, ok := h.handSeed(w, playerID, player)
	return ok
}

func (h *plantCropActionHandler) StartPlanting(w *ecs.World, playerID types.EntityID, player types.Handle, x, y float64) {
	if h.actions == nil {
		return
	}
	h.actions.Cancel(w, playerID, player)
	seed, _, ok := h.handSeed(w, playerID, player)
	if !ok {
		return
	}
	// Capture before direct dispatch: a player already at the tile center enters
	// validation synchronously. Bind the generation after dispatch returns.
	h.seeds[player] = seed
	h.actions.StartTargetedOnce(w, playerID, player, plantCropActionID, 0, types.InvalidHandle, x, y)
	active, found := ecs.GetComponent[components.ActiveGameAction](w, player)
	if !found || active.ActionID != plantCropActionID {
		delete(h.seeds, player)
		return
	}
	seed.generation = active.Generation
	h.seeds[player] = seed
}

func (h *plantCropActionHandler) UnavailableReason(w *ecs.World, playerID types.EntityID, player types.Handle) string {
	if w != h.world {
		return "ACTION_UNAVAILABLE"
	}
	if playerstate.ItemsLocked(w, player) {
		return playerstate.ItemsLockedReason
	}
	if _, _, ok := h.handSeed(w, playerID, player); !ok {
		return "SEED_REQUIRED"
	}
	return ""
}

func (h *plantCropActionHandler) ValidateTarget(w *ecs.World, playerID types.EntityID, player types.Handle, target ActionTarget) string {
	if reason := h.UnavailableReason(w, playerID, player); reason != "" {
		return reason
	}
	current, _, _ := h.handSeed(w, playerID, player)
	active, hasAction := ecs.GetComponent[components.ActiveGameAction](w, player)
	if hasAction && active.ActionID == plantCropActionID {
		if active.Phase == components.GameActionSelecting {
			current.generation = active.Generation
			h.seeds[player] = current
		} else {
			seed, found := h.seeds[player]
			if !found || seed.hand != current.hand || seed.version != current.version || seed.item != current.item || seed.generation != 0 && seed.generation != active.Generation {
				return "PLANT_SEED_CHANGED"
			}
		}
	}
	x, y, valid := tileCenter(target.X, target.Y)
	if !valid {
		return "ACTION_INVALID_TARGET"
	}
	tileX, tileY := tileCoordinates(ActionTarget{X: x, Y: y})
	tile, loaded := h.terrain.GetTileID(tileX, tileY)
	if !loaded {
		return "ACTION_INVALID_TARGET"
	}
	if tile != types.TilePlowed {
		return "BAD_TERRAIN"
	}
	chunk := h.terrain.GetChunkFast(types.WorldToChunkCoord(int(x), int(y), constt.ChunkSize, constt.CoordPerTile))
	if chunk == nil || chunk.GetState() != types.ChunkStateActive || chunk.Layer != w.Layer {
		return "ACTION_INVALID_TARGET"
	}
	if !h.siteAvailable(w, player, x, y) {
		return "PLANT_TILE_OCCUPIED"
	}
	return ""
}

func (*plantCropActionHandler) Start(*ecs.World, types.EntityID, types.Handle, ActionTarget, uint64) ActionResult {
	// ActionService uses preparedActionCompletion for this handler.
	return ActionResult{Outcome: ActionFailed, Reason: "ACTION_FAILED"}
}

func (h *plantCropActionHandler) PrepareCompletion(w *ecs.World, playerID types.EntityID, player types.Handle, target ActionTarget, generation uint64) string {
	if h.prepared != nil {
		return "ACTION_UNAVAILABLE"
	}
	if reason := h.ValidateTarget(w, playerID, player, target); reason != "" {
		return reason
	}
	seed, def, ok := h.handSeed(w, playerID, player)
	selected, selectedOK := h.seeds[player]
	if !ok || !selectedOK || selected.generation != generation || selected.hand != seed.hand || selected.version != seed.version || selected.item != seed.item || seed.version == math.MaxUint64 {
		return "PLANT_SEED_CHANGED"
	}
	x, y, _ := tileCenter(target.X, target.Y)
	chunk := h.terrain.GetChunkFast(types.WorldToChunkCoord(int(x), int(y), constt.ChunkSize, constt.CoordPerTile))
	plan := &preparedPlanting{playerID: playerID, player: player, seed: seed, chunk: chunk, x: x, y: y}
	chance := *def.CropConfig.PlantingSuccessChance
	plan.missed = chance <= 0 || chance < 1 && h.random() >= chance
	if !plan.missed {
		quality := rollCropQuality(seed.item.Quality, h.variance, h.randomInt64)
		id := h.ids.GetFreeID()
		if id == 0 || id > math.MaxInt64 {
			return "PLANT_FAILED"
		}
		plan.crop = gameworld.SpawnEntityFromDef(w, def, gameworld.DefSpawnParams{
			EntityID: id, X: x, Y: y, Quality: quality, Region: chunk.Region, Layer: chunk.Layer,
			InitReason: contracts.ObjectBehaviorInitReasonSpawn, BehaviorRegistry: h.registry,
		})
		if plan.crop == types.InvalidHandle {
			return "PLANT_FAILED"
		}
	}
	h.prepared = plan
	return ""
}

func (h *plantCropActionHandler) CommitPrepared() {
	plan := h.prepared
	ecs.WithComponent(h.world, plan.seed.hand, func(hand *components.InventoryContainer) {
		clear(hand.Items)
		hand.Items = hand.Items[:0]
		hand.HandMouseOffsetX, hand.HandMouseOffsetY = 0, 0
		hand.Version++
	})
	if plan.crop != types.InvalidHandle {
		coord := types.WorldToChunkCoord(int(plan.x), int(plan.y), constt.ChunkSize, constt.CoordPerTile)
		ecs.AddComponent(h.world, plan.crop, components.ChunkRef{
			CurrentChunkX: coord.X, CurrentChunkY: coord.Y, PrevChunkX: coord.X, PrevChunkY: coord.Y,
		})
		handles := [1]types.Handle{plan.crop}
		systems.RecomputeObjectBehaviorsNow(h.world, nil, nil, h.registry, handles[:])
		plan.chunk.Spatial().AddStatic(plan.crop, int(plan.x), int(plan.y))
		plan.chunk.MarkRawDataDirty()
	}
}

func (h *plantCropActionHandler) AbortPrepared() {
	if plan := h.prepared; plan != nil && plan.crop != types.InvalidHandle {
		h.world.Despawn(plan.crop)
	}
	h.prepared = nil
}

func (h *plantCropActionHandler) AfterCompletion() {
	plan := h.prepared
	h.prepared = nil
	delete(h.seeds, plan.player)
	if h.notify != nil {
		h.notify(h.world, plan.playerID, plan.player)
	}
	if plan.missed && h.alerts != nil {
		h.alerts.SendMiniAlert(plan.playerID, &netproto.S2C_MiniAlert{
			Severity: netproto.AlertSeverity_ALERT_SEVERITY_INFO, ReasonCode: "PLANTING_MISSED", TtlMs: 3000,
		})
	}
}

func (h *plantCropActionHandler) Cancel(_ *ecs.World, _ types.EntityID, player types.Handle, _ components.ActiveGameAction) {
	delete(h.seeds, player)
}

func (h *plantCropActionHandler) handSeed(w *ecs.World, playerID types.EntityID, player types.Handle) (plantingSeed, *objectdefs.ObjectDef, bool) {
	if w != h.world || playerID == 0 || !w.Alive(player) || w.GetHandleByEntityID(playerID) != player {
		return plantingSeed{}, nil, false
	}
	owner, found := ecs.GetComponent[components.InventoryOwner](w, player)
	if !found {
		return plantingSeed{}, nil, false
	}
	for _, link := range owner.Inventories {
		if link.OwnerID != playerID || link.Kind != constt.InventoryHand || link.Key != 0 || !w.Alive(link.Handle) {
			continue
		}
		hand, found := ecs.GetComponent[components.InventoryContainer](w, link.Handle)
		if !found || hand.OwnerID != playerID || hand.Kind != constt.InventoryHand || hand.Key != 0 || len(hand.Items) != 1 || hand.Items[0].Quantity != 1 || hand.Items[0].ItemID == 0 {
			return plantingSeed{}, nil, false
		}
		def := h.bySeed[hand.Items[0].TypeID]
		if def == nil {
			return plantingSeed{}, nil, false
		}
		return plantingSeed{hand: link.Handle, version: hand.Version, item: hand.Items[0]}, def, true
	}
	return plantingSeed{}, nil, false
}

func (h *plantCropActionHandler) siteAvailable(w *ecs.World, player types.Handle, x, y float64) bool {
	spatial, found := ecs.TryGetResource[*core.WorldColliderSpatial](w)
	if !found || *spatial == nil || (*spatial).Index == nil {
		return false
	}
	bounds := core.ColliderBounds{MinX: x - 2, MinY: y - 2, MaxX: x + 2, MaxY: y + 2}
	candidates, err := (*spatial).Index.QueryBoundedInto(bounds, h.candidates[:0], core.ColliderQueryBudget{MaxCells: placementMaxCells, MaxVisits: placementMaxVisits})
	if err != nil {
		return false
	}
	for _, candidate := range candidates {
		if candidate == player || !w.Alive(candidate) {
			continue
		}
		info, hasInfo := ecs.GetComponent[components.EntityInfo](w, candidate)
		collider, hasCollider := ecs.GetComponent[components.Collider](w, candidate)
		position, hasPosition := ecs.GetComponent[components.Transform](w, candidate)
		if !hasInfo || !info.IsStatic || !hasCollider || collider.Phantom != nil || !hasPosition {
			continue
		}
		if bounds.MinX < position.X+collider.HalfWidth && bounds.MaxX > position.X-collider.HalfWidth && bounds.MinY < position.Y+collider.HalfHeight && bounds.MaxY > position.Y-collider.HalfHeight {
			return false
		}
	}
	return true
}

func rollCropQuality(seed uint32, variance int64, draw func(int64) int64) uint32 {
	if variance == 0 {
		return seed
	}
	quality := int64(seed) + draw(variance*2+1) - variance
	return uint32(max(0, min(int64(math.MaxUint32), quality)))
}
