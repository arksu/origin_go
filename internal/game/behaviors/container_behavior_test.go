package behaviors

import (
	"reflect"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/types"
)

func TestContainerBehaviorApplyRuntimeFlags(t *testing.T) {
	testCases := []struct {
		name        string
		items       []components.InvItem
		openPlayers int
		wantFlags   []string
	}{
		{
			name:      "closed and empty",
			wantFlags: nil,
		},
		{
			name: "closed and has items",
			items: []components.InvItem{
				{ItemID: 7001, TypeID: 1, Quantity: 1, W: 1, H: 1},
			},
			wantFlags: []string{"container.has_items"},
		},
		{
			name:        "open and empty",
			openPlayers: 1,
			wantFlags:   []string{"container.open"},
		},
		{
			name: "open and has items",
			items: []components.InvItem{
				{ItemID: 7001, TypeID: 1, Quantity: 1, W: 1, H: 1},
			},
			openPlayers: 2,
			wantFlags:   []string{"container.has_items", "container.open"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			world := ecs.NewWorldForTesting()
			entityID := types.EntityID(5001)
			spawnContainerRoot(world, entityID, testCase.items)

			if testCase.openPlayers > 0 {
				players := make(map[types.EntityID]struct{}, testCase.openPlayers)
				for i := 0; i < testCase.openPlayers; i++ {
					players[types.EntityID(1000+i)] = struct{}{}
				}
				ecs.GetResource[ecs.OpenContainerState](world).PlayersByRoot[entityID] = players
			}

			result := containerBehavior{}.ApplyRuntime(&contracts.BehaviorRuntimeContext{
				World:    world,
				EntityID: entityID,
			})

			if !reflect.DeepEqual(result.Flags, testCase.wantFlags) {
				t.Fatalf("unexpected flags: got %#v want %#v", result.Flags, testCase.wantFlags)
			}
		})
	}
}

func spawnContainerRoot(world *ecs.World, entityID types.EntityID, items []components.InvItem) {
	containerHandle := world.SpawnWithoutExternalID()
	ecs.AddComponent(world, containerHandle, components.InventoryContainer{
		OwnerID: entityID,
		Kind:    constt.InventoryGrid,
		Key:     0,
		Width:   4,
		Height:  4,
		Items:   items,
	})
	ecs.GetResource[ecs.InventoryRefIndex](world).Add(constt.InventoryGrid, entityID, 0, containerHandle)
}
