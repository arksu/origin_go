package game

import (
	"testing"

	"origin/internal/ecs/components"
)

func TestLiftPutDownTargetPositionUsesRequestedTargetForAllObjectTypes(t *testing.T) {
	testCases := []struct {
		name               string
		usesObjectCollider bool
	}{
		{name: "object with collider", usesObjectCollider: true},
		{name: "object without collider", usesObjectCollider: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			pending := components.PendingLiftTransition{
				TargetX:            123.5,
				TargetY:            456.25,
				UsesObjectCollider: testCase.usesObjectCollider,
			}

			x, y := liftPutDownTargetPosition(pending)
			if x != pending.TargetX || y != pending.TargetY {
				t.Fatalf("liftPutDownTargetPosition() = (%v, %v), want (%v, %v)", x, y, pending.TargetX, pending.TargetY)
			}
		})
	}
}
