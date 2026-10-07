package systems

import (
	"context"
	"fmt"
	"testing"

	"origin/internal/characterattrs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"go.uber.org/zap"
)

var (
	characterBenchSHP, characterBenchHHP float64
	characterBenchLying                  bool
	characterBenchParams                 repository.UpdateCharactersParams
	characterBenchInventories            repository.UpsertInventoriesParams
)

func characterBenchFixture(b *testing.B, withInventories bool) (*CharacterSaver, *ecs.World, types.Handle) {
	b.Helper()
	world := ecs.NewWorldForTesting()
	handle := world.Spawn(10, nil)
	ecs.AddComponent(world, handle, components.Transform{})
	ecs.AddComponent(world, handle, components.EntityStats{Stamina: 100, Energy: 900})
	ecs.AddComponent(world, handle, components.EntityHealth{SHP: 21.4, HHP: 24.28})
	ecs.AddComponent(world, handle, components.CharacterProfile{Attributes: characterattrs.Default()})
	var inventories []InventorySnapshot
	if withInventories {
		inventories = []InventorySnapshot{
			characterSaveTestInventory(10, 0, 0, 1, "grid"),
			characterSaveTestInventory(10, 1, 0, 1, "hand"),
			characterSaveTestInventory(10, 2, 0, 1, "equipment"),
		}
	}
	saver := newCharacterSaver(0, characterSaveInventoryFunc(func(interface{}, types.EntityID, types.Handle) []InventorySnapshot {
		// The capture path stores this immutable fixture without mutating it.
		return inventories
	}), zap.NewNop(), func(context.Context, repository.UpdateCharactersParams, repository.UpsertInventoriesParams) error {
		return nil
	})
	b.Cleanup(saver.Stop)
	return saver, world, handle
}

func BenchmarkCharacterHealthSnapshot(b *testing.B) {
	saver, world, handle := characterBenchFixture(b, false)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		shp, hhp, lying, err := saver.resolveHealthSnapshotValues(world, handle)
		if err != nil {
			b.Fatal(err)
		}
		characterBenchSHP, characterBenchHHP, characterBenchLying = float64(shp), float64(hhp), lying
	}
}

func BenchmarkCharacterSaveCapture(b *testing.B) {
	for _, withInventories := range []bool{false, true} {
		b.Run(fmt.Sprintf("inventories_%t", withInventories), func(b *testing.B) {
			saver, world, handle := characterBenchFixture(b, withInventories)
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				if err := saver.Save(world, 10, handle); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkCharacterSaveParams(b *testing.B) {
	for _, size := range []int{1, 10, 100} {
		for _, withInventories := range []bool{false, true} {
			b.Run(fmt.Sprintf("batch_%d/inventories_%t", size, withInventories), func(b *testing.B) {
				batch := make([]*CharacterSnapshot, size)
				for index := range batch {
					snapshot := characterSaveTestSnapshot(int64(index+1), index)
					if withInventories {
						snapshot.Inventories = []InventorySnapshot{
							characterSaveTestInventory(int64(index+1), 0, 0, 1, "grid"),
							characterSaveTestInventory(int64(index+1), 1, 0, 1, "hand"),
							characterSaveTestInventory(int64(index+1), 2, 0, 1, "equipment"),
						}
					}
					batch[index] = &snapshot
				}
				b.ReportAllocs()
				b.ResetTimer()
				for iteration := 0; iteration < b.N; iteration++ {
					characterBenchParams, characterBenchInventories = characterSaveParams(batch)
				}
			})
		}
	}
}
