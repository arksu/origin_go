package world

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestObjectFactoryChunkExpiredResultNeverDeletesSynchronously(t *testing.T) {
	w := ecs.NewWorldForTesting()
	ecs.SetResource(w, ecs.TimeState{RuntimeSecondsTotal: 110})
	factory := NewObjectFactory(nil)
	deleter := &dropExpiryTestDeleter{}
	factory.SetObjectDeleter(deleter)
	handle, err := factory.BuildForChunk(w, expiredDropRaw(t, 9800), nil)
	require.Equal(t, types.InvalidHandle, handle)
	require.ErrorIs(t, err, ErrDroppedItemExpired)
	require.Zero(t, deleter.calls)
	require.Zero(t, w.EntityCount())
}

func TestObjectFactoryChunkDroppedRestoreAllocationParity(t *testing.T) {
	installCapacityObjectDefinitions(t)
	w := ecs.NewWorldForTesting()
	ecs.SetResource(w, ecs.TimeState{RuntimeSecondsTotal: 105})
	factory := NewObjectFactory(nil)
	raw := expiredDropRaw(t, 9801)
	rows := []repository.Inventory{{OwnerID: 9801, Kind: int16(constt.InventoryDroppedItem), Version: 1,
		Data: []byte(`{"kind":3,"key":0,"width":1,"height":1,"version":1,"items":[{"item_id":9801,"type_id":9403,"quality":10,"quantity":1}]}`)}}
	measure := func(chunkBuild bool) float64 {
		return testing.AllocsPerRun(100, func() {
			var handle types.Handle
			var err error
			if chunkBuild {
				handle, err = factory.BuildForChunk(w, raw, rows)
			} else {
				handle, err = factory.Build(w, raw, rows)
			}
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			index := ecs.GetResource[ecs.InventoryRefIndex](w)
			container, found := index.Lookup(constt.InventoryDroppedItem, 9801, 0)
			if !found {
				t.Fatal("missing dropped inventory")
			}
			index.Remove(constt.InventoryDroppedItem, 9801, 0)
			w.Despawn(container)
			w.Despawn(handle)
		})
	}
	baseline := measure(false)
	require.Equal(t, baseline, measure(true), "chunk restoration must use the same single metadata parse")
}
