package core

import "origin/internal/types"

// PositionObserver keeps secondary spatial indexes aligned with committed
// transforms, including immediate relocations outside the movement pipeline.
type PositionObserver interface {
	OnPositionCommitted(handle types.Handle, positionX, positionY float64)
}
