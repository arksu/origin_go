package game

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"sync"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
	"go.uber.org/zap"
)

const (
	ObjectDestructionQueueCapacity   = 512
	ObjectDestructionCaptureBudget   = 100
	ObjectDestructionDropBudget      = 32
	ObjectDestructionCleanupBudget   = 100
	objectDestructionSQLBudget       = 100
	objectDestructionShutdownTimeout = 30 * time.Second
)

var (
	ErrInvalidObjectDestructionService = errors.New("object destruction: invalid dependencies")
	ErrObjectDestructionQueueFull      = errors.New("object destruction: queue is full")
	ErrObjectDestructionStopped        = errors.New("object destruction: admission stopped")
	ErrObjectDestructionPending        = errors.New("object destruction: already pending")
	ErrObjectDestructionCapture        = errors.New("object destruction: source changed during capture")
	ErrObjectDestructionOverflow       = errors.New("object destruction: item count or identity overflow")
	ErrObjectDestructionShutdown       = errors.New("object destruction: shutdown has pending operations")
)

// ObjectDestructionChunks shares the existing chunk I/O workers. All methods
// except pins, WithPersistence and SubmitPersistenceJob run under shard lock.
type ObjectDestructionChunks interface {
	SubmitPersistenceJob(gameworld.PersistenceJob) bool
	PinPersistence(types.ChunkCoord) error
	UnpinPersistence(types.ChunkCoord)
	WithPersistence([]types.ChunkCoord, func() error) error
	InsertCommittedDropped(*repository.Object, *repository.Inventory) error
	RemoveCommittedSource(types.ChunkCoord, types.EntityID)
}

type ObjectDestructionPersister interface {
	ReplaceObjectWithDroppedItems(context.Context, int, types.EntityID, types.EntityID,
		func([]inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error)) error
}

// Transformation keeps the source identity instead of deleting its object row.
type objectLootTransformationPersister interface {
	TransformObjectWithDroppedItems(context.Context, *repository.Object, types.EntityID,
		func([]inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error)) error
}

type objectLootTransformationChunks interface {
	ReplaceCommittedSource(*repository.Object) error
}

type ObjectDestructionIDs interface {
	ReserveIDs(uint64) (types.EntityID, types.EntityID, error)
}

// ObjectDestructionDependencies makes the owning shard execution path explicit.
// Quarantine is synchronous; background workers may only use WithWorldRead.
type ObjectDestructionDependencies struct {
	Chunks                 ObjectDestructionChunks
	Persister              ObjectDestructionPersister
	IDs                    ObjectDestructionIDs
	Items                  *itemdefs.Registry
	WithWorldRead          func(func(*ecs.World))
	Quarantine             func(types.Handle)
	TransformCommitted     func(types.Handle, *objectdefs.ObjectDef) bool
	Region                 int
	MinX, MinY, MaxX, MaxY int // Inclusive minimum, exclusive maximum.
	Logger                 *zap.Logger
}

// ObjectDestructionService retains an immutable replacement until durable
// completion. The bounded operation slots also retain completions: a full
// network/server inbox cannot lose a committed replacement.
type ObjectDestructionService struct {
	world      *ecs.World
	deps       ObjectDestructionDependencies
	operations [ObjectDestructionQueueCapacity]objectDestructionOperation
	freeSlots  [ObjectDestructionQueueCapacity]uint16
	freeCount  int
	reserved   map[types.Handle]uint16 // slot + 1; entries are created by preparation.
	count      int
	closed     bool
	sequence   uint64
}

type destructionPhase uint8

const (
	destructionIdle destructionPhase = iota
	destructionReserved
	destructionCommitted
	destructionQueued
	destructionRunning
	destructionReady
)

// Tokens are private and consumed within one shard-locked action. A ticket
// prevents an aborted or completed token from affecting a reused slot.
type objectDestructionReservation struct {
	service *ObjectDestructionService
	slot    uint16
	ticket  uint64
}

type objectDestructionOperation struct {
	mu                  sync.Mutex
	service             *ObjectDestructionService
	slot                uint16
	ticket              uint64
	phase               destructionPhase
	err                 error
	due                 time.Time
	backoff             time.Duration
	target              types.Handle
	id                  types.EntityID
	region, layer, x, y int
	source              types.ChunkCoord
	pinned              []types.ChunkCoord
	seed                uint64
	dropTime            int64
	capture             *inventory.ObjectLootCapture
	captured            bool
	total               uint64
	extraFirst, lastID  types.EntityID
	allocated           bool
	committed           bool
	finalized           bool
	cleanupRead         int
	cursor              destructionLootCursor
	page                []destructionDroppedRecord
	pageRead            int
	replacement         *repository.Object
	replacementDef      *objectdefs.ObjectDef
}

type destructionDroppedRecord struct {
	object    *repository.Object
	inventory *repository.Inventory
}

type destructionLootCursor struct {
	item           int
	unit           uint32
	extra, emitted uint64
}

func NewObjectDestructionService(w *ecs.World, deps ObjectDestructionDependencies) (*ObjectDestructionService, error) {
	if w == nil || deps.Chunks == nil || deps.Persister == nil || deps.IDs == nil || deps.Items == nil ||
		deps.WithWorldRead == nil || deps.Quarantine == nil || deps.Region < 0 || deps.MinX >= deps.MaxX || deps.MinY >= deps.MaxY {
		return nil, ErrInvalidObjectDestructionService
	}
	if _, exists := ecs.TryGetResource[ecs.ObjectDestructionState](w); !exists {
		ecs.SetResource(w, ecs.ObjectDestructionState{Prepared: make(map[types.Handle]bool), Pending: make(map[types.Handle]bool)})
	}
	attachObjectTargetReferences(w)
	s := &ObjectDestructionService{world: w, deps: deps, freeCount: ObjectDestructionQueueCapacity, reserved: make(map[types.Handle]uint16)}
	for i := range s.operations {
		s.operations[i].service = s
		s.operations[i].slot = uint16(i)
		s.freeSlots[i] = uint16(len(s.operations) - 1 - i)
		s.operations[i].pinned = make([]types.ChunkCoord, 0, 5)
	}
	w.AddDespawnObserver(func(h types.Handle) {
		state := ecs.GetResource[ecs.ObjectDestructionState](w)
		delete(state.Prepared, h)
		delete(state.Pending, h)
		delete(s.reserved, h)
	})
	return s, nil
}

func (s *ObjectDestructionService) PrepareTarget(target types.Handle) error {
	if s == nil || s.world == nil || !s.world.Alive(target) {
		return ErrInvalidObjectDestructionService
	}
	if s.closed {
		return ErrObjectDestructionStopped
	}
	state := ecs.GetResource[ecs.ObjectDestructionState](s.world)
	if state.Pending[target] {
		return ErrObjectDestructionPending
	}
	state.Prepared[target] = true
	state.Pending[target] = false
	if _, exists := s.reserved[target]; !exists {
		s.reserved[target] = 0
	}
	return nil
}

// TransformWithLoot admits a durable lifecycle transition without applying damage.
// The owning shard lock covers preparation, quarantine and every completion.
func (s *ObjectDestructionService) TransformWithLoot(target types.Handle, destination *objectdefs.ObjectDef) error {
	if s == nil || destination == nil || destination.Key == "player" || destination.HP <= 0 ||
		(destination.Components != nil && len(destination.Components.Inventory) != 0) || s.deps.TransformCommitted == nil {
		return ErrInvalidObjectDestructionService
	}
	if _, ok := s.deps.Persister.(objectLootTransformationPersister); !ok {
		return ErrInvalidObjectDestructionService
	}
	if _, ok := s.deps.Chunks.(objectLootTransformationChunks); !ok {
		return ErrInvalidObjectDestructionService
	}
	if !s.world.Alive(target) || ecs.ObjectDestructionPending(s.world, target) {
		return ErrObjectDestructionPending
	}
	replacement, err := gameworld.NewObjectFactory(nil).Serialize(s.world, target)
	if err != nil {
		return err
	}
	if replacement == nil {
		return ErrObjectDestructionCapture
	}
	replacement.TypeID = destination.DefID
	replacement.Hp = sql.NullFloat64{Float64: float64(destination.HP), Valid: true}
	replacement.Data = pqtype.NullRawMessage{}
	if err := s.PrepareTarget(target); err != nil {
		return err
	}
	reservation, err := s.reserve(target)
	if err != nil {
		return err
	}
	op := s.reservationOperation(reservation, destructionReserved)
	op.replacement, op.replacementDef = replacement, destination
	s.commitReservation(reservation)
	s.finalizeReservation(reservation)
	return nil
}

// reserve retains a slot and source pin without changing gameplay state. All
// reservations are committed or canceled before releasing the shard lock.
func (s *ObjectDestructionService) reserve(target types.Handle) (objectDestructionReservation, error) {
	if s.closed {
		return objectDestructionReservation{}, ErrObjectDestructionStopped
	}
	if s.freeCount == 0 {
		return objectDestructionReservation{}, ErrObjectDestructionQueueFull
	}
	state := ecs.GetResource[ecs.ObjectDestructionState](s.world)
	if state.Pending[target] || s.reserved[target] != 0 {
		return objectDestructionReservation{}, ErrObjectDestructionPending
	}
	if !state.Prepared[target] {
		return objectDestructionReservation{}, ErrObjectDamageTargetUnprepared
	}
	if s.sequence == math.MaxUint64 {
		return objectDestructionReservation{}, ErrObjectDestructionOverflow
	}
	id, ok := s.world.GetExternalID(target)
	info, hasInfo := ecs.GetComponent[components.EntityInfo](s.world, target)
	position, hasPosition := ecs.GetComponent[components.Transform](s.world, target)
	chunk, hasChunk := ecs.GetComponent[components.ChunkRef](s.world, target)
	if !ok || !hasInfo || !hasPosition || !hasChunk || info.Region != s.deps.Region ||
		math.IsNaN(position.X) || math.IsInf(position.X, 0) || math.IsNaN(position.Y) || math.IsInf(position.Y, 0) ||
		position.X < float64(s.deps.MinX) || position.X >= float64(s.deps.MaxX) ||
		position.Y < float64(s.deps.MinY) || position.Y >= float64(s.deps.MaxY) {
		return objectDestructionReservation{}, ErrObjectDestructionCapture
	}
	now := ecs.GetResource[ecs.TimeState](s.world)
	if now.RuntimeSecondsTotal < 0 {
		return objectDestructionReservation{}, ErrObjectDestructionCapture
	}
	coord := types.ChunkCoord{X: chunk.CurrentChunkX, Y: chunk.CurrentChunkY}
	if info.Layer != s.world.Layer || coord != types.WorldToChunkCoord(int(position.X), int(position.Y), constt.ChunkSize, constt.CoordPerTile) {
		return objectDestructionReservation{}, ErrObjectDestructionCapture
	}
	if err := s.deps.Chunks.PinPersistence(coord); err != nil {
		return objectDestructionReservation{}, err
	}
	s.freeCount--
	slot := s.freeSlots[s.freeCount]
	op := &s.operations[slot]
	s.sequence++
	op.ticket = s.sequence
	op.target, op.id, op.region, op.layer = target, id, info.Region, info.Layer
	op.x, op.y, op.source = int(position.X), int(position.Y), coord
	op.pinned = append(op.pinned[:0], coord)
	op.seed = uint64(id) ^ s.sequence*0x9e3779b97f4a7c15 ^ uint64(now.UnixMs)
	op.dropTime, op.due, op.backoff = now.RuntimeSecondsTotal, now.Now, 0
	op.phase = destructionReserved
	s.count++
	s.reserved[target] = slot + 1
	return objectDestructionReservation{service: s, slot: slot, ticket: op.ticket}, nil
}

func (s *ObjectDestructionService) reservationOperation(reservation objectDestructionReservation, phase destructionPhase) *objectDestructionOperation {
	if reservation.service != s || reservation.ticket == 0 || int(reservation.slot) >= len(s.operations) {
		return nil
	}
	op := &s.operations[reservation.slot]
	op.mu.Lock()
	defer op.mu.Unlock()
	if op.ticket != reservation.ticket || op.phase != phase {
		return nil
	}
	return op
}

func (s *ObjectDestructionService) cancelReservation(reservation objectDestructionReservation) bool {
	op := s.reservationOperation(reservation, destructionReserved)
	if op == nil {
		return false
	}
	s.release(op)
	return true
}

// commitReservation publishes only pending intent; the receiver writes health.
// Workers cannot observe the operation until every hit is committed and its
// quarantine has completed through finalizeReservation.
func (s *ObjectDestructionService) commitReservation(reservation objectDestructionReservation) bool {
	op := s.reservationOperation(reservation, destructionReserved)
	if op == nil {
		return false
	}
	ecs.GetResource[ecs.ObjectDestructionState](s.world).Pending[op.target] = true
	op.phase = destructionCommitted
	return true
}

func (s *ObjectDestructionService) finalizeReservation(reservation objectDestructionReservation) bool {
	op := s.reservationOperation(reservation, destructionCommitted)
	if op == nil {
		return false
	}
	s.deps.Quarantine(op.target)
	op.phase = destructionQueued
	return true
}

// Update runs under shard lock before chunk activation. Work is admitted to
// existing workers; at most 32 durable ground records are installed per tick.
func (s *ObjectDestructionService) Update() {
	if s == nil || s.count == 0 {
		return
	}
	now := ecs.GetResource[ecs.TimeState](s.world).Now
	budget := ObjectDestructionDropBudget
	cleanupBudget := ObjectDestructionCleanupBudget
	for i := range s.operations {
		op := &s.operations[i]
		op.mu.Lock()
		if op.phase == destructionReady {
			if op.err != nil {
				if op.backoff == 0 {
					op.backoff = time.Second
				} else {
					op.backoff *= 2
					if op.backoff > 60*time.Second {
						op.backoff = 60 * time.Second
					}
				}
				op.due = now.Add(op.backoff)
				op.phase = destructionQueued
			} else {
				if op.committed && !op.finalized {
					if !s.finalizeSource(op, &cleanupBudget) {
						op.mu.Unlock()
						continue
					}
					op.finalized = true
				}
				for op.pageRead < len(op.page) && budget > 0 {
					record := op.page[op.pageRead]
					if s.deps.Chunks.InsertCommittedDropped(record.object, record.inventory) != nil {
						break
					}
					op.pageRead++
					budget--
				}
				if op.pageRead == len(op.page) {
					op.page = nil
					op.pageRead = 0
					if op.cursor.emitted == op.total {
						s.release(op)
					} else {
						op.phase = destructionQueued
						op.due = now
					}
				}
			}
		}
		if op.phase == destructionQueued && !now.Before(op.due) {
			op.phase = destructionRunning
			op.mu.Unlock()
			if !s.deps.Chunks.SubmitPersistenceJob(op) {
				op.mu.Lock()
				op.phase = destructionQueued
				op.mu.Unlock()
			}
			continue
		}
		op.mu.Unlock()
	}
}

func (s *ObjectDestructionService) finalizeSource(op *objectDestructionOperation, budget *int) bool {
	// Every nested value is now owned by durable dropped JSON. Remove only the
	// exact captured references, never recurse into arbitrary current owners.
	index := ecs.GetResource[ecs.InventoryRefIndex](s.world)
	refs := op.capture.ContainerRefs()
	for op.cleanupRead < len(refs) && *budget > 0 {
		ref := refs[op.cleanupRead]
		if current, ok := index.Lookup(ref.Kind, ref.OwnerID, ref.Key); ok && current == ref.Handle {
			index.Remove(ref.Kind, ref.OwnerID, ref.Key)
		}
		if s.world.Alive(ref.Handle) {
			s.world.Despawn(ref.Handle)
		}
		op.cleanupRead++
		*budget--
	}
	if op.cleanupRead != len(refs) || *budget == 0 {
		return false
	}
	*budget--
	if op.replacement != nil {
		if err := s.deps.Chunks.(objectLootTransformationChunks).ReplaceCommittedSource(op.replacement); err != nil {
			return false
		}
		// The ordinary transform helper rejects quarantined entities. Open this
		// exact generation only for its synchronous, already durable transition.
		state := ecs.GetResource[ecs.ObjectDestructionState](s.world)
		state.Pending[op.target] = false
		if !s.deps.TransformCommitted(op.target, op.replacementDef) {
			state.Pending[op.target] = true
			return false
		}
		return true
	}
	s.deps.Chunks.RemoveCommittedSource(op.source, op.id)
	if s.world.Alive(op.target) {
		if id, ok := s.world.GetExternalID(op.target); ok && id == op.id {
			s.world.Despawn(op.target)
		}
	}
	return true
}

func (s *ObjectDestructionService) release(op *objectDestructionOperation) {
	for _, coord := range op.pinned {
		s.deps.Chunks.UnpinPersistence(coord)
	}
	op.phase = destructionIdle
	op.err = nil
	op.capture = nil
	op.captured = false
	op.pinned = op.pinned[:0]
	op.total = 0
	op.extraFirst = 0
	op.lastID = 0
	op.allocated = false
	op.committed = false
	op.finalized = false
	op.cleanupRead = 0
	op.cursor = destructionLootCursor{}
	op.page = nil
	op.pageRead = 0
	op.replacement = nil
	op.replacementDef = nil
	if _, exists := s.reserved[op.target]; exists {
		s.reserved[op.target] = 0
	}
	s.freeSlots[s.freeCount] = op.slot
	s.freeCount++
	s.count--
}

func (s *ObjectDestructionService) StopAdmission()    { s.closed = true }
func (s *ObjectDestructionService) PendingCount() int { return s.count }

func (op *objectDestructionOperation) Complete(err error) {
	if err != nil && op.service.deps.Logger != nil {
		op.service.deps.Logger.Error("Object destruction persistence failed", zap.Uint64("object_id", uint64(op.id)), zap.Error(err))
	}
	op.mu.Lock()
	op.err = err
	op.phase = destructionReady
	op.mu.Unlock()
}

// Run never mutates ECS. The source is quarantined throughout incremental capture.
func (op *objectDestructionOperation) Run() error {
	s := op.service
	if !op.committed {
		// Once captured and assigned identities, only owned immutable data is
		// retried. A database failure never recaptures a different source tree.
		if !op.captured {
			if op.capture == nil {
				var refs []ecs.InventoryRefEntry
				var rootCount int
				s.deps.WithWorldRead(func(w *ecs.World) { rootCount = ecs.GetResource[ecs.InventoryRefIndex](w).OwnerEntryCount(op.id) })
				var buffer [ObjectDestructionCaptureBudget]ecs.InventoryRefEntry
				for start := 0; start < rootCount; start += ObjectDestructionCaptureBudget {
					var page []ecs.InventoryRefEntry
					valid := false
					s.deps.WithWorldRead(func(w *ecs.World) {
						index := ecs.GetResource[ecs.InventoryRefIndex](w)
						valid = w.Alive(op.target) && ecs.ObjectDestructionPending(w, op.target) && index.OwnerEntryCount(op.id) == rootCount
						if valid {
							page = index.EntriesByOwnerRangeInto(op.id, start, ObjectDestructionCaptureBudget, buffer[:0])
						}
					})
					if !valid {
						return ErrObjectDestructionCapture
					}
					refs = append(refs, page...)
				}
				if op.replacement != nil {
					op.capture = inventory.NewObjectLootCaptureForTransformation(op.id, refs)
				} else {
					op.capture = inventory.NewObjectLootCapture(op.id, refs)
				}
			}
			for {
				var done bool
				var err error
				s.deps.WithWorldRead(func(w *ecs.World) {
					if !w.Alive(op.target) || !ecs.ObjectDestructionPending(w, op.target) {
						err = ErrObjectDestructionCapture
						return
					}
					done, err = op.capture.CaptureBatch(w, s.deps.Items, ObjectDestructionCaptureBudget)
				})
				if err != nil {
					op.capture = nil
					return err
				}
				if done {
					break
				}
			}
			op.captured = true
		}
		if !op.allocated {
			var total, extras uint64
			lastID := op.capture.MaxItemID()
			for _, item := range op.capture.Items() {
				if uint64(item.Quantity) > math.MaxInt64-total {
					return ErrObjectDestructionOverflow
				}
				total += uint64(item.Quantity)
				extras += uint64(item.Quantity) - 1
			}
			var extraFirst types.EntityID
			if extras > 0 {
				first, last, err := s.deps.IDs.ReserveIDs(extras)
				if err != nil {
					return err
				}
				if first == 0 || last < first || last > math.MaxInt64 || uint64(last-first)+1 != extras || first <= op.capture.MaxItemID() {
					return ErrObjectDestructionOverflow
				}
				extraFirst = first
				if last > lastID {
					lastID = last
				}
			}
			op.total, op.extraFirst, op.lastID = total, extraFirst, lastID
			op.allocated = true
		}
		// The square can span at most four destination chunks. Pin them once and
		// serialize their I/O with source replacement; no lock is waited on by tick.
		for _, x := range []int{clampDrop(op.x-constt.DestroyedObjectDropSpread, s.deps.MinX, s.deps.MaxX-1), clampDrop(op.x+constt.DestroyedObjectDropSpread, s.deps.MinX, s.deps.MaxX-1)} {
			for _, y := range []int{clampDrop(op.y-constt.DestroyedObjectDropSpread, s.deps.MinY, s.deps.MaxY-1), clampDrop(op.y+constt.DestroyedObjectDropSpread, s.deps.MinY, s.deps.MaxY-1)} {
				coord := types.WorldToChunkCoord(x, y, constt.ChunkSize, constt.CoordPerTile)
				found := false
				for _, pin := range op.pinned {
					if pin == coord {
						found = true
						break
					}
				}
				if !found {
					if err := s.deps.Chunks.PinPersistence(coord); err != nil {
						return err
					}
					op.pinned = append(op.pinned, coord)
				}
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cursor := destructionLootCursor{}
		err := s.deps.Chunks.WithPersistence(op.pinned, func() error {
			next := func(dst []inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error) {
				return op.records(&cursor, dst, objectDestructionSQLBudget)
			}
			if op.replacement != nil {
				return s.deps.Persister.(objectLootTransformationPersister).TransformObjectWithDroppedItems(ctx, op.replacement, op.lastID, next)
			}
			return s.deps.Persister.ReplaceObjectWithDroppedItems(ctx, op.region, op.id, op.lastID, next)
		})
		if err != nil {
			return err
		}
		op.committed = true
	}
	records, err := op.records(&op.cursor, nil, ObjectDestructionDropBudget)
	if err != nil {
		return err
	}
	op.page = make([]destructionDroppedRecord, 0, len(records))
	for _, r := range records {
		op.page = append(op.page, destructionDroppedRecord{
			object:    &repository.Object{ID: int64(r.EntityID), TypeID: r.TypeID, Region: r.Region, Layer: r.Layer, X: r.X, Y: r.Y, ChunkX: r.ChunkX, ChunkY: r.ChunkY, Data: pqtype.NullRawMessage{RawMessage: r.ObjectData, Valid: true}, Hp: sql.NullFloat64{}},
			inventory: &repository.Inventory{OwnerID: int64(r.EntityID), Kind: int16(constt.InventoryDroppedItem), InventoryKey: 0, Data: r.InventoryData, Version: 1},
		})
	}
	return nil
}

func (op *objectDestructionOperation) records(cursor *destructionLootCursor, dst []inventory.DroppedItemPersistenceRecord, limit int) ([]inventory.DroppedItemPersistenceRecord, error) {
	dst = dst[:0]
	position := *cursor
	items := op.capture.Items()
	for position.item < len(items) && len(dst) < limit {
		item := items[position.item]
		id := item.ItemID
		if position.unit > 0 {
			id = op.extraFirst + types.EntityID(position.extra)
			position.extra++
		}
		x := clampDrop(op.x+dropOffset(op.seed, position.emitted*2), op.service.deps.MinX, op.service.deps.MaxX-1)
		y := clampDrop(op.y+dropOffset(op.seed, position.emitted*2+1), op.service.deps.MinY, op.service.deps.MaxY-1)
		coord := types.WorldToChunkCoord(x, y, constt.ChunkSize, constt.CoordPerTile)
		params := inventory.SpawnDroppedEntityParams{DroppedEntityID: id, ItemID: id, TypeID: item.TypeID, Resource: item.Resource, Quality: item.Quality, Quantity: 1, W: item.W, H: item.H, DropX: x, DropY: y, Region: op.region, Layer: op.layer, ChunkX: coord.X, ChunkY: coord.Y, NowRuntimeSeconds: op.dropTime}
		record, err := inventory.BuildDroppedItemPersistenceRecord(params, item.NestedInventory)
		if err != nil {
			return nil, err
		}
		dst = append(dst, record)
		position.unit++
		position.emitted++
		if position.unit == item.Quantity {
			position.item++
			position.unit = 0
		}
	}
	*cursor = position
	return dst, nil
}

func clampDrop(n, min, max int) int {
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

// Counter-based randomness keeps retry and streaming materialization identical
// without retaining one coordinate pair per stack unit.
func dropOffset(seed, index uint64) int {
	v := seed + (index+1)*0x9e3779b97f4a7c15
	v = (v ^ (v >> 30)) * 0xbf58476d1ce4e5b9
	v = (v ^ (v >> 27)) * 0x94d049bb133111eb
	v ^= v >> 31
	return int(v%uint64(2*constt.DestroyedObjectDropSpread+1)) - constt.DestroyedObjectDropSpread
}
