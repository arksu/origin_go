package game

import (
	"go.uber.org/zap"
	"origin/internal/actiondefs"
	"origin/internal/characterattrs"
	"origin/internal/combat"
	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/itemdefs"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
	"path/filepath"
	"testing"
	"time"
)

type combatFixture struct {
	world         *ecs.World
	actor, target types.Handle
	actions       *ActionService
	service       *CombatService
	receiver      *testCombatReceiver
	sender        *testActionSender
	results       []CombatResult
	commands      *systems.NetworkCommandSystem
	inbox         *network.PlayerCommandInbox
	commandID     uint64
	revision      uint64
}

func newCombatFixture(t *testing.T, stamina float64) *combatFixture {
	t.Helper()
	previous := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	items, err := itemdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "items"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	itemdefs.SetGlobalForTesting(items)
	definitions, err := actiondefs.LoadFromDirectory(filepath.Join("..", "..", "data", "actions"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	selected := make([]actiondefs.Definition, 0, 2)
	for _, id := range []string{"axe_aoe", "axe_single"} {
		definition, _ := definitions.Get(id)
		selected = append(selected, *definition)
	}
	world := ecs.NewWorldForTesting()
	fixture := &combatFixture{world: world, sender: &testActionSender{}, receiver: &testCombatReceiver{hp: 100}}
	fixture.actor = world.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 100, Y: 100})
		ecs.AddComponent(w, h, components.Movement{Mode: constt.Run, Speed: 32})
		ecs.AddComponent(w, h, components.EntityStats{Stamina: stamina, Energy: 1000})
		ecs.AddComponent(w, h, components.CharacterProfile{Attributes: characterattrs.Default()})
	})
	equipment := world.Spawn(10, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.InventoryContainer{OwnerID: 1, Kind: constt.InventoryEquipment, Items: []components.InvItem{{ItemID: 100, TypeID: 1002, Quality: 10, EquipSlot: netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND}}})
	})
	ecs.GetResource[ecs.InventoryRefIndex](world).Add(constt.InventoryEquipment, 1, 0, equipment)
	fixture.target = world.Spawn(2, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 110, Y: 100})
		ecs.AddComponent(w, h, components.Collider{HalfWidth: 1, HalfHeight: 1})
	})
	chunk := core.NewChunk(types.ChunkCoord{}, 0, 0, constt.ChunkSize)
	chunk.Spatial().AddStatic(fixture.target, 110, 100)
	receivers := NewCombatReceivers(world, combatTestChunks{types.ChunkCoord{}: chunk})
	if err := receivers.Register(fixture.target, fixture.receiver); err != nil {
		t.Fatal(err)
	}
	fixture.service = NewCombatService(world, receivers, true, zap.NewNop())
	handlers := map[string]ActionHandler{}
	for index := range selected {
		definition := &selected[index]
		handlers[definition.ID] = &combatActionHandler{combat: fixture.service, definition: definition}
	}
	fixture.actions, err = NewActionService(world, actiondefs.NewRegistry(selected), handlers, fixture.sender)
	if err != nil {
		t.Fatal(err)
	}
	fixture.actions.SetCombatService(fixture.service)
	fixture.service.OnResult = func(result CombatResult) { fixture.results = append(fixture.results, result) }
	fixture.commands, fixture.inbox = directionalTestCommands(world, fixture.actions)
	fixture.at(0)
	return fixture
}

func (fixture *combatFixture) at(milliseconds int64) {
	timing := ecs.GetResource[ecs.TimeState](fixture.world)
	timing.Now = time.Unix(100, 0).Add(time.Duration(milliseconds) * time.Millisecond)
	timing.UnixMs = milliseconds
}
func (fixture *combatFixture) send(t *testing.T, kind network.CommandType, payload any, client uint64, layer int) {
	t.Helper()
	fixture.commandID++
	if err := fixture.inbox.Enqueue(&network.PlayerCommand{ClientID: client, CharacterID: 1, CommandID: fixture.commandID, CommandType: kind, Payload: payload, Layer: layer}); err != nil {
		t.Fatal(err)
	}
	fixture.commands.Update(fixture.world, 0.1)
}
func (fixture *combatFixture) arm(t *testing.T, id string) {
	fixture.revision++
	fixture.send(t, network.CmdActivateAction, &netproto.C2S_ActivateAction{ActionId: id, StreamEpoch: 1, RequestRevision: fixture.revision}, 1, 0)
}
func (fixture *combatFixture) commit(t *testing.T) *netproto.MapClick {
	fixture.revision++
	state := fixture.actions.State(fixture.world, fixture.actor)
	click := &netproto.MapClick{X: 118, Y: 100, CombatAttempt: &netproto.CombatAttempt{ActionId: state.ActionId, SelectionGeneration: state.SelectionGeneration, StreamEpoch: 1, RequestRevision: fixture.revision}}
	fixture.send(t, network.CmdMapClick, click, 1, 0)
	return click
}

func TestCombatLifecyclePaymentDeadlinesAndReplay(t *testing.T) {
	fixture := newCombatFixture(t, 60)
	fixture.arm(t, "axe_aoe")
	stats, _ := ecs.GetComponent[components.EntityStats](fixture.world, fixture.actor)
	if stats.Stamina != 60 || fixture.actions.State(fixture.world, fixture.actor).Phase != "selecting" {
		t.Fatal("selection paid or did not arm")
	}
	click := fixture.commit(t)
	stats, _ = ecs.GetComponent[components.EntityStats](fixture.world, fixture.actor)
	state, _ := ecs.GetComponent[components.CombatState](fixture.world, fixture.actor)
	if stats.Stamina != 0 || state.Execution == nil || state.LastCombatEventAt != 0 || state.Execution.StartEventSequence == 0 {
		t.Fatal("exact payment/start failed")
	}
	fixture.actions.Cancel(fixture.world, 1, fixture.actor)
	fixture.actions.Recheck(fixture.world, 1, fixture.actor)
	fixture.send(t, network.CmdMapClick, click, 1, 0)
	fixture.at(599)
	fixture.service.Update(fixture.world, .017)
	if fixture.receiver.hits != 0 {
		t.Fatal("early strike")
	}
	fixture.at(600)
	fixture.service.Update(fixture.world, .017)
	fixture.service.Update(fixture.world, .017)
	if fixture.receiver.hp != 94 || fixture.receiver.hits != 1 || fixture.actions.State(fixture.world, fixture.actor).Phase != "recovery" {
		t.Fatal("strike resolution not exactly once")
	}
	fixture.at(1000)
	fixture.service.Update(fixture.world, .1)
	if components.CombatCommitted(fixture.world, fixture.actor) {
		t.Fatal("recovery did not end")
	}
	state, _ = ecs.GetComponent[components.CombatState](fixture.world, fixture.actor)
	if state.LastCombatEventAt != 600 || state.Cooldowns["axe_aoe"] != time.Unix(102, 0) {
		t.Fatal("activity/cooldown wrong")
	}
	fixture.at(3000)
	ecs.WithComponent(fixture.world, fixture.actor, func(stats *components.EntityStats) { stats.Stamina = 120 })
	fixture.send(t, network.CmdMapClick, click, 1, 0)
	fixture.service.Update(fixture.world, .1)
	stats, _ = ecs.GetComponent[components.EntityStats](fixture.world, fixture.actor)
	movement, _ := ecs.GetComponent[components.Movement](fixture.world, fixture.actor)
	if stats.Stamina != 120 || fixture.receiver.hits != 1 || movement.TargetType != constt.TargetNone {
		t.Fatal("replay became an attack or ordinary click")
	}
}

func TestCombatRejectionAndIndependentCooldowns(t *testing.T) {
	fixture := newCombatFixture(t, 59)
	fixture.arm(t, "axe_aoe")
	rejected := fixture.commit(t)
	state, _ := ecs.GetComponent[components.CombatState](fixture.world, fixture.actor)
	if state.Execution != nil || len(state.Cooldowns) != 0 || state.LastCombatEventAt != 0 {
		t.Fatal("rejection changed combat")
	}
	ecs.WithComponent(fixture.world, fixture.actor, func(stats *components.EntityStats) { stats.Stamina = 300 })
	fixture.send(t, network.CmdMapClick, rejected, 1, 0)
	if components.CombatCommitted(fixture.world, fixture.actor) {
		t.Fatal("rejected request replayed")
	}
	fixture.commit(t)
	fixture.at(1000)
	fixture.service.Update(fixture.world, .1)
	fixture.arm(t, "axe_aoe")
	fixture.commit(t)
	if components.CombatCommitted(fixture.world, fixture.actor) {
		t.Fatal("same action ignored cooldown")
	}
	fixture.arm(t, "axe_single")
	fixture.commit(t)
	if !components.CombatCommitted(fixture.world, fixture.actor) {
		t.Fatal("independent action cooldown blocked")
	}
	fixture.at(2000)
	fixture.service.Update(fixture.world, .05)
	if fixture.receiver.hp != 85 || len(fixture.results) != 2 {
		t.Fatalf("crossed deadlines did not resolve once: hp=%v results=%v", fixture.receiver.hp, len(fixture.results))
	}
	fixture.arm(t, "axe_aoe")
	fixture.commit(t)
	if !components.CombatCommitted(fixture.world, fixture.actor) {
		t.Fatal("cooldown not ready at exact deadline")
	}
}

func TestCombatCurrentPositionsStrengthAndInterruption(t *testing.T) {
	for _, scenario := range []string{"escape", "move actor", "strength", "interrupt windup", "interrupt recovery"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newCombatFixture(t, 120)
			fixture.arm(t, "axe_aoe")
			fixture.commit(t)
			switch scenario {
			case "escape":
				ecs.WithComponent(fixture.world, fixture.target, func(transform *components.Transform) { transform.X = 140 })
			case "move actor":
				ecs.WithComponent(fixture.world, fixture.actor, func(transform *components.Transform) { transform.X = 80; transform.Direction = 180 })
			case "strength":
				fixture.service.Strength = func(*ecs.World, types.Handle) (float64, error) { return 16, nil }
			case "interrupt windup":
				fixture.service.Interrupt(fixture.actor)
			}
			fixture.at(600)
			fixture.service.Update(fixture.world, .1)
			if scenario == "interrupt recovery" {
				fixture.service.Interrupt(fixture.actor)
			}
			fixture.at(1500)
			fixture.service.Update(fixture.world, .1)
			expected := 100.0
			if scenario == "strength" {
				expected = 88
			}
			if scenario == "interrupt recovery" {
				expected = 94
			}
			if fixture.receiver.hp != expected {
				t.Fatalf("HP %v != %v", fixture.receiver.hp, expected)
			}
			state, _ := ecs.GetComponent[components.CombatState](fixture.world, fixture.actor)
			stats, _ := ecs.GetComponent[components.EntityStats](fixture.world, fixture.actor)
			if stats.Stamina != 60 || state.Execution != nil || len(state.Cooldowns) != 1 {
				t.Fatal("interrupt/refund/recovery changed")
			}
			expectedActivity := int64(600)
			if scenario == "interrupt windup" {
				expectedActivity = 0
			}
			if state.LastCombatEventAt != expectedActivity {
				t.Fatalf("activity=%v", state.LastCombatEventAt)
			}
		})
	}
}

func TestCombatInputSessionEpochAndSelection(t *testing.T) {
	fixture := newCombatFixture(t, 180)
	for _, request := range []struct {
		client   uint64
		epoch    uint32
		revision uint64
		layer    int
	}{
		{2, 1, 1, 0}, {1, 2, 1, 0}, {1, 1, 0, 0}, {1, 1, 1, 1},
	} {
		fixture.send(t, network.CmdActivateAction, &netproto.C2S_ActivateAction{ActionId: "axe_aoe", StreamEpoch: request.epoch, RequestRevision: request.revision}, request.client, request.layer)
		if fixture.actions.State(fixture.world, fixture.actor).Phase != "idle" {
			t.Fatal("bad session armed action")
		}
	}
	fixture.send(t, network.CmdActivateAction, &netproto.C2S_ActivateAction{ActionId: "axe_aoe"}, 1, 0)
	if fixture.actions.State(fixture.world, fixture.actor).Phase != "idle" {
		t.Fatal("untagged activation accepted")
	}
	fixture.arm(t, "axe_aoe")
	active := fixture.actions.State(fixture.world, fixture.actor)
	click := &netproto.MapClick{X: 118, Y: 100, CombatAttempt: &netproto.CombatAttempt{ActionId: "axe_aoe", SelectionGeneration: active.SelectionGeneration + 1, StreamEpoch: 1, RequestRevision: 2}}
	fixture.send(t, network.CmdMapClick, click, 1, 0)
	if components.CombatCommitted(fixture.world, fixture.actor) {
		t.Fatal("stale selection committed")
	}
	click.CombatAttempt.SelectionGeneration = active.SelectionGeneration
	fixture.send(t, network.CmdMapClick, click, 1, 0)
	if components.CombatCommitted(fixture.world, fixture.actor) {
		t.Fatal("consumed invalid attempt reused")
	}
	fixture.revision = 2
	fixture.commit(t)
	fixture.service.enabled = false
	fixture.at(3000)
	fixture.service.Update(fixture.world, .1)
	fixture.arm(t, "axe_single")
	if fixture.actions.State(fixture.world, fixture.actor).Phase != "idle" {
		t.Fatal("disabled combat accepted")
	}
}

func TestCombatNonPlayerActor(t *testing.T) {
	fixture := newCombatFixture(t, 120)
	npc := fixture.world.Spawn(3, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 100, Y: 100})
		ecs.AddComponent(w, h, components.EntityStats{Stamina: 60})
	})
	fixture.service.Strength = func(*ecs.World, types.Handle) (float64, error) { return 1, nil }
	definition, _ := fixture.actions.definitions.Get("axe_aoe")
	if reason := fixture.service.Begin(npc, definition, combatWeapon{Weapon: combat.Weapon{BaseDamage: 6, Range: 18, Quality: 10}}, combat.Point{X: 1}, 0); reason != "" {
		t.Fatal(reason)
	}
	fixture.at(1000)
	fixture.service.Update(fixture.world, .1)
	if fixture.receiver.hp != 94 {
		t.Fatal("NPC required player session/inventory")
	}
}

func TestCombatConflictingWorkAndBusyRequests(t *testing.T) {
	for _, work := range []string{"craft", "build", "context", "carry", "execution"} {
		t.Run(work, func(t *testing.T) {
			fixture := newCombatFixture(t, 120)
			switch work {
			case "craft":
				ecs.AddComponent(fixture.world, fixture.actor, components.ActiveCraft{})
			case "build":
				ecs.AddComponent(fixture.world, fixture.actor, components.PendingBuildPlacement{})
			case "context":
				ecs.AddComponent(fixture.world, fixture.actor, components.PendingContextAction{})
			case "carry":
				ecs.AddComponent(fixture.world, fixture.actor, components.LiftCarryState{})
			case "execution":
				ecs.AddComponent(fixture.world, fixture.actor, components.ActiveGameAction{ActionID: "dig", Phase: components.GameActionExecuting})
			}
			fixture.actions.ActivateCombat(fixture.world, 1, fixture.actor, "axe_aoe")
			if fixture.actions.State(fixture.world, fixture.actor).ActionId == "axe_aoe" {
				t.Fatal("combat replaced active work")
			}
			stats, _ := ecs.GetComponent[components.EntityStats](fixture.world, fixture.actor)
			if stats.Stamina != 120 {
				t.Fatal("busy activation charged")
			}
		})
	}
	fixture := newCombatFixture(t, 180)
	fixture.arm(t, "axe_aoe")
	fixture.commit(t)
	for _, milliseconds := range []int64{200, 700} {
		fixture.at(milliseconds)
		fixture.service.Update(fixture.world, .1)
		fixture.arm(t, "axe_single")
		fixture.actions.Cancel(fixture.world, 1, fixture.actor)
		fixture.actions.StartTargetedOnce(fixture.world, 1, fixture.actor, "lift_down", 0, 0, 100, 100)
		state, _ := ecs.GetComponent[components.CombatState](fixture.world, fixture.actor)
		if state.Execution == nil || state.Execution.ActionID != "axe_aoe" {
			t.Fatal("busy request replaced cycle")
		}
	}
}

func TestCombatEventOrderDepletionAndRegenDelay(t *testing.T) {
	fixture := newCombatFixture(t, 120)
	timing := ecs.GetResource[ecs.TimeState](fixture.world)
	timing.Tick = 100
	timing.TickPeriod = 17 * time.Millisecond
	updates := ecs.GetResource[ecs.EntityStatsUpdateState](fixture.world)
	updates.ScheduleRegen(fixture.actor, 101)
	fixture.arm(t, "axe_aoe")
	fixture.commit(t)
	interval := ecs.ResolveStaminaRegenIntervalTicks(fixture.world)
	if due := updates.PopDueRegen(100+interval-1, nil); len(due) != 0 {
		t.Fatal("combat did not delay regeneration")
	}
	if due := updates.PopDueRegen(100+interval, nil); len(due) != 1 || due[0] != fixture.actor {
		t.Fatal("combat did not schedule regeneration")
	}
	if dirty := updates.PopDuePlayerStatsPush(10000, nil); len(dirty) != 1 || dirty[0] != 1 {
		t.Fatal("spent stamina not marked dirty")
	}
	npc := fixture.world.Spawn(3, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 100, Y: 100})
		ecs.AddComponent(w, h, components.EntityStats{Stamina: 60})
	})
	fixture.service.Strength = func(*ecs.World, types.Handle) (float64, error) { return 1, nil }
	definition, _ := fixture.actions.definitions.Get("axe_aoe")
	if reason := fixture.service.Begin(npc, definition, combatWeapon{Weapon: combat.Weapon{BaseDamage: 6, Range: 18, Quality: 10}}, combat.Point{X: 1}, 0); reason != "" {
		t.Fatal(reason)
	}
	fixture.receiver.hp = 5
	fixture.at(600)
	fixture.service.Update(fixture.world, .017)
	if len(fixture.results) != 2 || fixture.results[0].ActorID != 1 || fixture.results[1].ActorID != 3 || len(fixture.results[0].Hits) != 1 || len(fixture.results[1].Hits) != 0 {
		t.Fatalf("event order/depletion: %+v", fixture.results)
	}
	if fixture.results[0].EventSequence >= fixture.results[0].Hits[0].EventSequence || fixture.results[0].Hits[0].EventSequence >= fixture.results[1].EventSequence {
		t.Fatal("event sequences not monotonic")
	}
}

func TestCombatExternalActorInvalidationAndActivityMaximum(t *testing.T) {
	for _, reason := range []string{"KO", "death", "stun", "despawn", "damage"} {
		t.Run(reason, func(t *testing.T) {
			fixture := newCombatFixture(t, 120)
			fixture.arm(t, "axe_aoe")
			fixture.commit(t)
			ecs.WithComponent(fixture.world, fixture.actor, func(state *components.CombatState) { state.LastCombatEventAt = 900 })
			switch reason {
			case "KO":
				ecs.AddComponent(fixture.world, fixture.actor, components.EntityHealth{HHP: 10, SHP: 0, KOUntilTick: 100})
			case "death":
				ecs.AddComponent(fixture.world, fixture.actor, components.EntityHealth{})
			case "stun":
				ecs.WithComponent(fixture.world, fixture.actor, func(movement *components.Movement) { movement.State = constt.StateStunned })
			case "despawn":
				fixture.world.Despawn(fixture.actor)
			case "damage":
				ecs.AddComponent(fixture.world, fixture.actor, components.EntityHealth{HHP: 10, SHP: 5})
			}
			fixture.at(600)
			fixture.service.Update(fixture.world, .1)
			if reason == "damage" {
				if fixture.receiver.hits != 1 || !components.CombatCommitted(fixture.world, fixture.actor) {
					t.Fatal("ordinary damage interrupted")
				}
			} else if fixture.receiver.hits != 0 || components.CombatCommitted(fixture.world, fixture.actor) {
				t.Fatal("external invalidation did not interrupt")
			}
			if state, ok := ecs.GetComponent[components.CombatState](fixture.world, fixture.actor); ok && state.LastCombatEventAt != 900 {
				t.Fatal("activity moved backward")
			}
		})
	}
}
