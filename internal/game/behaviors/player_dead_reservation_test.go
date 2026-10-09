package behaviors

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestReservedDeadRecipientCannotBeUnequippedByAnotherPlayer(t *testing.T) {
	setupPlayerDeathItemRegistry(t)
	w, playerID, player, corpseID, corpse, equipment := setupPlayerDeadBehaviorWorld(t, []components.InvItem{{
		ItemID: 9101, TypeID: 9001, Quality: 77, Quantity: 1, W: 1, H: 1, EquipSlot: netproto.EquipSlot_EQUIP_SLOT_HEAD,
	}})
	require.True(t, ecs.ReserveInventoryOwner(w, corpseID, corpse))
	before, _ := ecs.GetComponent[components.InventoryContainer](w, equipment)
	behavior := playerDeadBehavior{}
	list := &contracts.BehaviorActionListContext{World: w, PlayerID: playerID, PlayerHandle: player, TargetID: corpseID, TargetHandle: corpse}
	require.Empty(t, behavior.ProvideActions(list))
	require.False(t, behavior.ValidateAction(&contracts.BehaviorActionValidateContext{World: w, TargetID: corpseID, TargetHandle: corpse, ActionID: actionUnequip}).OK)
	calls := 0
	result := behavior.ExecuteAction(&contracts.BehaviorActionExecuteContext{
		World: w, PlayerID: playerID, PlayerHandle: player, TargetID: corpseID, TargetHandle: corpse, ActionID: actionUnequip,
		Deps: &contracts.ExecutionDeps{GiveItem: func(*ecs.World, types.EntityID, types.Handle, string, uint32, uint32) contracts.GiveItemOutcome {
			calls++
			return contracts.GiveItemOutcome{Success: true}
		}},
	})
	require.False(t, result.OK)
	require.Zero(t, calls)
	after, _ := ecs.GetComponent[components.InventoryContainer](w, equipment)
	require.Equal(t, before.Version, after.Version)
	require.Equal(t, before.Items, after.Items)
	state, _ := ecs.GetComponent[components.ObjectInternalState](w, corpse)
	require.False(t, state.IsDirty)
	require.True(t, ecs.ReleaseInventoryOwner(w, corpseID, corpse))
	require.Equal(t, []contracts.ContextAction{{ActionID: actionUnequip, Title: "Unequip"}}, behavior.ProvideActions(list))
}
