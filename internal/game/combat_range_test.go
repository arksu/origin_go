package game

import (
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"math"
	"origin/internal/combat"
	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
	"path/filepath"
	"strings"
	"testing"
)

func newCombatRangeFixture(t *testing.T) (*combatFixture, *CombatRangeService) {
	t.Helper()
	fixture := newCombatFixture(t, 500)
	fixture.world.Despawn(fixture.target)
	chunks := combatTestChunks{}
	chunk := core.NewChunk(types.ChunkCoord{}, 0, 0, constt.ChunkSize)
	chunk.SetState(types.ChunkStateActive)
	chunks[types.ChunkCoord{}] = chunk
	fixture.service.receivers = NewCombatReceivers(fixture.world, chunks)
	service, err := NewCombatRangeService(fixture.world, fixture.service, chunks, &digTestIDAllocator{next: 1000}, filepath.Join("..", "..", "data", "combat_range.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	fixture.service.Strength = service.Strength
	return fixture, service
}
func TestCombatRangePresetsUseSharedHitPath(t *testing.T) {
	for _, preset := range []string{"small", "large", "tied", "moving", "blocker"} {
		t.Run(preset, func(t *testing.T) {
			fixture, service := newCombatRangeFixture(t)
			if err := service.Create(fixture.actor, preset); err != nil {
				t.Fatal(err)
			}
			fixture.arm(t, "axe_aoe")
			fixture.commit(t)
			fixture.at(600)
			service.Update(fixture.world, .1)
			fixture.service.Update(fixture.world, .1)
			want := len(service.ranges[fixture.actor].fixtures)
			if preset == "moving" {
				want = 0
			}
			if preset == "blocker" {
				want = 1
			}
			if len(fixture.results) != 1 || len(fixture.results[0].Hits) != want {
				t.Fatalf("%s: %#v", preset, fixture.results)
			}
			for _, hit := range fixture.results[0].Hits {
				if hit.Damage != 6 {
					t.Fatal("range bypassed common damage")
				}
			}
		})
	}
}
func TestCombatRangeFractionalResetRemovalAndPersistence(t *testing.T) {
	fixture, service := newCombatRangeFixture(t)
	if err := service.Create(fixture.actor, "small"); err != nil {
		t.Fatal(err)
	}
	layout := service.ranges[fixture.actor]
	target := layout.fixtures[0].handle
	id, _ := fixture.world.GetExternalID(target)
	ecs.WithComponent(fixture.world, target, func(target *components.CombatTestTarget) { target.HP = 1 })
	hit := CombatHit{EventSequence: 1, ExecutionID: 1, AttackerID: 1, AttackerIncarnation: fixture.actor, TargetID: id, TargetIncarnation: target, RawDamage: .4}
	if _, applied, err := fixture.service.receivers.Apply(hit); err != nil || !applied {
		t.Fatal(err)
	}
	state, _ := ecs.GetComponent[components.CombatTestTarget](fixture.world, target)
	if math.Abs(state.HP-.6) > 1e-12 {
		t.Fatal(state)
	}
	factory := &gameworld.ObjectFactory{}
	persisted, err := factory.Serialize(fixture.world, target)
	if err != nil || persisted != nil {
		t.Fatal("fixture must not persist")
	}
	hit.EventSequence++
	hit.RawDamage = 10
	fixture.service.receivers.Apply(hit)
	contacts, err := fixture.service.receivers.Contacts(1, combat.Sector{Origin: combat.Point{X: 100, Y: 100}, Direction: combat.Point{X: 1}, Range: 18, AngleDegrees: 90}, false)
	if err != nil || len(contacts) != 2 {
		t.Fatalf("depleted contact: %v %v", contacts, err)
	}
	if err := service.Reset(fixture.actor); err != nil {
		t.Fatal(err)
	}
	state, _ = ecs.GetComponent[components.CombatTestTarget](fixture.world, target)
	if state.HP != 100 || state.Revision != 4 {
		t.Fatal(state)
	}
	service.Remove(fixture.actor)
	if fixture.world.Alive(target) || len(layout.fixtures[0].chunk.Spatial().GetAllHandles()) != 0 {
		t.Fatal("range cleanup incomplete")
	}
	if err := service.Create(fixture.actor, "small"); err != nil {
		t.Fatal(err)
	}
	hit.EventSequence++
	if _, applied, err := fixture.service.receivers.Apply(hit); err != nil || applied {
		t.Fatal("old incarnation hit replacement")
	}
}
func TestCombatRangePublicCommandsAndBounds(t *testing.T) {
	fixture, service := newCombatRangeFixture(t)
	chat := &mockChatDeliveryService{messages: map[types.EntityID]string{}}
	handler := &ChatAdminCommandHandler{combatRange: service, chatDelivery: chat, logger: zap.NewNop()}
	fixture.service.enabled = false
	handler.HandleCommand(fixture.world, 1, fixture.actor, "/combat-range create")
	if len(service.ranges) != 0 || !strings.Contains(chat.messages[1], "disabled") {
		t.Fatal("disabled command had effects")
	}
	fixture.service.enabled = true
	handler.HandleCommand(fixture.world, 1, fixture.actor, "/combat-range create small")
	if len(service.ranges) != 1 {
		t.Fatal(chat.messages[1])
	}
	handler.HandleCommand(fixture.world, 1, fixture.actor, "/combat-range create small")
	if len(service.ranges) != 1 || !strings.Contains(chat.messages[1], "existing") {
		t.Fatal("duplicate range")
	}
	for _, value := range []string{"59", "60"} {
		handler.HandleCommand(fixture.world, 1, fixture.actor, "/combat-range stamina "+value)
		stats, _ := ecs.GetComponent[components.EntityStats](fixture.world, fixture.actor)
		if int(stats.Stamina) != map[string]int{"59": 59, "60": 60}[value] {
			t.Fatal(stats)
		}
	}
	handler.HandleCommand(fixture.world, 1, fixture.actor, "/combat-range strength 1.5")
	strength, err := service.Strength(fixture.world, fixture.actor)
	if err != nil || strength != 1.5 {
		t.Fatal(strength, err)
	}
	handler.HandleCommand(fixture.world, 1, fixture.actor, "/combat-range strength NaN")
	if !strings.Contains(chat.messages[1], "finite") {
		t.Fatal("invalid setup accepted")
	}
	service.Remove(fixture.actor)
	chunk := service.chunks.GetChunk(types.ChunkCoord{})
	blocker := fixture.world.Spawn(8000, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 110, Y: 100})
		ecs.AddComponent(w, h, components.Collider{HalfWidth: 1, HalfHeight: 1})
	})
	chunk.Spatial().AddStatic(blocker, 110, 100)
	if err := service.Create(fixture.actor, "small"); err == nil {
		t.Fatal("overlap accepted")
	}
	if !fixture.world.Alive(blocker) {
		t.Fatal("unrelated object removed")
	}
	chunk.Spatial().RemoveStatic(blocker, 110, 100)
	for index := 0; index < combatRangeLimit; index++ {
		service.ranges[types.Handle(9000+index)] = &combatRange{}
	}
	if err := service.Create(fixture.actor, "small"); err == nil {
		t.Fatal("range limit ignored")
	}
}

func TestCombatRangeStarterAxeUsesNormalGrantAndEquip(t *testing.T) {
	fixture, service := newCombatRangeFixture(t)
	root, _ := installDigInventory(t, fixture.world, fixture.actor)
	ecs.WithComponent(fixture.world, root, func(container *components.InventoryContainer) { container.Width = 8; container.Height = 8 })
	equipment := fixture.world.GetHandleByEntityID(10)
	ecs.WithComponent(fixture.world, equipment, func(container *components.InventoryContainer) { container.Items = nil; container.Version = 1 })
	ecs.WithComponent(fixture.world, fixture.actor, func(owner *components.InventoryOwner) {
		owner.Inventories = append(owner.Inventories, components.InventoryLink{Kind: constt.InventoryEquipment, OwnerID: 1, Handle: equipment})
	})
	sender := &combatRangeInventorySender{}
	ids := &digTestIDAllocator{next: 2000}
	handler := &ChatAdminCommandHandler{combatRange: service, inventoryExecutor: inventory.NewInventoryExecutor(zap.NewNop(), ids, nil, nil, nil), inventoryResultSender: sender, chatDelivery: &mockChatDeliveryService{messages: map[types.EntityID]string{}}, logger: zap.NewNop()}
	handler.HandleCommand(fixture.world, 1, fixture.actor, "/combat-range create small")
	container, _ := ecs.GetComponent[components.InventoryContainer](fixture.world, root)
	require.Len(t, container.Items, 1)
	axe := container.Items[0]
	require.Equal(t, uint32(10), axe.Quality)
	require.Equal(t, uint32(1002), axe.TypeID)
	require.NotEmpty(t, sender.updates, "grant must synchronize the existing inventory UI")
	handler.HandleCommand(fixture.world, 1, fixture.actor, "/combat-range axe")
	container, _ = ecs.GetComponent[components.InventoryContainer](fixture.world, root)
	require.Len(t, container.Items, 1, "range setup reuses an existing starter axe")
	operations := inventory.NewInventoryOperationService(zap.NewNop(), ids, nil)
	slot := netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND
	result := operations.ExecuteMove(fixture.world, 1, fixture.actor, 1, &netproto.InventoryMoveSpec{
		Src:    &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: 1},
		Dst:    &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_EQUIPMENT, OwnerId: 1},
		ItemId: uint64(axe.ItemID), DstEquipSlot: &slot,
	}, nil)
	require.True(t, result.Success, result.Message)
	definition, _ := fixture.actions.definitions.Get("axe_aoe")
	weapon, err := resolveCombatWeapon(fixture.world, 1, definition)
	require.NoError(t, err)
	require.Equal(t, float64(10), weapon.Quality)
	require.Equal(t, axe.ItemID, weapon.ItemID)
	require.False(t, ecs.HasComponent[components.EntityHealth](fixture.world, fixture.actor), "setup must not create or change real health")
}

type combatRangeInventorySender struct {
	testPlayerGiveItemSender
	updates []*netproto.InventoryState
}

func (*combatRangeInventorySender) SendContainerOpened(types.EntityID, *netproto.InventoryState) {}
func (*combatRangeInventorySender) SendContainerClosed(types.EntityID, *netproto.InventoryRef)   {}
func (sender *combatRangeInventorySender) SendInventoryUpdate(_ types.EntityID, states []*netproto.InventoryState) {
	sender.updates = append(sender.updates, states...)
}
