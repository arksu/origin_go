package ecs

import (
	"math"
	"testing"

	constt "origin/internal/const"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestInventoryRefOwnerIndex(t *testing.T) {
	var index InventoryRefIndex
	index.Add(constt.InventoryHand, 10, 0, 103)
	index.Add(constt.InventoryGrid, 20, 0, 200)
	index.Add(constt.InventoryGrid, 10, 9, 102)
	index.Add(constt.InventoryGrid, 10, 1, 101)
	index.Add(constt.InventoryGrid, 10, 1, 111)
	entries := index.EntriesByOwnerInto(10, nil)
	require.Len(t, entries, 3)
	require.Equal(t, types.Handle(111), entries[0].Handle)
	require.Equal(t, uint32(1), entries[0].Key)
	require.Equal(t, uint32(9), entries[1].Key)
	require.Equal(t, constt.InventoryHand, entries[2].Kind)
	require.Equal(t, 3, index.OwnerEntryCount(10))
	entries[0].Handle = 999
	actual, found := index.Lookup(constt.InventoryGrid, 10, 1)
	require.True(t, found)
	require.Equal(t, types.Handle(111), actual)

	index.Remove(constt.InventoryGrid, 10, 9)
	index.Remove(constt.InventoryGrid, 10, 9) // Repeated removal preserves both indexes.
	require.Equal(t, 2, index.OwnerEntryCount(10))
	require.Equal(t, []types.Handle{111, 103}, index.RemoveAllByOwner(10))
	require.Zero(t, index.OwnerEntryCount(10))
	_, found = index.Lookup(constt.InventoryGrid, 10, 1)
	require.False(t, found)
	actual, found = index.Lookup(constt.InventoryGrid, 20, 0)
	require.True(t, found)
	require.Equal(t, types.Handle(200), actual)
	require.Nil(t, index.RemoveAllByOwner(10))
	index.Add(constt.InventoryGrid, 10, 2, 112)
	require.Equal(t, 1, index.OwnerEntryCount(10))
	index.Remove(constt.InventoryGrid, 10, 2)
	require.Zero(t, index.OwnerEntryCount(10))
}

func TestInventoryRefOwnerEnumerationAllocs(t *testing.T) {
	var index InventoryRefIndex
	for i := types.EntityID(1); i <= 100_000; i++ {
		index.Add(constt.InventoryGrid, i, 0, types.Handle(i))
	}
	index.Add(constt.InventoryGrid, 1, 1, 100_001)
	buffer := make([]InventoryRefEntry, 0, 2)
	allocations := testing.AllocsPerRun(1000, func() {
		buffer = index.EntriesByOwnerInto(1, buffer[:0])
	})
	require.Zero(t, allocations)
	require.Len(t, buffer, 2)
	allocations = testing.AllocsPerRun(1000, func() {
		buffer = index.EntriesByOwnerRangeInto(1, 1, 100, buffer[:0])
	})
	require.Zero(t, allocations)
	require.Len(t, buffer, 1)
	require.Equal(t, uint32(1), buffer[0].Key)
}

func TestInventoryRefOwnerRange(t *testing.T) {
	var index InventoryRefIndex
	for key := uint32(5); key > 0; key-- {
		index.Add(constt.InventoryGrid, 10, key, types.Handle(key))
	}
	for _, scenario := range []struct {
		start, limit int
		keys         []uint32
	}{
		{0, 2, []uint32{1, 2}}, {2, 2, []uint32{3, 4}}, {4, 2, []uint32{5}},
		{2, math.MaxInt, []uint32{3, 4, 5}}, {5, 2, nil}, {-1, 2, nil}, {0, 0, nil}, {0, -1, nil},
	} {
		buffer := []InventoryRefEntry{{InventoryRefKey: InventoryRefKey{Key: 99}}}
		entries := index.EntriesByOwnerRangeInto(10, scenario.start, scenario.limit, buffer)
		require.Equal(t, uint32(99), entries[0].Key, "existing destination prefix must be preserved")
		var keys []uint32
		for _, entry := range entries[1:] {
			keys = append(keys, entry.Key)
		}
		require.Equal(t, scenario.keys, keys)
	}
	require.Nil(t, index.EntriesByOwnerRangeInto(20, 0, 100, nil))
	copy := index.EntriesByOwnerRangeInto(10, 0, 2, nil)
	index.Add(constt.InventoryGrid, 10, 1, 100)
	require.Equal(t, types.Handle(1), copy[0].Handle, "range must own its reference values")
}

func TestInventoryRefOwnerRegistrationReusesCapacity(t *testing.T) {
	var index InventoryRefIndex
	allocations := testing.AllocsPerRun(1000, func() {
		index.Add(constt.InventoryGrid, 10, 0, 101)
		index.Add(constt.InventoryGrid, 10, 1, 102)
		index.Remove(constt.InventoryGrid, 10, 0)
		index.Remove(constt.InventoryGrid, 10, 1)
	})
	require.Zero(t, allocations)
	require.Empty(t, index.byOwner, "removed owner IDs must not accumulate")
	require.Len(t, index.freeOwners, 1)
}

func BenchmarkInventoryRefOwnerEnumeration(b *testing.B) {
	for _, owners := range []int{1_000, 100_000} {
		b.Run(map[int]string{1_000: "Owners1000", 100_000: "Owners100000"}[owners], func(b *testing.B) {
			var index InventoryRefIndex
			for i := 1; i <= owners; i++ {
				index.Add(constt.InventoryGrid, types.EntityID(i), 0, types.Handle(i))
			}
			index.Add(constt.InventoryGrid, 1, 1, 100_001)
			buffer := make([]InventoryRefEntry, 0, 2)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				buffer = index.EntriesByOwnerInto(1, buffer[:0])
			}
		})
	}
}
