package ecs

import (
	"fmt"
	"reflect"

	constt "origin/internal/const"
	"origin/internal/types"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Typed Resource API
// ---------------------------------------------------------------------------
// Resources are singleton data shared across systems, stored by type.
// Usage:
//   ecs.InitResource(w, MovedEntities{...})   // at world init
//   res := ecs.GetResource[MovedEntities](w)  // in systems — returns *T, panics if missing
//   ecs.SetResource(w, value)                 // replace value
//   ok  := ecs.HasResource[MovedEntities](w)  // check existence
// ---------------------------------------------------------------------------

// resourceKey returns the reflect.Type used as map key for type T.
func resourceKey[T any]() reflect.Type {
	return reflect.TypeOf((*T)(nil)).Elem()
}

// InitResource stores an initial resource value in the World.
// Intended for use during World construction. Returns a pointer to the stored value.
func InitResource[T any](w *World, value T) *T {
	key := resourceKey[T]()
	ptr := new(T)
	*ptr = value
	w.resources[key] = ptr
	return ptr
}

// SetResource replaces (or inserts) a resource value in the World.
func SetResource[T any](w *World, value T) {
	key := resourceKey[T]()
	ptr := new(T)
	*ptr = value
	w.resources[key] = ptr
}

// GetResource returns a pointer to the resource of type T.
// Panics if the resource was never initialised — this is a programming error.
func GetResource[T any](w *World) *T {
	key := resourceKey[T]()
	v, ok := w.resources[key]
	if !ok {
		panic(fmt.Sprintf("ecs.GetResource: resource %v not initialised", key))
	}
	return v.(*T)
}

// TryGetResource returns a pointer and a bool indicating existence.
func TryGetResource[T any](w *World) (*T, bool) {
	key := resourceKey[T]()
	v, ok := w.resources[key]
	if !ok {
		return nil, false
	}
	return v.(*T), true
}

// HasResource checks whether a resource of type T exists.
func HasResource[T any](w *World) bool {
	key := resourceKey[T]()
	_, ok := w.resources[key]
	return ok
}

// TimeState holds per-tick time data, updated once before systems run.
// Systems read time exclusively from this resource instead of calling time.Now().
type TimeState struct {
	Tick       uint64
	TickRate   int
	TickPeriod time.Duration
	Delta      float64       // Fixed-step dt in seconds
	Now        time.Time     // Monotonic game time at tick start
	UnixMs     int64         // Now.UnixMilli() — for network packets
	Uptime     time.Duration // Time since server start
}

// MovedEntities tracks entities that moved during the current frame
type MovedEntities struct {
	Handles []types.Handle
	IntentX []float64
	IntentY []float64
	Count   int
}

func (me *MovedEntities) Add(h types.Handle, x, y float64) {
	if me.Count >= len(me.Handles) {
		me.grow()
	}
	me.Handles[me.Count] = h
	me.IntentX[me.Count] = x
	me.IntentY[me.Count] = y
	me.Count++
}

func (me *MovedEntities) grow() {
	newCap := len(me.Handles) * 2
	if newCap == 0 {
		newCap = 256
	}
	newHandles := make([]types.Handle, newCap)
	copy(newHandles, me.Handles)
	me.Handles = newHandles

	newIntentX := make([]float64, newCap)
	copy(newIntentX, me.IntentX)
	me.IntentX = newIntentX

	newIntentY := make([]float64, newCap)
	copy(newIntentY, me.IntentY)
	me.IntentY = newIntentY
}

type VisibilityState struct {
	// кого видит эта сущность
	VisibleByObserver map[types.Handle]ObserverVisibility
	// кто видит эту сущность, у кого я нахожусь в списке Known
	// нужно для отправки пакетов (broadcast) о событиях, отправляем только тем, кто меня видит.
	ObserversByVisibleTarget map[types.Handle]map[types.Handle]struct{}
	Mu                       sync.RWMutex // Protects visibility maps for concurrent access
}

type ChunkGen struct {
	Coord types.ChunkCoord
	Gen   uint64
}

type ObserverVisibility struct {
	Known          map[types.Handle]types.EntityID // кого видит эта сущность (Handle -> EntityID)
	NextUpdateTime time.Time
	LastX          float64
	LastY          float64
	LastChunkX     int
	LastChunkY     int
	LastChunkGens  []ChunkGen // chunk generations at last vision update (dirty-flag skip)
}

type DetachedEntities struct {
	Map map[types.EntityID]DetachedEntity
}

// DetachedEntity represents a player entity that has disconnected but remains in the world
type DetachedEntity struct {
	Handle         types.Handle
	ExpirationTime time.Time
	DetachedAt     time.Time
}

// AddDetachedEntity adds an entity to the detached entities map
func (d *DetachedEntities) AddDetachedEntity(entityID types.EntityID, handle types.Handle, expirationTime time.Time, detachedAt time.Time) {
	d.Map[entityID] = DetachedEntity{
		Handle:         handle,
		ExpirationTime: expirationTime,
		DetachedAt:     detachedAt,
	}
}

// RemoveDetachedEntity removes an entity from the detached entities map
func (d *DetachedEntities) RemoveDetachedEntity(entityID types.EntityID) {
	delete(d.Map, entityID)
}

// GetDetachedEntity returns a detached entity by EntityID
func (d *DetachedEntities) GetDetachedEntity(entityID types.EntityID) (DetachedEntity, bool) {
	entity, ok := d.Map[entityID]
	return entity, ok
}

// IsDetached checks if an entity is in detached state
func (d *DetachedEntities) IsDetached(entityID types.EntityID) bool {
	_, ok := d.Map[entityID]
	return ok
}

type CharacterEntities struct {
	Map map[types.EntityID]CharacterEntity
}

type CharacterEntity struct {
	Handle     types.Handle
	LastSaveAt time.Time
	NextSaveAt time.Time
	SavesCount int
}

func (c *CharacterEntities) Add(entityID types.EntityID, handle types.Handle, nextSaveAt time.Time) {
	c.Map[entityID] = CharacterEntity{
		Handle:     handle,
		LastSaveAt: time.Time{},
		NextSaveAt: nextSaveAt,
		SavesCount: 0,
	}
}

func (c *CharacterEntities) Remove(entityID types.EntityID) {
	delete(c.Map, entityID)
}

func (c *CharacterEntities) UpdateSaveTime(entityID types.EntityID, lastSaveAt, nextSaveAt time.Time) {
	if entity, ok := c.Map[entityID]; ok {
		entity.LastSaveAt = lastSaveAt
		entity.NextSaveAt = nextSaveAt
		entity.SavesCount++
		c.Map[entityID] = entity
	}
}

// GetAll returns all character entity IDs
func (c *CharacterEntities) GetAll() []types.EntityID {
	entityIDs := make([]types.EntityID, 0, len(c.Map))
	for entityID := range c.Map {
		entityIDs = append(entityIDs, entityID)
	}
	return entityIDs
}

// InventoryRefKey uniquely identifies an inventory container: (kind, owner_id, inventory_key)
type InventoryRefKey struct {
	Kind    constt.InventoryKind
	OwnerID types.EntityID
	Key     uint32
}

// InventoryRefIndex provides O(1) lookup from InventoryRef to Handle
type InventoryRefIndex struct {
	index map[InventoryRefKey]types.Handle
}

func (idx *InventoryRefIndex) Add(kind constt.InventoryKind, ownerID types.EntityID, key uint32, handle types.Handle) {
	idx.index[InventoryRefKey{Kind: kind, OwnerID: ownerID, Key: key}] = handle
}

func (idx *InventoryRefIndex) Remove(kind constt.InventoryKind, ownerID types.EntityID, key uint32) {
	delete(idx.index, InventoryRefKey{Kind: kind, OwnerID: ownerID, Key: key})
}

func (idx *InventoryRefIndex) Lookup(kind constt.InventoryKind, ownerID types.EntityID, key uint32) (types.Handle, bool) {
	h, ok := idx.index[InventoryRefKey{Kind: kind, OwnerID: ownerID, Key: key}]
	return h, ok
}

// RemoveAllByOwner removes all inventory refs for the owner and returns removed handles.
func (idx *InventoryRefIndex) RemoveAllByOwner(ownerID types.EntityID) []types.Handle {
	if len(idx.index) == 0 {
		return nil
	}

	removed := make([]types.Handle, 0, 2)
	for key, handle := range idx.index {
		if key.OwnerID != ownerID {
			continue
		}
		delete(idx.index, key)
		removed = append(removed, handle)
	}
	return removed
}

// PlayerLink stores the current one-to-one link between a player and a world target.
type PlayerLink struct {
	PlayerID     types.EntityID
	PlayerHandle types.Handle
	TargetID     types.EntityID
	TargetHandle types.Handle

	// Snapshot positions taken at link creation.
	PlayerX float64
	PlayerY float64
	TargetX float64
	TargetY float64

	CreatedAt time.Time
}

// LinkIntent stores explicit player intent to link with a specific target.
// Link is created only when collision confirms physical contact with this target.
type LinkIntent struct {
	TargetID     types.EntityID
	TargetHandle types.Handle
	CreatedAt    time.Time
}

// LinkState keeps active links and reverse index in ECS resource storage.
//
// Invariants:
// - one player has at most one active link (LinkedByPlayer)
// - PlayersByTarget is the reverse index for fast fanout.
type LinkState struct {
	LinkedByPlayer  map[types.EntityID]PlayerLink
	PlayersByTarget map[types.EntityID]map[types.EntityID]struct{}
	IntentByPlayer  map[types.EntityID]LinkIntent
}

func (s *LinkState) SetIntent(playerID, targetID types.EntityID, targetHandle types.Handle, createdAt time.Time) {
	s.IntentByPlayer[playerID] = LinkIntent{
		TargetID:     targetID,
		TargetHandle: targetHandle,
		CreatedAt:    createdAt,
	}
}

func (s *LinkState) ClearIntent(playerID types.EntityID) {
	delete(s.IntentByPlayer, playerID)
}

func (s *LinkState) SetLink(link PlayerLink) {
	// Keep reverse index consistent when relinking.
	if prev, ok := s.LinkedByPlayer[link.PlayerID]; ok && prev.TargetID != link.TargetID {
		if players, found := s.PlayersByTarget[prev.TargetID]; found {
			delete(players, link.PlayerID)
			if len(players) == 0 {
				delete(s.PlayersByTarget, prev.TargetID)
			}
		}
	}

	s.LinkedByPlayer[link.PlayerID] = link

	players, ok := s.PlayersByTarget[link.TargetID]
	if !ok {
		players = make(map[types.EntityID]struct{}, 4)
		s.PlayersByTarget[link.TargetID] = players
	}
	players[link.PlayerID] = struct{}{}
}

func (s *LinkState) RemoveLink(playerID types.EntityID) (PlayerLink, bool) {
	link, ok := s.LinkedByPlayer[playerID]
	if !ok {
		return PlayerLink{}, false
	}

	delete(s.LinkedByPlayer, playerID)

	if players, found := s.PlayersByTarget[link.TargetID]; found {
		delete(players, playerID)
		if len(players) == 0 {
			delete(s.PlayersByTarget, link.TargetID)
		}
	}

	return link, true
}

func (s *LinkState) GetLink(playerID types.EntityID) (PlayerLink, bool) {
	link, ok := s.LinkedByPlayer[playerID]
	return link, ok
}

// OpenContainerState tracks per-player opened world object containers and nested refs.
//
// Invariants:
// - One player can have only one opened root world object at a time.
// - Opened refs are tracked per player and reverse-indexed for fanout updates.
type OpenContainerState struct {
	// PendingAutoOpenByPlayer stores object IDs that must be opened
	// immediately after successful link creation.
	PendingAutoOpenByPlayer map[types.EntityID]types.EntityID

	// OpenRootByPlayer maps player -> opened root world-object owner_id.
	OpenRootByPlayer map[types.EntityID]types.EntityID

	// PlayersByRoot maps world-object owner_id -> set(playerID).
	PlayersByRoot map[types.EntityID]map[types.EntityID]struct{}

	// OpenRefsByPlayer maps player -> set(opened inventory refs), includes root and nested.
	OpenRefsByPlayer map[types.EntityID]map[InventoryRefKey]struct{}

	// PlayersByRef maps inventory ref -> set(playerID) for update fanout.
	PlayersByRef map[InventoryRefKey]map[types.EntityID]struct{}
}

func (s *OpenContainerState) SetPendingAutoOpen(playerID, targetID types.EntityID) {
	s.PendingAutoOpenByPlayer[playerID] = targetID
}

func (s *OpenContainerState) ClearPendingAutoOpen(playerID types.EntityID) {
	delete(s.PendingAutoOpenByPlayer, playerID)
}

func (s *OpenContainerState) GetPendingAutoOpen(playerID types.EntityID) (types.EntityID, bool) {
	targetID, ok := s.PendingAutoOpenByPlayer[playerID]
	return targetID, ok
}

func (s *OpenContainerState) SetRootOpened(playerID, rootOwnerID types.EntityID) {
	if prevRoot, ok := s.OpenRootByPlayer[playerID]; ok && prevRoot != rootOwnerID {
		if players, found := s.PlayersByRoot[prevRoot]; found {
			delete(players, playerID)
			if len(players) == 0 {
				delete(s.PlayersByRoot, prevRoot)
			}
		}
	}

	s.OpenRootByPlayer[playerID] = rootOwnerID
	players, ok := s.PlayersByRoot[rootOwnerID]
	if !ok {
		players = make(map[types.EntityID]struct{}, 4)
		s.PlayersByRoot[rootOwnerID] = players
	}
	players[playerID] = struct{}{}
}

func (s *OpenContainerState) GetOpenedRoot(playerID types.EntityID) (types.EntityID, bool) {
	rootID, ok := s.OpenRootByPlayer[playerID]
	return rootID, ok
}

func (s *OpenContainerState) OpenRef(playerID types.EntityID, key InventoryRefKey) {
	refs, ok := s.OpenRefsByPlayer[playerID]
	if !ok {
		refs = make(map[InventoryRefKey]struct{}, 4)
		s.OpenRefsByPlayer[playerID] = refs
	}
	refs[key] = struct{}{}

	players, ok := s.PlayersByRef[key]
	if !ok {
		players = make(map[types.EntityID]struct{}, 4)
		s.PlayersByRef[key] = players
	}
	players[playerID] = struct{}{}
}

func (s *OpenContainerState) CloseRef(playerID types.EntityID, key InventoryRefKey) bool {
	refs, ok := s.OpenRefsByPlayer[playerID]
	if !ok {
		return false
	}
	if _, found := refs[key]; !found {
		return false
	}

	delete(refs, key)
	if len(refs) == 0 {
		delete(s.OpenRefsByPlayer, playerID)
	}

	if players, found := s.PlayersByRef[key]; found {
		delete(players, playerID)
		if len(players) == 0 {
			delete(s.PlayersByRef, key)
		}
	}
	return true
}

func (s *OpenContainerState) IsRefOpened(playerID types.EntityID, key InventoryRefKey) bool {
	refs, ok := s.OpenRefsByPlayer[playerID]
	if !ok {
		return false
	}
	_, found := refs[key]
	return found
}

func (s *OpenContainerState) PlayersOpenedRef(key InventoryRefKey) map[types.EntityID]struct{} {
	return s.PlayersByRef[key]
}

// CloseAllForPlayer removes all opened refs and root state for player and returns closed refs.
func (s *OpenContainerState) CloseAllForPlayer(playerID types.EntityID) []InventoryRefKey {
	closed := make([]InventoryRefKey, 0, 4)
	refs, hasRefs := s.OpenRefsByPlayer[playerID]
	if hasRefs {
		for key := range refs {
			closed = append(closed, key)
		}
	}

	for _, key := range closed {
		s.CloseRef(playerID, key)
	}

	if rootID, ok := s.OpenRootByPlayer[playerID]; ok {
		delete(s.OpenRootByPlayer, playerID)
		if players, found := s.PlayersByRoot[rootID]; found {
			delete(players, playerID)
			if len(players) == 0 {
				delete(s.PlayersByRoot, rootID)
			}
		}
	}

	delete(s.PendingAutoOpenByPlayer, playerID)
	return closed
}

// ObjectBehaviorDirtyQueue tracks object handles that require behavior recomputation.
// It is deduplicated and drained by ObjectBehaviorSystem with a per-tick budget.
type ObjectBehaviorDirtyQueue struct {
	pending []types.Handle
	inQueue map[types.Handle]struct{}
	head    int
}

func (q *ObjectBehaviorDirtyQueue) Mark(handle types.Handle) bool {
	if handle == types.InvalidHandle {
		return false
	}
	if _, exists := q.inQueue[handle]; exists {
		return false
	}
	q.inQueue[handle] = struct{}{}
	q.pending = append(q.pending, handle)
	return true
}

func (q *ObjectBehaviorDirtyQueue) Drain(max int, dst []types.Handle) []types.Handle {
	if max <= 0 {
		max = len(q.pending) - q.head
	}
	if max <= 0 {
		return dst
	}

	drained := 0
	for q.head < len(q.pending) && drained < max {
		h := q.pending[q.head]
		q.head++
		delete(q.inQueue, h)
		dst = append(dst, h)
		drained++
	}

	q.compact()
	return dst
}

func (q *ObjectBehaviorDirtyQueue) PendingCount() int {
	return len(q.pending) - q.head
}

func (q *ObjectBehaviorDirtyQueue) compact() {
	if q.head == 0 {
		return
	}
	if q.head == len(q.pending) {
		q.pending = q.pending[:0]
		q.head = 0
		return
	}
	if q.head > 1024 || q.head*2 >= len(q.pending) {
		copy(q.pending, q.pending[q.head:])
		q.pending = q.pending[:len(q.pending)-q.head]
		q.head = 0
	}
}

func MarkObjectBehaviorDirty(w *World, handle types.Handle) bool {
	if handle == types.InvalidHandle || !w.Alive(handle) {
		return false
	}
	queue := GetResource[ObjectBehaviorDirtyQueue](w)
	return queue.Mark(handle)
}

func MarkObjectBehaviorDirtyByEntityID(w *World, entityID types.EntityID) bool {
	if entityID == 0 {
		return false
	}
	handle := w.GetHandleByEntityID(entityID)
	if handle == types.InvalidHandle {
		return false
	}
	return MarkObjectBehaviorDirty(w, handle)
}
