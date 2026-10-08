package game

import (
	"math"
	"testing"

	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestEntityIDReservationPreservesContiguousRanges(t *testing.T) {
	allocator := &EntityIDManager{currentID: 40, rangeEnd: 50}
	first, last, err := allocator.ReserveIDs(3)
	require.NoError(t, err)
	require.Equal(t, types.EntityID(41), first)
	require.Equal(t, types.EntityID(43), last)
	first, last, err = allocator.ReserveIDs(10)
	require.NoError(t, err)
	require.Equal(t, types.EntityID(44), first)
	require.Equal(t, types.EntityID(53), last)
	require.Equal(t, uint64(53), allocator.rangeEnd)
	first, last, err = allocator.ReserveIDs(0)
	require.NoError(t, err)
	require.Zero(t, first)
	require.Zero(t, last)
	allocator.currentID = math.MaxInt64 - 1
	_, _, err = allocator.ReserveIDs(2)
	require.ErrorIs(t, err, ErrEntityIDExhausted)
	require.Equal(t, uint64(math.MaxInt64-1), allocator.currentID)
}
