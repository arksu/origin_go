package ecs

import (
	"testing"

	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestInventoryReservationsExactIdentityAndRelease(t *testing.T) {
	w := NewWorldForTesting()
	owner, other := w.Spawn(10, nil), w.Spawn(20, nil)
	require.False(t, InventoryOwnerReserved(w, 10))
	require.False(t, ReserveInventoryOwner(nil, 10, owner))
	require.False(t, ReserveInventoryOwner(w, 0, owner))
	require.False(t, ReserveInventoryOwner(w, 10, other))
	require.True(t, ReserveInventoryOwner(w, 10, owner))
	require.True(t, InventoryOwnerReserved(w, 10))
	require.True(t, InventoryHandleReserved(w, owner))
	require.False(t, InventoryHandleReserved(w, other))
	require.False(t, ReserveInventoryOwner(w, 10, owner), "a second claim cannot share a reservation")
	require.False(t, ReleaseInventoryOwner(w, 10, other))
	require.True(t, InventoryOwnerReserved(w, 10))
	require.True(t, ReleaseInventoryOwner(w, 10, owner))
	require.False(t, InventoryOwnerReserved(w, 10))
	require.False(t, ReleaseInventoryOwner(w, 10, owner))
}

func TestInventoryReservationsDespawnCannotAffectReplacementGeneration(t *testing.T) {
	w := NewWorldForTesting()
	old := w.Spawn(10, nil)
	require.True(t, ReserveInventoryOwner(w, 10, old))
	w.Despawn(old)
	require.Empty(t, GetResource[InventoryReservations](w).Owners)
	replacement := w.Spawn(10, nil)
	require.NotEqual(t, old, replacement)
	require.False(t, InventoryHandleReserved(w, old))
	require.True(t, ReserveInventoryOwner(w, 10, replacement))
	require.False(t, ReleaseInventoryOwner(w, 10, old))
	require.True(t, InventoryHandleReserved(w, replacement))
}

func TestInventoryReservationHotChecksDoNotAllocate(t *testing.T) {
	w := NewWorldForTesting()
	owner := w.Spawn(10, nil)
	require.True(t, ReserveInventoryOwner(w, 10, owner))
	require.Zero(t, testing.AllocsPerRun(100, func() {
		if !InventoryOwnerReserved(w, 10) || !InventoryHandleReserved(w, owner) || InventoryOwnerReserved(w, 20) || InventoryHandleReserved(w, types.InvalidHandle) {
			panic("unexpected reservation lookup")
		}
	}))
	require.True(t, ReleaseInventoryOwner(w, 10, owner))
	require.Zero(t, testing.AllocsPerRun(100, func() {
		if !ReserveInventoryOwner(w, 10, owner) || !ReleaseInventoryOwner(w, 10, owner) {
			panic("unexpected reservation lifecycle")
		}
	}))
}
