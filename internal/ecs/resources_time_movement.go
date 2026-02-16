package ecs

import (
	"sync"
	"time"

	"origin/internal/types"
)

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

type BehaviorTickPolicy struct {
	GlobalBudgetPerTick int
	CatchUpLimitTicks   uint64
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
