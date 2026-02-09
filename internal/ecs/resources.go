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

// LinkEntry stores a player↔object link with stable IDs for event emission.
type LinkEntry struct {
	PlayerHandle types.Handle
	PlayerID     types.EntityID
	ObjectHandle types.Handle
	ObjectID     types.EntityID
}

// LinkedObjects tracks player↔object links for interaction mechanics.
// Players can link to at most one object at a time.
type LinkedObjects struct {
	PlayerToObject  map[types.Handle]LinkEntry
	ObjectToPlayers map[types.Handle]map[types.Handle]struct{}
}

func (lo *LinkedObjects) Link(playerHandle types.Handle, playerID types.EntityID, objectHandle types.Handle, objectID types.EntityID) bool {
	if lo.PlayerToObject == nil {
		lo.PlayerToObject = make(map[types.Handle]LinkEntry, 64)
	}
	if lo.ObjectToPlayers == nil {
		lo.ObjectToPlayers = make(map[types.Handle]map[types.Handle]struct{}, 64)
	}
	if current, ok := lo.PlayerToObject[playerHandle]; ok {
		if current.ObjectHandle == objectHandle {
			return false
		}
		lo.unlinkNoEvent(playerHandle, current.ObjectHandle)
	}
	lo.PlayerToObject[playerHandle] = LinkEntry{
		PlayerHandle: playerHandle,
		PlayerID:     playerID,
		ObjectHandle: objectHandle,
		ObjectID:     objectID,
	}
	set := lo.ObjectToPlayers[objectHandle]
	if set == nil {
		set = make(map[types.Handle]struct{}, 4)
		lo.ObjectToPlayers[objectHandle] = set
	}
	set[playerHandle] = struct{}{}
	return true
}

func (lo *LinkedObjects) Unlink(playerHandle types.Handle) (LinkEntry, bool) {
	entry, ok := lo.PlayerToObject[playerHandle]
	if !ok {
		return LinkEntry{}, false
	}
	lo.unlinkNoEvent(playerHandle, entry.ObjectHandle)
	return entry, true
}

func (lo *LinkedObjects) UnlinkAllFromObject(objectHandle types.Handle) []LinkEntry {
	set := lo.ObjectToPlayers[objectHandle]
	if len(set) == 0 {
		return nil
	}
	entries := make([]LinkEntry, 0, len(set))
	for playerHandle := range set {
		if entry, ok := lo.PlayerToObject[playerHandle]; ok {
			entries = append(entries, entry)
			delete(lo.PlayerToObject, playerHandle)
		}
	}
	delete(lo.ObjectToPlayers, objectHandle)
	return entries
}

func (lo *LinkedObjects) unlinkNoEvent(playerHandle, objectHandle types.Handle) {
	delete(lo.PlayerToObject, playerHandle)
	if set, ok := lo.ObjectToPlayers[objectHandle]; ok {
		delete(set, playerHandle)
		if len(set) == 0 {
			delete(lo.ObjectToPlayers, objectHandle)
		}
	}
}

// OpenContainerEntry tracks a single opened inventory container for a player.
type OpenContainerEntry struct {
	Handle types.Handle
	OwnerID types.EntityID
	Kind   constt.InventoryKind
	Key    uint32
}

// PlayerOpenContainers stores containers opened by a player (object containers only).
type PlayerOpenContainers struct {
	Containers map[InventoryRefKey]OpenContainerEntry
}

// OpenContainers tracks opened object containers per player and reverse mappings.
type OpenContainers struct {
	PlayerToContainers map[types.EntityID]*PlayerOpenContainers
	ContainerToPlayers map[InventoryRefKey]map[types.EntityID]struct{}
}

func (oc *OpenContainers) ensure() {
	if oc.PlayerToContainers == nil {
		oc.PlayerToContainers = make(map[types.EntityID]*PlayerOpenContainers, 64)
	}
	if oc.ContainerToPlayers == nil {
		oc.ContainerToPlayers = make(map[InventoryRefKey]map[types.EntityID]struct{}, 64)
	}
}

func (oc *OpenContainers) Open(playerID types.EntityID, entry OpenContainerEntry) bool {
	oc.ensure()
	key := InventoryRefKey{Kind: entry.Kind, OwnerID: entry.OwnerID, Key: entry.Key}
	player := oc.PlayerToContainers[playerID]
	if player == nil {
		player = &PlayerOpenContainers{
			Containers: make(map[InventoryRefKey]OpenContainerEntry, 8),
		}
		oc.PlayerToContainers[playerID] = player
	}
	if _, exists := player.Containers[key]; exists {
		return false
	}
	player.Containers[key] = entry

	set := oc.ContainerToPlayers[key]
	if set == nil {
		set = make(map[types.EntityID]struct{}, 4)
		oc.ContainerToPlayers[key] = set
	}
	set[playerID] = struct{}{}
	return true
}

func (oc *OpenContainers) Has(playerID types.EntityID, key InventoryRefKey) bool {
	if oc.PlayerToContainers == nil {
		return false
	}
	player := oc.PlayerToContainers[playerID]
	if player == nil {
		return false
	}
	_, ok := player.Containers[key]
	return ok
}

func (oc *OpenContainers) Close(playerID types.EntityID, key InventoryRefKey) (OpenContainerEntry, bool) {
	if oc.PlayerToContainers == nil {
		return OpenContainerEntry{}, false
	}
	player := oc.PlayerToContainers[playerID]
	if player == nil {
		return OpenContainerEntry{}, false
	}
	entry, ok := player.Containers[key]
	if !ok {
		return OpenContainerEntry{}, false
	}
	delete(player.Containers, key)
	if len(player.Containers) == 0 {
		delete(oc.PlayerToContainers, playerID)
	}
	if set, ok := oc.ContainerToPlayers[key]; ok {
		delete(set, playerID)
		if len(set) == 0 {
			delete(oc.ContainerToPlayers, key)
		}
	}
	return entry, true
}

func (oc *OpenContainers) CloseAll(playerID types.EntityID) []OpenContainerEntry {
	if oc.PlayerToContainers == nil {
		return nil
	}
	player := oc.PlayerToContainers[playerID]
	if player == nil || len(player.Containers) == 0 {
		return nil
	}
	entries := make([]OpenContainerEntry, 0, len(player.Containers))
	for key, entry := range player.Containers {
		entries = append(entries, entry)
		if set, ok := oc.ContainerToPlayers[key]; ok {
			delete(set, playerID)
			if len(set) == 0 {
				delete(oc.ContainerToPlayers, key)
			}
		}
	}
	delete(oc.PlayerToContainers, playerID)
	return entries
}

func (oc *OpenContainers) PlayersFor(key InventoryRefKey) []types.EntityID {
	if oc.ContainerToPlayers == nil {
		return nil
	}
	set := oc.ContainerToPlayers[key]
	if len(set) == 0 {
		return nil
	}
	players := make([]types.EntityID, 0, len(set))
	for playerID := range set {
		players = append(players, playerID)
	}
	return players
}
