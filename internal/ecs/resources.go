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

type ObserverVisibility struct {
	Known          map[types.Handle]types.EntityID // кого видит эта сущность (Handle -> EntityID)
	NextUpdateTime time.Time
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
