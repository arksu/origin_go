package ecs

import (
	"origin/internal/types"
	"time"
)

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
}

type ObserverVisibility struct {
	Known          map[types.Handle]struct{} // кого видит эта сущность
	NextUpdateTime time.Time
}
