package inventory

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/behaviors/contracts"
	netproto "origin/internal/network/proto"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type progressTestRegistry struct {
	contracts.BehaviorRegistry
	byKey map[string]contracts.Behavior
}

func (r progressTestRegistry) GetBehavior(key string) (contracts.Behavior, bool) {
	behavior, found := r.byKey[key]
	return behavior, found
}

type progressTestBehavior struct {
	key string
	get func(*contracts.InventoryItemProgressContext) *netproto.ItemProgress
}

func (b progressTestBehavior) Key() string { return b.key }
func (b progressTestBehavior) InventoryItemProgress(ctx *contracts.InventoryItemProgressContext) *netproto.ItemProgress {
	return b.get(ctx)
}

func TestItemProgressProjectionUsesFirstValidProviderInObjectOrder(t *testing.T) {
	f := setupDryingInventory(t, true)
	item := dryingTestItem(3101, 101)
	container, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.root)
	container.Items = []components.InvItem{item}
	want := &netproto.ItemProgress{DurationSeconds: 120, Timing: &netproto.ItemProgress_PausedRemainingSeconds{PausedRemainingSeconds: 0}}
	calls := make([]string, 0, 4)
	registry := progressTestRegistry{byKey: make(map[string]contracts.Behavior)}
	for _, entry := range []struct {
		key      string
		progress *netproto.ItemProgress
	}{
		{"no progress", nil},
		{"invalid", &netproto.ItemProgress{DurationSeconds: 120}},
		{"winner", want},
		{"later", &netproto.ItemProgress{DurationSeconds: 90, Timing: &netproto.ItemProgress_CompletionRuntimeSeconds{CompletionRuntimeSeconds: 400}}},
	} {
		registry.byKey[entry.key] = progressTestBehavior{entry.key, func(ctx *contracts.InventoryItemProgressContext) *netproto.ItemProgress {
			calls = append(calls, entry.key)
			require.Same(t, f.w, ctx.World)
			require.Equal(t, f.frame, ctx.Handle)
			require.Equal(t, f.frameID, ctx.EntityID)
			require.Equal(t, uint32(20), ctx.EntityType)
			require.Equal(t, item, ctx.Item)
			require.Equal(t, container, ctx.Container)
			return entry.progress
		}}
	}
	ecs.WithComponent(f.w, f.frame, func(info *components.EntityInfo) {
		info.Behaviors = []string{"missing", "no progress", "invalid", "winner", "later"}
	})
	require.Same(t, want, ResolveItemProgress(f.w, registry, container, item))
	require.Equal(t, []string{"no progress", "invalid", "winner"}, calls)
	require.Nil(t, ResolveItemProgress(f.w, nil, container, item))
	container.OwnerID = item.ItemID // A nested inventory has no object behavior owner.
	require.Nil(t, ResolveItemProgress(f.w, registry, container, item))
	require.Len(t, calls, 3)
}

func TestItemProgressRejectsInvalidTiming(t *testing.T) {
	for _, progress := range []*netproto.ItemProgress{
		nil,
		{DurationSeconds: 0, Timing: &netproto.ItemProgress_CompletionRuntimeSeconds{CompletionRuntimeSeconds: 10}},
		{DurationSeconds: -1, Timing: &netproto.ItemProgress_CompletionRuntimeSeconds{CompletionRuntimeSeconds: 10}},
		{DurationSeconds: 10},
		{DurationSeconds: 10, Timing: (*netproto.ItemProgress_CompletionRuntimeSeconds)(nil)},
		{DurationSeconds: 10, Timing: &netproto.ItemProgress_CompletionRuntimeSeconds{CompletionRuntimeSeconds: -1}},
		{DurationSeconds: 10, Timing: &netproto.ItemProgress_PausedRemainingSeconds{PausedRemainingSeconds: -1}},
		{DurationSeconds: 10, Timing: &netproto.ItemProgress_PausedRemainingSeconds{PausedRemainingSeconds: 11}},
	} {
		require.False(t, validItemProgress(progress), "%+v", progress)
	}
}

func TestItemProgressSnapshotAndOperationSerialization(t *testing.T) {
	f := setupDryingInventory(t, true)
	ecs.WithComponent(f.w, f.frame, func(info *components.EntityInfo) { info.Behaviors = []string{"progress"} })
	for _, progress := range []*netproto.ItemProgress{
		nil,
		{DurationSeconds: 120, Timing: &netproto.ItemProgress_CompletionRuntimeSeconds{CompletionRuntimeSeconds: 420}},
		{DurationSeconds: 120, Timing: &netproto.ItemProgress_PausedRemainingSeconds{PausedRemainingSeconds: 30}},
	} {
		registry := progressTestRegistry{byKey: map[string]contracts.Behavior{
			"progress": progressTestBehavior{"progress", func(*contracts.InventoryItemProgressContext) *netproto.ItemProgress { return progress }},
		}}
		sender := NewSnapshotSender(zap.NewNop(), registry)
		executor := NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
		executor.SetBehaviorRegistry(registry)
		for _, kind := range []constt.InventoryKind{constt.InventoryGrid, constt.InventoryHand, constt.InventoryEquipment} {
			container := components.InventoryContainer{OwnerID: f.frameID, Kind: kind, Width: 2, Height: 2, Version: 7, Items: []components.InvItem{dryingTestItem(3101, 101)}}
			fromLogin := sender.buildInventoryState(f.w, container)
			fromOperation := systems.BuildInventoryStateProto(executor.convertContainerToState(f.w, &ContainerInfo{Container: &container}))
			var loginItem, operationItem *netproto.ItemInstance
			switch kind {
			case constt.InventoryGrid:
				loginItem, operationItem = fromLogin.GetGrid().Items[0].Item, fromOperation.GetGrid().Items[0].Item
			case constt.InventoryHand:
				loginItem, operationItem = fromLogin.GetHand().Item, fromOperation.GetHand().Item
			case constt.InventoryEquipment:
				loginItem, operationItem = fromLogin.GetEquipment().Items[0].Item, fromOperation.GetEquipment().Items[0].Item
			}
			require.True(t, proto.Equal(progress, loginItem.Progress))
			require.True(t, proto.Equal(progress, operationItem.Progress))
			if progress == nil {
				require.Nil(t, loginItem.Progress)
				require.Nil(t, operationItem.Progress)
			}
		}
	}
}
