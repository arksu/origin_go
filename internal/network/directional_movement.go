package network

import (
	"math"

	netproto "origin/internal/network/proto"
)

// ValidMoveDirection validates the wire vector before any session or gameplay effects.
func ValidMoveDirection(direction *netproto.MoveDirection) bool {
	if direction == nil || direction.InputRevision == 0 {
		return false
	}
	x, y := float64(direction.X), float64(direction.Y)
	return !math.IsNaN(x) && !math.IsNaN(y) && x >= -1 && x <= 1 && y >= -1 && y <= 1
}
