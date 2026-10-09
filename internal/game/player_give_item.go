package game

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/game/inventory"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

type playerGiveItemSender interface {
	SendInventoryOpResult(types.EntityID, *netproto.S2C_InventoryOpResult)
	SendExpGained(types.EntityID, *netproto.S2C_ExpGained)
	SendFx(types.EntityID, *netproto.S2C_Fx)
}

func newPlayerGiveItemAdapter(executor *inventory.InventoryExecutor, sender playerGiveItemSender) contracts.GiveItemFn {
	return func(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, itemKey string, count, quality uint32) contracts.GiveItemOutcome {
		if executor == nil {
			return contracts.GiveItemOutcome{Message: "inventory executor unavailable"}
		}
		result := executor.GiveItem(world, playerID, playerHandle, itemKey, count, quality)
		if result == nil {
			return contracts.GiveItemOutcome{Message: "nil give result"}
		}
		publishPlayerGiveItemResult(world, playerID, playerHandle, result, executor, sender)
		return contracts.GiveItemOutcome{
			Success: result.Success, AnyDropped: false, PlacedInHand: result.PlacedInHand,
			GrantedCount: result.GrantedCount, Message: result.Message,
		}
	}
}

func (s *Shard) skullGranted(playerID types.EntityID, playerHandle types.Handle, result *inventory.GiveItemResult) {
	publishPlayerGiveItemResult(s.world, playerID, playerHandle, result, s.inventoryExecutor, s)
}

func publishPlayerGiveItemResult(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, result *inventory.GiveItemResult, executor *inventory.InventoryExecutor, sender playerGiveItemSender) {
	if result.Success && len(result.UpdatedContainers) > 0 && sender != nil {
		states := executor.ConvertContainersToStates(world, result.UpdatedContainers)
		updated := make([]*netproto.InventoryState, 0, len(states))
		for _, state := range states {
			updated = append(updated, systems.BuildInventoryStateProto(state))
		}
		if len(updated) > 0 {
			sender.SendInventoryOpResult(playerID, &netproto.S2C_InventoryOpResult{
				OpId:    0,
				Success: true,
				Updated: updated,
			})
		}
	}
	if result.Success && result.DiscoveryLPGained > 0 && sender != nil {
		lp := result.DiscoveryLPGained
		sender.SendExpGained(playerID, &netproto.S2C_ExpGained{EntityId: uint64(playerID), Lp: &lp})

		var posX, posY float64
		ecs.WithComponent(world, playerHandle, func(transform *components.Transform) {
			posX, posY = transform.X, transform.Y
		})
		sender.SendFx(playerID, &netproto.S2C_Fx{
			FxKey:    "exp_gain",
			Position: &netproto.Vector2{X: int32(posX), Y: int32(posY)},
		})
	}
}
