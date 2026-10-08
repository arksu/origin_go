package world

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

// BenchmarkObjectPersistence keeps definition data and inventories fixed across
// persistence changes. Build includes bounded teardown so the same warmed World
// and entity ID can be reused without measuring World initialization.
func BenchmarkObjectPersistence(b *testing.B) {
	previousObjects, previousItems := objectdefs.Global(), itemdefs.Global()
	b.Cleanup(func() {
		objectdefs.SetGlobalForTesting(previousObjects)
		itemdefs.SetGlobalForTesting(previousItems)
	})
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 1, Key: "player"},
		{DefID: 9801, Key: "benchmark_object", HP: 100, IsStatic: true, Resource: "benchmark_object"},
		{
			DefID: 9802, Key: "benchmark_container", HP: 100, IsStatic: true, Resource: "benchmark_container",
			Behaviors: map[string]json.RawMessage{"container": json.RawMessage(`{}`)},
			Components: &objectdefs.Components{Inventory: []objectdefs.InventoryDef{
				{Kind: "grid", Key: 0, W: 4, H: 4},
				{Kind: "grid", Key: 1, W: 4, H: 4},
			}},
		},
	}))
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 9901, Key: "benchmark_item", Resource: "benchmark_item", Size: itemdefs.Size{W: 1, H: 1}},
	}))

	for _, scenario := range []struct {
		name   string
		typeID int
		roots  int
	}{{"Simple", 9801, 0}, {"Container", 9802, 2}} {
		b.Run(scenario.name, func(b *testing.B) {
			raw := repository.Object{
				ID: 98001, TypeID: scenario.typeID, Region: 1, X: 10, Y: 20, Quality: 10,
				Hp: sql.NullFloat64{Float64: 100, Valid: true},
			}
			inventories := make([]repository.Inventory, 0, scenario.roots)
			for key := 0; key < scenario.roots; key++ {
				data := objectInventoryDataV1{
					Kind: uint8(constt.InventoryGrid), Key: uint32(key), Width: 4, Height: 4, Version: 1,
					Items: []objectInventoryItem{
						{ItemID: uint64(99001 + key*2), TypeID: 9901, Quality: 10, Quantity: 1},
						{ItemID: uint64(99002 + key*2), TypeID: 9901, Quality: 10, Quantity: 2, X: 1},
					},
				}
				payload, err := json.Marshal(data)
				if err != nil {
					b.Fatal(err)
				}
				inventories = append(inventories, repository.Inventory{
					OwnerID: raw.ID, Kind: int16(constt.InventoryGrid), InventoryKey: int16(key), Version: 1, Data: payload,
				})
			}
			b.Run("Build", func(b *testing.B) {
				w := ecs.NewWorldWithCapacity(64, nil, 0)
				factory := &ObjectFactory{}
				build := func() types.Handle {
					h, err := factory.Build(w, &raw, inventories)
					if err != nil {
						b.Fatal(err)
					}
					return h
				}
				cleanup := func(h types.Handle) {
					if owner, ok := ecs.GetComponent[components.InventoryOwner](w, h); ok {
						refs := ecs.GetResource[ecs.InventoryRefIndex](w)
						for _, link := range owner.Inventories {
							refs.Remove(link.Kind, link.OwnerID, link.Key)
							w.Despawn(link.Handle)
						}
					}
					w.Despawn(h)
				}
				cleanup(build())
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					cleanup(build())
				}
			})
			for _, operation := range []string{"Serialize", "Capture"} {
				b.Run(operation, func(b *testing.B) {
					w := ecs.NewWorldWithCapacity(64, nil, 0)
					factory := &ObjectFactory{}
					h, err := factory.Build(w, &raw, inventories)
					if err != nil {
						b.Fatal(err)
					}
					ecs.AddComponent(w, h, components.ChunkRef{})
					b.ReportAllocs()
					b.ResetTimer()
					if operation == "Serialize" {
						for i := 0; i < b.N; i++ {
							if _, err := factory.Serialize(w, h); err != nil {
								b.Fatal(err)
							}
						}
					} else {
						for i := 0; i < b.N; i++ {
							if _, err := factory.CaptureWorldObjectSnapshot(w, h); err != nil {
								b.Fatal(err)
							}
						}
					}
				})
			}
		})
	}
}

// BenchmarkObjectHP excludes World creation and the unrelated entities from the
// measured section. Equal work at both sizes detects accidental World scans.
func BenchmarkObjectHP(b *testing.B) {
	for _, noise := range []int{1_000, 100_000} {
		b.Run(fmt.Sprintf("Entities%d", noise), func(b *testing.B) {
			w := ecs.NewWorldWithCapacity(uint32(noise+8), nil, 0)
			for i := 0; i < noise; i++ {
				h := w.SpawnWithoutExternalID()
				ecs.AddComponent(w, h, components.Transform{X: float64(i), Y: 10})
			}
			target := w.SpawnWithoutExternalID()
			ecs.AddComponent(w, target, components.ObjectInternalState{HP: 100, HasHP: true})
			stale := w.SpawnWithoutExternalID()
			w.Despawn(stale)
			w.SpawnWithoutExternalID() // Reuse the index with a new generation.

			b.Run("Read", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					state, ok := ecs.GetComponent[components.ObjectInternalState](w, target)
					if !ok || !state.HasHP || state.HP != 100 {
						b.Fatal("health read failed")
					}
				}
			})
			b.Run("Validate", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if err := ValidateObjectHP(objectHPBenchmarkValidValues[i&1]); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("ValidateInvalid", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if err := ValidateObjectHP(objectHPBenchmarkInvalidValues[i&1]); err != ErrInvalidObjectHP {
						b.Fatal("invalid health was accepted")
					}
				}
			})
			b.Run("SetChanged", func(b *testing.B) {
				if err := SetObjectHP(w, target, 101); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if err := SetObjectHP(w, target, float64(100+(i&1))); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("SetNoop", func(b *testing.B) {
				if err := SetObjectHP(w, target, 100); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if err := SetObjectHP(w, target, 100); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("SetInvalid", func(b *testing.B) {
				invalid := math.Inf(1)
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if err := SetObjectHP(w, target, invalid); err != ErrInvalidObjectHP {
						b.Fatal("invalid health was accepted")
					}
				}
			})
			b.Run("SetStale", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if err := SetObjectHP(w, stale, 100); err != ErrEntityNotFound {
						b.Fatal("stale handle was accepted")
					}
				}
			})
		})
	}
}

// Mutable inputs prevent the compiler from replacing numerical validation with
// the expected constant result. The benchmark never changes their values.
var (
	objectHPBenchmarkValidValues   = [2]float64{0, 100}
	objectHPBenchmarkInvalidValues = [2]float64{math.NaN(), math.Inf(1)}
)
