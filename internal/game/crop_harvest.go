package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"origin/internal/characterattrs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entitystats"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	"origin/internal/itemdefs"
	"origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/playerstate"
	"origin/internal/types"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sqlc-dev/pqtype"
	"go.uber.org/zap"
)

const cropHarvestCapacity = 128

type cropHarvestPersister interface {
	PersistCropHarvest(context.Context, inventory.CropHarvestPersistenceRecord) error
}

type cropHarvestDependencies struct {
	chunks        ObjectDestructionChunks
	persister     cropHarvestPersister
	ids           ObjectDestructionIDs
	executor      *inventory.InventoryExecutor
	withWorldRead func(func(*ecs.World))
	quarantine    func(types.Handle)
	granted       func(types.EntityID, types.Handle, *inventory.GiveItemResult)
	rejected      func(types.EntityID)
	roll          func(int) int
	logger        *zap.Logger
}

type cropHarvestService struct {
	world      *ecs.World
	deps       cropHarvestDependencies
	operations [cropHarvestCapacity]cropHarvestOperation
	count      int
	closed     bool
}

type cropHarvestPhase uint8

const (
	cropHarvestIdle cropHarvestPhase = iota
	cropHarvestRunning
	cropHarvestReady
	cropHarvestRetry
)

type cropHarvestOperation struct {
	mu          sync.Mutex
	service     *cropHarvestService
	phase       cropHarvestPhase
	err         error
	due         time.Time
	backoff     time.Duration
	target      types.Handle
	targetID    types.EntityID
	recipient   types.Handle
	recipientID types.EntityID
	source      *repository.Object
	coord       types.ChunkCoord
	grant       *inventory.PreparedCropGrant
	items       []components.InvItem
	lastID      types.EntityID
	dropTime    int64
	cost        float64
	record      *inventory.CropHarvestPersistenceRecord
	dropPage    []destructionDroppedRecord
	dropRead    int
	applied     bool
	removed     bool
}

func newCropHarvestService(w *ecs.World, deps cropHarvestDependencies) (*cropHarvestService, error) {
	if w == nil || deps.chunks == nil || deps.persister == nil || deps.ids == nil || deps.executor == nil || deps.withWorldRead == nil || deps.quarantine == nil {
		return nil, ErrInvalidObjectDestructionService
	}
	if deps.roll == nil {
		deps.roll = rand.IntN
	}
	if deps.logger == nil {
		deps.logger = zap.NewNop()
	}
	if state, found := ecs.TryGetResource[ecs.ObjectDestructionState](w); !found {
		ecs.SetResource(w, ecs.ObjectDestructionState{Prepared: make(map[types.Handle]bool), Pending: make(map[types.Handle]bool)})
	} else if state.Pending == nil {
		state.Pending = make(map[types.Handle]bool)
	}
	s := &cropHarvestService{world: w, deps: deps}
	for i := range s.operations {
		s.operations[i].service = s
	}
	return s, nil
}

func (s *Shard) initCropHarvest() error {
	if s.objectDestruction == nil {
		return ErrInvalidObjectDestructionService
	}
	persister, ok := s.objectDestruction.deps.Persister.(cropHarvestPersister)
	if !ok {
		return ErrInvalidObjectDestructionService
	}
	var err error
	s.cropHarvest, err = newCropHarvestService(s.world, cropHarvestDependencies{chunks: s.chunkManager, persister: persister, ids: s.entityIDManager,
		executor: s.inventoryExecutor, withWorldRead: s.WithWorldRead, quarantine: s.quarantineDestroyedObject, granted: s.skullGranted,
		rejected: func(id types.EntityID) {
			s.SendMiniAlert(id, &proto.S2C_MiniAlert{ReasonCode: "HARVEST_FAILED", Severity: proto.AlertSeverity_ALERT_SEVERITY_WARNING})
		}, logger: s.logger})
	return err
}

func (s *Shard) harvestCrop(w *ecs.World, playerID types.EntityID, player types.Handle, targetID types.EntityID, target types.Handle) contracts.BehaviorResult {
	if s == nil || w != s.world || s.cropHarvest == nil {
		return cropHarvestFailure("HARVEST_UNAVAILABLE")
	}
	return s.cropHarvest.harvest(playerID, player, targetID, target)
}

func cropHarvestFailure(reason string) contracts.BehaviorResult {
	return contracts.BehaviorResult{UserVisible: true, ReasonCode: reason, Severity: contracts.BehaviorAlertSeverityWarning}
}

func (s *cropHarvestService) harvest(playerID types.EntityID, player types.Handle, targetID types.EntityID, target types.Handle) contracts.BehaviorResult {
	w := s.world
	if s.closed || s.count == len(s.operations) || !w.Alive(player) || !w.Alive(target) || w.GetHandleByEntityID(playerID) != player || w.GetHandleByEntityID(targetID) != target ||
		playerstate.ItemsLocked(w, player) || ecs.ObjectDestructionPending(w, target) || ecs.InventoryOwnerReserved(w, playerID) {
		return cropHarvestFailure("HARVEST_UNAVAILABLE")
	}
	active, exists := ecs.GetComponent[components.ActiveCyclicAction](w, player)
	if !exists || active.BehaviorKey != "crop" || active.ActionID != "harvest" || active.TargetKind != components.CyclicActionTargetObject || active.TargetID != targetID || active.TargetHandle != target ||
		!active.MutatesItems || active.CycleDurationTicks == 0 || active.CycleElapsedTicks < active.CycleDurationTicks {
		return cropHarvestFailure("HARVEST_UNAVAILABLE")
	}
	links, exists := ecs.TryGetResource[ecs.LinkState](w)
	if !exists {
		return cropHarvestFailure("HARVEST_UNAVAILABLE")
	}
	link, linked := links.GetLink(playerID)
	if !linked || link.PlayerHandle != player || link.TargetID != targetID || link.TargetHandle != target {
		return cropHarvestFailure("HARVEST_UNAVAILABLE")
	}
	definition, valid := behaviors.CropDefinition(w, target, targetID)
	if !valid || definition.CropConfig == nil || active.CycleDurationTicks != definition.CropConfig.HarvestCycleDurationTicks {
		return cropHarvestFailure("HARVEST_UNAVAILABLE")
	}
	now := ecs.GetResource[ecs.TimeState](w)
	if _, err := behaviors.AdvanceCropGrowth(w, target, targetID, now.RuntimeSecondsTotal); err != nil {
		return cropHarvestFailure("HARVEST_UNAVAILABLE")
	}
	state := behaviors.CropState(w, target)
	if state == nil || state.Stage < 0 || state.Stage >= len(definition.CropConfig.Stages) {
		return cropHarvestFailure("HARVEST_UNAVAILABLE")
	}
	products := definition.CropConfig.Stages[state.Stage].Harvest
	if len(products) == 0 || len(products) > inventory.MaxCropHarvestUnits {
		return cropHarvestFailure("HARVEST_NOT_READY")
	}
	cost := definition.CropConfig.HarvestStaminaCost
	stats, exists := ecs.GetComponent[components.EntityStats](w, player)
	if !exists || cost <= 0 || math.IsNaN(cost) || math.IsInf(cost, 0) || math.IsNaN(stats.Stamina) || math.IsInf(stats.Stamina, 0) || entitystats.ClampStamina(stats.Stamina, cropHarvestMaxStamina(w, player)) < cost {
		return cropHarvestFailure("LOW_STAMINA")
	}
	grant, err := s.deps.executor.PrepareCropGrant(w, playerID, player)
	if err != nil {
		return cropHarvestFailure("HARVEST_UNAVAILABLE")
	}
	source, err := gameworld.NewObjectFactory(nil).Serialize(w, target)
	if err != nil || source == nil {
		return cropHarvestFailure("HARVEST_UNAVAILABLE")
	}
	items := make([]components.InvItem, 0, inventory.MaxCropHarvestUnits)
	info, _ := ecs.GetComponent[components.EntityInfo](w, target)
	for _, product := range products {
		itemDefinition, found := itemdefs.Global().GetByKey(product.ItemKey)
		if !found || itemDefinition.Container != nil || product.Min < 1 || product.Max < product.Min || product.Max > inventory.MaxCropHarvestUnits {
			return cropHarvestFailure("HARVEST_UNAVAILABLE")
		}
		count := product.Min + s.deps.roll(product.Max-product.Min+1)
		if count < product.Min || count > product.Max || len(items)+count > inventory.MaxCropHarvestUnits {
			return cropHarvestFailure("HARVEST_UNAVAILABLE")
		}
		for i := 0; i < count; i++ {
			items = append(items, components.InvItem{TypeID: uint32(itemDefinition.DefID), Resource: itemDefinition.ResolveResource(false), Quality: info.Quality, Quantity: 1, W: uint8(itemDefinition.Size.W), H: uint8(itemDefinition.Size.H)})
		}
	}
	coord := types.ChunkCoord{X: source.ChunkX, Y: source.ChunkY}
	if err := s.deps.chunks.PinPersistence(coord); err != nil {
		return cropHarvestFailure("HARVEST_UNAVAILABLE")
	}
	if !ecs.ReserveInventoryOwner(w, playerID, player) {
		s.deps.chunks.UnpinPersistence(coord)
		return cropHarvestFailure("HARVEST_UNAVAILABLE")
	}
	first, last, err := s.deps.ids.ReserveIDs(uint64(len(items)))
	if err != nil || first == 0 || last > math.MaxInt64 || last-first+1 != types.EntityID(len(items)) {
		ecs.ReleaseInventoryOwner(w, playerID, player)
		s.deps.chunks.UnpinPersistence(coord)
		return cropHarvestFailure("HARVEST_UNAVAILABLE")
	}
	for i := range items {
		items[i].ItemID = first + types.EntityID(i)
	}
	var op *cropHarvestOperation
	for i := range s.operations {
		s.operations[i].mu.Lock()
		if s.operations[i].phase == cropHarvestIdle {
			op = &s.operations[i]
			s.operations[i].mu.Unlock()
			break
		}
		s.operations[i].mu.Unlock()
	}
	op.target, op.targetID, op.recipient, op.recipientID = target, targetID, player, playerID
	op.source, op.coord, op.grant, op.items, op.lastID, op.dropTime, op.cost = source, coord, grant, items, last, now.RuntimeSecondsTotal, cost
	op.phase = cropHarvestRunning
	ecs.GetResource[ecs.ObjectDestructionState](w).Pending[target] = true
	// Admission is nonblocking. A worker's first World read waits for this
	// shard-locked completion to publish its reservation and exact cost.
	if !s.deps.chunks.SubmitPersistenceJob(op) {
		delete(ecs.GetResource[ecs.ObjectDestructionState](w).Pending, target)
		s.release(op)
		return cropHarvestFailure("HARVEST_UNAVAILABLE")
	}
	s.count++
	ecs.CancelBehaviorTicksByEntityID(w, targetID)
	// The same clamped balance was checked above under this shard lock; no
	// actor/profile/stat mutation occurs between the check and this exact-cost
	// commit. The shared helper also schedules regen and propagates move mode.
	behaviors.ConsumePlayerActionStamina(w, player, cost)
	return contracts.BehaviorResult{OK: true}
}

func (op *cropHarvestOperation) Run() error {
	s := op.service
	if op.record == nil {
		for {
			var done bool
			var err error
			s.deps.withWorldRead(func(w *ecs.World) { done, err = op.grant.CaptureBatch(w, ObjectDestructionCaptureBudget) })
			if err != nil {
				return err
			}
			if done {
				break
			}
		}
		snapshots, overflow, err := op.grant.Build(op.items)
		if err != nil {
			return err
		}
		receipt := inventory.CropHarvestReceipt{FirstItemID: op.items[0].ItemID, LastItemID: op.lastID, RecipientID: op.recipientID}
		var envelope components.ObjectStateEnvelope
		if op.source.Data.Valid && len(op.source.Data.RawMessage) > 0 {
			if json.Unmarshal(op.source.Data.RawMessage, &envelope) != nil {
				return inventory.ErrInvalidCropGrant
			}
		}
		envelope.Version = 1
		if envelope.Behaviors == nil {
			envelope.Behaviors = make(map[string]json.RawMessage)
		}
		encoded, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		envelope.Behaviors["crop_harvest_receipt"] = encoded
		data, err := json.Marshal(envelope)
		if err != nil {
			return err
		}
		record := &inventory.CropHarvestPersistenceRecord{Source: op.source, Receipt: receipt, DeletedData: pqtype.NullRawMessage{RawMessage: data, Valid: true}, PlayerInventories: snapshots}
		for _, item := range overflow {
			drop, err := inventory.BuildDroppedItemPersistenceRecord(inventory.SpawnDroppedEntityParams{DroppedEntityID: item.ItemID, ItemID: item.ItemID, TypeID: item.TypeID, Resource: item.Resource, Quality: item.Quality,
				Quantity: 1, W: item.W, H: item.H, Region: op.source.Region, Layer: op.source.Layer, DropX: op.source.X, DropY: op.source.Y, ChunkX: op.coord.X, ChunkY: op.coord.Y, DropperID: op.recipientID, NowRuntimeSeconds: op.dropTime}, nil)
			if err != nil {
				return err
			}
			record.Drops = append(record.Drops, drop)
			op.dropPage = append(op.dropPage, destructionDroppedRecord{object: &repository.Object{ID: int64(drop.EntityID), TypeID: drop.TypeID, Region: drop.Region, Layer: drop.Layer, X: drop.X, Y: drop.Y,
				ChunkX: drop.ChunkX, ChunkY: drop.ChunkY, Data: pqtype.NullRawMessage{RawMessage: drop.ObjectData, Valid: true}}, inventory: &repository.Inventory{OwnerID: int64(drop.EntityID), Kind: int16(constt.InventoryDroppedItem), Data: drop.InventoryData, Version: 1}})
		}
		op.record = record
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.deps.chunks.WithPersistence([]types.ChunkCoord{op.coord}, func() error { return s.deps.persister.PersistCropHarvest(ctx, *op.record) })
}

func (op *cropHarvestOperation) Complete(err error) {
	op.mu.Lock()
	op.err = err
	op.phase = cropHarvestReady
	op.mu.Unlock()
}

func definiteCropHarvestFailure(err error) bool {
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) && len(databaseError.Code) >= 2 && (databaseError.Code[:2] == "22" || databaseError.Code[:2] == "23") {
		return true
	}
	return errors.Is(err, inventory.ErrInvalidCropGrant) || errors.Is(err, inventory.ErrInvalidObjectLootCapture) || errors.Is(err, inventory.ErrCropHarvestSourceConflict) || errors.Is(err, inventory.ErrCropHarvestInventoryConflict)
}

func (s *cropHarvestService) Update() {
	if s == nil || s.count == 0 {
		return
	}
	now := ecs.GetResource[ecs.TimeState](s.world).Now
	budget := ObjectDestructionDropBudget
	for i := range s.operations {
		op := &s.operations[i]
		op.mu.Lock()
		if op.phase == cropHarvestReady {
			if op.err != nil {
				if definiteCropHarvestFailure(op.err) {
					s.reject(op)
					s.release(op)
					s.count--
					op.mu.Unlock()
					continue
				}
				if op.backoff == 0 {
					op.backoff = time.Second
				} else {
					op.backoff *= 2
					if op.backoff > time.Minute {
						op.backoff = time.Minute
					}
				}
				s.deps.logger.Warn("Crop harvest persistence unresolved", zap.Uint64("crop_id", uint64(op.targetID)), zap.Error(op.err))
				op.due = now.Add(op.backoff)
				op.phase = cropHarvestRetry
			} else {
				if !op.applied {
					result, err := op.grant.Apply(s.world)
					if err != nil {
						s.deps.logger.Error("Committed crop harvest awaits inventory application", zap.Uint64("crop_id", uint64(op.targetID)), zap.Error(err))
						op.mu.Unlock()
						continue
					}
					op.applied = true
					if s.deps.granted != nil {
						s.deps.granted(op.recipientID, op.recipient, result)
					}
				}
				if !op.removed {
					// Quarantine only after durability; the pending crop occupied
					// its tile throughout the transaction and uncertain retries.
					if s.world.Alive(op.target) && s.world.GetHandleByEntityID(op.targetID) == op.target {
						s.deps.quarantine(op.target)
					}
					s.deps.chunks.RemoveCommittedSource(op.coord, op.targetID)
					if s.world.Alive(op.target) && s.world.GetHandleByEntityID(op.targetID) == op.target {
						s.world.Despawn(op.target)
					}
					op.removed = true
				}
				for op.dropRead < len(op.dropPage) && budget > 0 {
					drop := op.dropPage[op.dropRead]
					if s.deps.chunks.InsertCommittedDropped(drop.object, drop.inventory) != nil {
						break
					}
					op.dropRead++
					budget--
				}
				if op.dropRead == len(op.dropPage) {
					s.release(op)
					s.count--
					op.mu.Unlock()
					continue
				}
			}
		}
		if op.phase == cropHarvestRetry && !now.Before(op.due) {
			op.phase = cropHarvestRunning
			op.mu.Unlock()
			if !s.deps.chunks.SubmitPersistenceJob(op) {
				op.mu.Lock()
				op.phase = cropHarvestRetry
				op.mu.Unlock()
			}
			continue
		}
		op.mu.Unlock()
	}
}

func (s *cropHarvestService) reject(op *cropHarvestOperation) {
	w := s.world
	delete(ecs.GetResource[ecs.ObjectDestructionState](w).Pending, op.target)
	if w.Alive(op.target) && w.GetHandleByEntityID(op.targetID) == op.target {
		if err := behaviors.InitializeCropState(w, op.target, op.targetID); err != nil {
			s.deps.logger.Error("Failed to resume crop growth", zap.Error(err))
		}
	}
	if w.Alive(op.recipient) && w.GetHandleByEntityID(op.recipientID) == op.recipient {
		maxStamina := cropHarvestMaxStamina(w, op.recipient)
		ecs.MutateComponent[components.EntityStats](w, op.recipient, func(stats *components.EntityStats) bool {
			stats.Stamina = entitystats.ClampStamina(stats.Stamina+op.cost, maxStamina)
			return true
		})
		behaviors.ConsumePlayerActionStamina(w, op.recipient, 0)
		ecs.MarkPlayerStatsDirtyByHandle(w, op.recipient, ecs.ResolvePlayerStatsTTLms(w))
	}
	if s.deps.rejected != nil {
		s.deps.rejected(op.recipientID)
	}
}

func cropHarvestMaxStamina(w *ecs.World, player types.Handle) float64 {
	if profile, exists := ecs.GetComponent[components.CharacterProfile](w, player); exists {
		return entitystats.MaxStaminaFromAttributes(profile.Attributes)
	}
	return entitystats.MaxStaminaFromCon(characterattrs.DefaultValue)
}

func (s *cropHarvestService) release(op *cropHarvestOperation) {
	ecs.ReleaseInventoryOwner(s.world, op.recipientID, op.recipient)
	s.deps.chunks.UnpinPersistence(op.coord)
	op.phase = cropHarvestIdle
	op.err = nil
	op.due = time.Time{}
	op.backoff = 0
	op.source = nil
	op.grant = nil
	op.items = nil
	op.record = nil
	op.dropPage = nil
	op.dropRead = 0
	op.applied = false
	op.removed = false
}

func (s *cropHarvestService) StopAdmission() {
	if s != nil {
		s.closed = true
	}
}
func (s *Shard) pendingCropHarvest() bool { return s.cropHarvest != nil && s.cropHarvest.count != 0 }

func (s *Shard) drainCropHarvest() {
	if s.cropHarvest == nil {
		return
	}
	s.mu.Lock()
	s.cropHarvest.StopAdmission()
	base := ecs.GetResource[ecs.TimeState](s.world).Now
	s.mu.Unlock()
	started := time.Now()
	for {
		s.mu.Lock()
		state := ecs.GetResource[ecs.TimeState](s.world)
		previous := state.Now
		state.Now = base.Add(time.Since(started))
		s.cropHarvest.Update()
		state.Now = previous
		pending := s.cropHarvest.count
		s.mu.Unlock()
		if pending == 0 {
			return
		}
		if time.Since(started) > objectDestructionShutdownTimeout {
			s.logger.Error("Shutdown crop harvest persistence incomplete", zap.Int("pending", pending))
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
