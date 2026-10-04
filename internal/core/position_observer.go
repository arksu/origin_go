package core

import "origin/internal/types"

// PositionObserver keeps secondary spatial indexes aligned with committed
// transforms, including immediate relocations outside the movement pipeline.
type PositionObserver interface {
	OnPositionCommitted(handle types.Handle, positionX, positionY float64)
}

// PositionObservers preserves every secondary index when a new observer joins.
type PositionObservers []PositionObserver

func (observers PositionObservers) OnPositionCommitted(handle types.Handle, positionX, positionY float64) {
	for _, observer := range observers {
		observer.OnPositionCommitted(handle, positionX, positionY)
	}
}
