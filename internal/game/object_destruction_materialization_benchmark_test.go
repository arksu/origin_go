package game

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
)

// BenchmarkObjectDestructionDropMaterialization measures the actual dropped
// ObjectFactory Build path, including bounded teardown, in a warmed World.
func BenchmarkObjectDestructionDropMaterialization(b *testing.B) {
	installObjectDestructionIntegrationDefs(b)
	for _, population := range []struct {
		name  string
		count int
	}{{"Entities1000", 1000}, {"Entities100000", 100000}} {
		for _, scenario := range []struct {
			name   string
			typeID uint32
			bag    bool
		}{{"Simple", 1, false}, {"Bag", 2, true}} {
			b.Run(population.name+"/"+scenario.name, func(b *testing.B) {
				w := ecs.NewWorldWithCapacity(uint32(population.count+8), nil, 0)
				for i := 0; i < population.count; i++ {
					w.Spawn(types.EntityID(10000+i), nil)
				}
				var nested *inventory.InventoryDataV1
				if scenario.bag {
					nested = &inventory.InventoryDataV1{Kind: uint8(constt.InventoryGrid), Width: 4, Height: 4, Version: 2, Items: []inventory.InventoryItemV1{{ItemID: 900, TypeID: 1, Quality: 23, Quantity: 2}}}
				}
				record, err := inventory.BuildDroppedItemPersistenceRecord(inventory.SpawnDroppedEntityParams{
					DroppedEntityID: 30, ItemID: 30, TypeID: scenario.typeID, Resource: scenario.name, Quality: 17, Quantity: 1, W: 1, H: 1, DropX: 50, DropY: 50, Region: 1, NowRuntimeSeconds: 10,
				}, nested)
				if err != nil {
					b.Fatal(err)
				}
				raw := repository.Object{ID: 30, TypeID: constt.DroppedItemTypeID, Region: 1, X: 50, Y: 50, Data: pqtype.NullRawMessage{RawMessage: record.ObjectData, Valid: true}}
				rows := []repository.Inventory{{OwnerID: 30, Kind: int16(constt.InventoryDroppedItem), Version: 1, Data: record.InventoryData}}
				factory := gameworld.NewObjectFactory(nil)
				index := ecs.GetResource[ecs.InventoryRefIndex](w)
				buffer := make([]ecs.InventoryRefEntry, 0, 2)
				buildAndRemove := func() {
					handle, err := factory.Build(w, &raw, rows)
					if err != nil {
						b.Fatal(err)
					}
					buffer = index.EntriesByOwnerInto(30, buffer[:0])
					for _, ref := range buffer {
						index.Remove(ref.Kind, ref.OwnerID, ref.Key)
						w.Despawn(ref.Handle)
					}
					w.Despawn(handle)
				}
				buildAndRemove()
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					buildAndRemove()
				}
			})
		}
	}
}
