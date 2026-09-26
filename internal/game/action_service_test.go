package game

import (
	"testing"

	"origin/internal/actiondefs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

type testActionSender struct {
	states   []*netproto.S2C_ActionStateChanged
	lists    []*netproto.S2C_ActionList
	alerts   []*netproto.S2C_MiniAlert
	finished []*netproto.S2C_CyclicActionFinished
	progress []*netproto.S2C_CyclicActionProgress
}

func (sender *testActionSender) SendActionStateChanged(_ types.EntityID, state *netproto.S2C_ActionStateChanged) {
	sender.states = append(sender.states, state)
}
func (sender *testActionSender) SendActionList(_ types.EntityID, list *netproto.S2C_ActionList) {
	sender.lists = append(sender.lists, list)
}
func (sender *testActionSender) SendMiniAlert(_ types.EntityID, alert *netproto.S2C_MiniAlert) {
	sender.alerts = append(sender.alerts, alert)
}
func (sender *testActionSender) SendCyclicActionFinished(_ types.EntityID, finished *netproto.S2C_CyclicActionFinished) {
	sender.finished = append(sender.finished, finished)
}
func (sender *testActionSender) SendCyclicActionProgress(_ types.EntityID, progress *netproto.S2C_CyclicActionProgress) {
	sender.progress = append(sender.progress, progress)
}

type testActionHandler struct {
	startCount   int
	status       ActionOutcome
	canceled     int
	unavailable  string
	reasonCalls  int
	onValidate   func()
	targetReason func(ActionTarget) string
}

func (handler *testActionHandler) UnavailableReason(*ecs.World, types.EntityID, types.Handle) string {
	handler.reasonCalls++
	return handler.unavailable
}
func (handler *testActionHandler) ValidateTarget(_ *ecs.World, _ types.EntityID, _ types.Handle, target ActionTarget) string {
	if handler.onValidate != nil {
		handler.onValidate()
	}
	if handler.targetReason != nil {
		return handler.targetReason(target)
	}
	if target.ObjectID == 2 {
		return "BAD_TARGET"
	}
	return ""
}
func (handler *testActionHandler) Start(*ecs.World, types.EntityID, types.Handle, ActionTarget, uint64) ActionResult {
	handler.startCount++
	return ActionResult{Outcome: handler.status}
}
func (handler *testActionHandler) Cancel(*ecs.World, types.EntityID, types.Handle, components.ActiveGameAction) {
	handler.canceled++
}

func TestActionCatalogKeepsRegistryOrderWithoutCheckingRequirements(t *testing.T) {
	world := ecs.NewWorldForTesting()
	world.Spawn(1, nil)
	registry := actiondefs.NewRegistry([]actiondefs.Definition{
		{ID: "second", SourceFile: "second.json", Target: actiondefs.Target{Kind: actiondefs.TargetTile}},
		{ID: "first", SourceFile: "first.json", Target: actiondefs.Target{Kind: actiondefs.TargetObject}},
	})
	second := &testActionHandler{unavailable: "ACTION_UNAVAILABLE"}
	first := &testActionHandler{unavailable: "ACTION_UNAVAILABLE"}
	sender := &testActionSender{}
	service, err := NewActionService(world, registry, map[string]ActionHandler{"second": second, "first": first}, sender)
	if err != nil {
		t.Fatal(err)
	}
	service.SendList(1)
	if len(sender.lists) != 1 || len(sender.lists[0].Actions) != 2 {
		t.Fatalf("expected one complete catalog: %#v", sender.lists)
	}
	if sender.lists[0].Actions[0].Id != "second" || sender.lists[0].Actions[1].Id != "first" {
		t.Fatalf("catalog order changed: %#v", sender.lists[0].Actions)
	}
	if second.reasonCalls != 0 || first.reasonCalls != 0 {
		t.Fatal("catalog construction checked player requirements")
	}
}

func TestActionServiceSelectionRejectionAndCompletion(t *testing.T) {
	world := ecs.NewWorldForTesting()
	player := world.Spawn(1, nil)
	world.Spawn(2, nil)
	target := world.Spawn(3, nil)
	sender := &testActionSender{}
	handler := &testActionHandler{status: ActionSucceeded}
	registry := actiondefs.NewRegistry([]actiondefs.Definition{{
		ID: "test_object", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetObject, Cursor: "lift"},
	}})
	service, err := NewActionService(world, registry, map[string]ActionHandler{"test_object": handler}, sender)
	if err != nil {
		t.Fatal(err)
	}
	service.Activate(world, 1, player, "test_object")
	if got := service.State(world, player); got.Phase != "selecting" || got.Cursor != "lift" {
		t.Fatalf("expected selecting, got %#v", got)
	}
	if !service.HandleArmedClick(world, 1, player, 2, world.GetHandleByEntityID(2), 5, 6) {
		t.Fatal("live invalid object click was not consumed")
	}
	if got := service.State(world, player); got.Phase != "selecting" || handler.startCount != 0 {
		t.Fatalf("rejection changed action state: %#v", got)
	}
	if len(sender.alerts) != 1 || sender.alerts[0].ReasonCode != "BAD_TARGET" {
		t.Fatalf("missing rejection alert: %#v", sender.alerts)
	}
	if !service.HandleArmedClick(world, 1, player, 3, target, 7, 8) {
		t.Fatal("valid object click was not consumed")
	}
	if got := service.State(world, player); got.Phase != "idle" || got.Cursor != "" || handler.startCount != 1 {
		t.Fatalf("successful action did not end: %#v", got)
	}
	if len(sender.states) != 2 || sender.states[0].Phase != "selecting" || sender.states[1].Phase != "idle" {
		t.Fatalf("instant action sent an intermediate state: %#v", sender.states)
	}
}

func TestActionRequirementsDoNotRefreshCatalog(t *testing.T) {
	previousItems := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousItems) })
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 101, Key: "test_axe", Tags: []string{"axe"}},
		{DefID: 102, Key: "test_pack"},
	}))
	world := ecs.NewWorldForTesting()
	player := world.Spawn(1, func(world *ecs.World, handle types.Handle) {
		ecs.AddComponent(world, handle, components.CharacterProfile{Skills: []string{"woodcutting"}})
		ecs.AddComponent(world, handle, components.EntityStats{Stamina: 5})
	})
	equipment := world.Spawn(10, func(world *ecs.World, handle types.Handle) {
		ecs.AddComponent(world, handle, components.InventoryContainer{OwnerID: 1, Kind: constt.InventoryEquipment, Items: []components.InvItem{
			{TypeID: 101, EquipSlot: netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND},
			{TypeID: 102, EquipSlot: netproto.EquipSlot_EQUIP_SLOT_BACK},
		}})
	})
	ecs.GetResource[ecs.InventoryRefIndex](world).Add(constt.InventoryEquipment, 1, 0, equipment)
	registry := actiondefs.NewRegistry([]actiondefs.Definition{{ID: "test_tile", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetTile},
		Requirements: actiondefs.Requirements{Skills: []string{"woodcutting"}, Equipment: []actiondefs.EquipmentRequirement{
			{Slots: []string{"left_hand", "right_hand"}, ItemTag: "axe"},
			{Slots: []string{"back"}, ItemKey: "test_pack"},
		}}, Execution: actiondefs.Execution{Stamina: 3}}})
	sender := &testActionSender{}
	service, err := NewActionService(world, registry, map[string]ActionHandler{"test_tile": &testActionHandler{status: ActionSucceeded}}, sender)
	if err != nil {
		t.Fatal(err)
	}
	service.SendList(1)
	if len(sender.lists) != 1 || len(sender.lists[0].Actions) != 1 {
		t.Fatalf("expected one catalog snapshot: %#v", sender.lists)
	}
	listed := sender.lists[0].Actions[0]
	if listed.Id != "test_tile" || listed.Stamina != 3 || len(listed.RequiredSkills) != 1 || len(listed.RequiredEquipment) != 2 {
		t.Fatalf("catalog omitted definition data: %#v", listed)
	}
	service.Activate(world, 1, player, "test_tile")
	ecs.WithComponent(world, equipment, func(container *components.InventoryContainer) { container.Items = container.Items[:1] })
	service.Recheck(world, 1, player)
	if service.State(world, player).Phase != "idle" {
		t.Fatal("losing equipment did not cancel the action")
	}
	if len(sender.lists) != 1 {
		t.Fatalf("equipment loss refreshed catalog: %#v", sender.lists)
	}
	ecs.WithComponent(world, equipment, func(container *components.InventoryContainer) {
		container.Items = append(container.Items, components.InvItem{TypeID: 102, EquipSlot: netproto.EquipSlot_EQUIP_SLOT_BACK})
	})
	ecs.WithComponent(world, player, func(stats *components.EntityStats) { stats.Stamina = 2 })
	service.Activate(world, 1, player, "test_tile")
	if len(sender.alerts) != 1 || sender.alerts[0].ReasonCode != "LOW_STAMINA" {
		t.Fatalf("insufficient stamina alert: %#v", sender.alerts)
	}
	ecs.WithComponent(world, player, func(stats *components.EntityStats) { stats.Stamina = 5 })
	ecs.WithComponent(world, player, func(profile *components.CharacterProfile) { profile.Skills = nil })
	service.Activate(world, 1, player, "test_tile")
	if len(sender.alerts) != 2 || sender.alerts[1].ReasonCode != "ACTION_REQUIRES_SKILL" {
		t.Fatalf("missing skill alert: %#v", sender.alerts)
	}
	if service.State(world, player).Phase != "idle" || len(sender.lists) != 1 {
		t.Fatal("rejected activation started an action or refreshed catalog")
	}
}

func TestActionDirectValidationAtTargetAndCommit(t *testing.T) {
	world := ecs.NewWorldForTesting()
	player := world.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.CharacterProfile{Skills: []string{"woodcutting"}})
		ecs.AddComponent(w, h, components.EntityStats{Stamina: 5})
	})
	registry := actiondefs.NewRegistry([]actiondefs.Definition{{
		ID: "test_tile", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetTile},
		Requirements: actiondefs.Requirements{Skills: []string{"woodcutting"}},
		Execution:    actiondefs.Execution{Stamina: 2},
	}})
	handler := &testActionHandler{status: ActionDeferred}
	sender := &testActionSender{}
	service, err := NewActionService(world, registry, map[string]ActionHandler{"test_tile": handler}, sender)
	if err != nil {
		t.Fatal(err)
	}
	setSkill := func(skills []string) {
		ecs.WithComponent(world, player, func(profile *components.CharacterProfile) { profile.Skills = skills })
	}

	service.Activate(world, 1, player, "test_tile")
	setSkill(nil)
	service.HandleArmedClick(world, 1, player, 0, types.InvalidHandle, 7, 8)
	if service.State(world, player).Phase != "idle" || handler.startCount != 0 || len(sender.alerts) != 1 || sender.alerts[0].ReasonCode != "ACTION_REQUIRES_SKILL" {
		t.Fatal("target acceptance did not reject missing skill")
	}

	setSkill([]string{"woodcutting"})
	handler.onValidate = func() { setSkill(nil) }
	service.Activate(world, 1, player, "test_tile")
	service.HandleArmedClick(world, 1, player, 0, types.InvalidHandle, 7, 8)
	if service.State(world, player).Phase != "idle" || handler.startCount != 0 || len(sender.alerts) != 2 || sender.alerts[1].ReasonCode != "ACTION_REQUIRES_SKILL" {
		t.Fatal("handler start did not recheck requirements")
	}

	setSkill([]string{"woodcutting"})
	handler.onValidate = nil
	service.Activate(world, 1, player, "test_tile")
	service.HandleArmedClick(world, 1, player, 0, types.InvalidHandle, 7, 8)
	active, exists := ecs.GetComponent[components.ActiveGameAction](world, player)
	if !exists || active.Phase != components.GameActionExecuting || handler.startCount != 1 {
		t.Fatal("deferred action did not start")
	}
	setSkill(nil)
	if service.CanCommit(world, 1, player, active.Generation) {
		t.Fatal("commit accepted lost skill")
	}
	service.Recheck(world, 1, player)
	stats, _ := ecs.GetComponent[components.EntityStats](world, player)
	if service.State(world, player).Phase != "idle" || stats.Stamina != 5 {
		t.Fatal("rejected commit charged stamina or left action active")
	}
}

func TestActionTimedAndInstantCosts(t *testing.T) {
	for _, test := range []struct {
		name        string
		ticks       int
		outcome     ActionOutcome
		wantStamina float64
	}{
		{name: "timed success", ticks: 2, outcome: ActionSucceeded, wantStamina: 8},
		{name: "instant success", ticks: 0, outcome: ActionSucceeded, wantStamina: 8},
		{name: "timed failure", ticks: 2, outcome: ActionFailed, wantStamina: 10},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := ecs.NewWorldForTesting()
			player := world.Spawn(1, func(world *ecs.World, handle types.Handle) {
				ecs.AddComponent(world, handle, components.EntityStats{Stamina: 10})
			})
			registry := actiondefs.NewRegistry([]actiondefs.Definition{{ID: "test_none", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetNone}, Execution: actiondefs.Execution{Ticks: test.ticks, Stamina: 2}}})
			handler, sender := &testActionHandler{status: test.outcome}, &testActionSender{}
			service, err := NewActionService(world, registry, map[string]ActionHandler{"test_none": handler}, sender)
			if err != nil {
				t.Fatal(err)
			}
			service.Activate(world, 1, player, "test_none")
			for tick := 0; tick < test.ticks; tick++ {
				cycle, exists := ecs.GetComponent[components.ActiveCyclicAction](world, player)
				if !exists {
					t.Fatal("timed cycle is missing")
				}
				service.AdvanceCycle(world, 1, player, cycle, sender)
				if tick < test.ticks-1 {
					stats, _ := ecs.GetComponent[components.EntityStats](world, player)
					if stats.Stamina != 10 {
						t.Fatal("stamina charged before success")
					}
				}
			}
			stats, _ := ecs.GetComponent[components.EntityStats](world, player)
			if stats.Stamina != test.wantStamina || handler.startCount != 1 || service.State(world, player).Phase != "idle" {
				t.Fatalf("wrong result: stats=%#v starts=%d state=%#v", stats, handler.startCount, service.State(world, player))
			}
			if test.ticks > 0 {
				if len(sender.states) != 2 || sender.states[0].Phase != "executing" || sender.states[1].Phase != "idle" {
					t.Fatalf("timed action did not report its cycle state: %#v", sender.states)
				}
			} else if len(sender.states) != 1 || sender.states[0].Phase != "idle" {
				t.Fatalf("instant action sent an intermediate state: %#v", sender.states)
			}
			if test.ticks > 0 && (len(sender.progress) != test.ticks || len(sender.finished) != 1) {
				t.Fatalf("wrong cycle packets: progress=%d finished=%d", len(sender.progress), len(sender.finished))
			}
		})
	}
}

func TestCanceledTimedActionHasNoCost(t *testing.T) {
	world := ecs.NewWorldForTesting()
	player := world.Spawn(1, func(world *ecs.World, handle types.Handle) {
		ecs.AddComponent(world, handle, components.EntityStats{Stamina: 10})
	})
	registry := actiondefs.NewRegistry([]actiondefs.Definition{{ID: "test_none", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetNone}, Execution: actiondefs.Execution{Ticks: 3, Stamina: 2}}})
	handler, sender := &testActionHandler{status: ActionSucceeded}, &testActionSender{}
	service, err := NewActionService(world, registry, map[string]ActionHandler{"test_none": handler}, sender)
	if err != nil {
		t.Fatal(err)
	}
	service.Activate(world, 1, player, "test_none")
	cycle, _ := ecs.GetComponent[components.ActiveCyclicAction](world, player)
	service.AdvanceCycle(world, 1, player, cycle, sender)
	service.Cancel(world, 1, player)
	stats, _ := ecs.GetComponent[components.EntityStats](world, player)
	if stats.Stamina != 10 || handler.startCount != 0 || len(sender.finished) != 1 || sender.finished[0].Result != netproto.CyclicActionFinishResult_CYCLIC_ACTION_FINISH_RESULT_CANCELED {
		t.Fatalf("canceled action had an effect: %#v", stats)
	}
}

func TestActionRepeatAndTargetFailure(t *testing.T) {
	for _, repeatable := range []bool{false, true} {
		world := ecs.NewWorldForTesting()
		player := world.Spawn(1, nil)
		target := world.Spawn(3, nil)
		registry := actiondefs.NewRegistry([]actiondefs.Definition{{
			ID: "test_object", SourceFile: "test.json",
			Target:       actiondefs.Target{Kind: actiondefs.TargetObject, Cursor: "lift"},
			IsRepeatable: &repeatable,
		}})
		handler := &testActionHandler{status: ActionFailed}
		service, err := NewActionService(world, registry, map[string]ActionHandler{"test_object": handler}, &testActionSender{})
		if err != nil {
			t.Fatal(err)
		}
		service.Activate(world, 1, player, "test_object")
		service.HandleArmedClick(world, 1, player, 3, target, 5, 6)
		if state := service.State(world, player); state.Phase != "selecting" || state.Cursor != "lift" {
			t.Fatalf("target failure must allow retry: %#v", state)
		}
		handler.status = ActionSucceeded
		service.HandleArmedClick(world, 1, player, 3, target, 5, 6)
		wantPhase := "idle"
		if repeatable {
			wantPhase = "selecting"
		}
		if state := service.State(world, player); state.Phase != wantPhase {
			t.Fatalf("repeatable=%t: got %#v", repeatable, state)
		}
	}
}

func TestRepeatableTileActionAcceptsAnotherClick(t *testing.T) {
	world := ecs.NewWorldForTesting()
	player := world.Spawn(1, nil)
	repeatable := true
	registry := actiondefs.NewRegistry([]actiondefs.Definition{{
		ID: "test_tile", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetTile, Cursor: "dig"}, IsRepeatable: &repeatable,
	}})
	handler := &testActionHandler{status: ActionSucceeded}
	service, err := NewActionService(world, registry, map[string]ActionHandler{"test_tile": handler}, &testActionSender{})
	if err != nil {
		t.Fatal(err)
	}
	service.Activate(world, 1, player, "test_tile")
	for index := 0; index < 2; index++ {
		service.HandleArmedClick(world, 1, player, 0, types.InvalidHandle, float64(index+1), 8)
		if state := service.State(world, player); state.Phase != "selecting" || state.Cursor != "dig" {
			t.Fatalf("repeatable tile did not re-arm: %#v", state)
		}
	}
	if handler.startCount != 2 {
		t.Fatalf("expected two executions, got %d", handler.startCount)
	}
}

func TestActionSwitchCancelsApproachAndLateCallback(t *testing.T) {
	world := ecs.NewWorldForTesting()
	player := world.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{})
	})
	target := world.Spawn(3, nil)
	registry := actiondefs.NewRegistry([]actiondefs.Definition{
		{ID: "test_object", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetObject}},
		{ID: "test_tile", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetTile}},
	})
	approach := &testActionHandler{status: ActionApproaching}
	tile := &testActionHandler{status: ActionSucceeded}
	sender := &testActionSender{}
	service, err := NewActionService(world, registry, map[string]ActionHandler{"test_object": approach, "test_tile": tile}, sender)
	if err != nil {
		t.Fatal(err)
	}
	service.Activate(world, 1, player, "test_object")
	ecs.WithComponent(world, player, func(m *components.Movement) { m.SetTargetHandle(target, 5, 6) })
	service.HandleArmedClick(world, 1, player, 3, target, 5, 6)
	old, _ := ecs.GetComponent[components.ActiveGameAction](world, player)
	if old.Phase != components.GameActionApproaching {
		t.Fatalf("expected approach: %#v", old)
	}
	service.Activate(world, 1, player, "test_tile")
	if approach.canceled != 1 || service.State(world, player).ActionId != "test_tile" {
		t.Fatalf("switch did not cancel old action: cancels=%d state=%#v", approach.canceled, service.State(world, player))
	}
	service.Complete(world, 1, player, old.Generation, true, "")
	if service.State(world, player).ActionId != "test_tile" {
		t.Fatal("late callback changed replacement action")
	}
	service.HandleArmedClick(world, 1, player, 0, types.InvalidHandle, 7, 8)
	if tile.startCount != 1 || service.State(world, player).Phase != "idle" {
		t.Fatal("replacement tile action did not execute")
	}
}

func TestActionLossDuringCycleAndUnavailableReplacement(t *testing.T) {
	world := ecs.NewWorldForTesting()
	player := world.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.CharacterProfile{Skills: []string{"woodcutting"}})
		ecs.AddComponent(w, h, components.EntityStats{Stamina: 10})
	})
	registry := actiondefs.NewRegistry([]actiondefs.Definition{
		{ID: "test_none", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetNone}, Requirements: actiondefs.Requirements{Skills: []string{"woodcutting"}}, Execution: actiondefs.Execution{Ticks: 3, Stamina: 2}},
		{ID: "test_tile", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetTile}},
	})
	cycleHandler := &testActionHandler{status: ActionSucceeded}
	unavailable := &testActionHandler{status: ActionSucceeded, unavailable: "ACTION_UNAVAILABLE"}
	sender := &testActionSender{}
	service, err := NewActionService(world, registry, map[string]ActionHandler{"test_none": cycleHandler, "test_tile": unavailable}, sender)
	if err != nil {
		t.Fatal(err)
	}
	service.Activate(world, 1, player, "test_none")
	service.Activate(world, 1, player, "test_tile")
	if service.State(world, player).Phase != "idle" || cycleHandler.startCount != 0 || sender.alerts[len(sender.alerts)-1].ReasonCode != "ACTION_UNAVAILABLE" {
		t.Fatal("unavailable replacement must leave idle without effect")
	}
	service.Activate(world, 1, player, "test_none")
	ecs.WithComponent(world, player, func(profile *components.CharacterProfile) { profile.Skills = nil })
	service.Recheck(world, 1, player)
	stats, _ := ecs.GetComponent[components.EntityStats](world, player)
	if service.State(world, player).Phase != "idle" || stats.Stamina != 10 || cycleHandler.startCount != 0 {
		t.Fatal("skill loss did not cancel cycle without cost")
	}
}

func TestTileActionIgnoresClickedObjectAndStopsOwnedPointMovement(t *testing.T) {
	world := ecs.NewWorldForTesting()
	player := world.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{})
	})
	target := world.Spawn(3, nil)
	registry := actiondefs.NewRegistry([]actiondefs.Definition{{ID: "test_tile", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetTile}}})
	service, err := NewActionService(world, registry, map[string]ActionHandler{"test_tile": &testActionHandler{status: ActionApproaching}}, &testActionSender{})
	if err != nil {
		t.Fatal(err)
	}
	service.Activate(world, 1, player, "test_tile")
	service.HandleArmedClick(world, 1, player, 3, target, 7, 8)
	active, _ := ecs.GetComponent[components.ActiveGameAction](world, player)
	if active.TargetID != 0 || active.TargetHandle != types.InvalidHandle {
		t.Fatalf("tile action retained clicked object: %#v", active)
	}
	ecs.WithComponent(world, player, func(m *components.Movement) { m.SetTargetPoint(7, 8) })
	service.Cancel(world, 1, player)
	movement, _ := ecs.GetComponent[components.Movement](world, player)
	if movement.TargetType != constt.TargetNone {
		t.Fatalf("cancel did not stop tile action movement: %#v", movement)
	}
}

func TestActionApproachTimeoutReturnsToSelection(t *testing.T) {
	world := ecs.NewWorldForTesting()
	player := world.Spawn(1, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, components.Movement{}) })
	target := world.Spawn(3, nil)
	registry := actiondefs.NewRegistry([]actiondefs.Definition{{ID: "test_object", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetObject, Cursor: "lift"}}})
	handler := &testActionHandler{status: ActionApproaching}
	sender := &testActionSender{}
	service, err := NewActionService(world, registry, map[string]ActionHandler{"test_object": handler}, sender)
	if err != nil {
		t.Fatal(err)
	}
	service.Activate(world, 1, player, "test_object")
	service.HandleArmedClick(world, 1, player, 3, target, 7, 8)
	if len(sender.states) != 2 || sender.states[0].Phase != "selecting" || sender.states[1].Phase != "approaching" {
		t.Fatalf("untimed approach sent an intermediate state: %#v", sender.states)
	}
	active, _ := ecs.GetComponent[components.ActiveGameAction](world, player)
	if active.ExpireAtUnixMs <= 0 {
		t.Fatal("approach has no deadline")
	}
	ecs.GetResource[ecs.TimeState](world).UnixMs = active.ExpireAtUnixMs
	service.Recheck(world, 1, player)
	if state := service.State(world, player); state.Phase != "selecting" || state.Cursor != "lift" {
		t.Fatalf("timeout did not restore selection: %#v", state)
	}
	if handler.canceled != 1 || len(sender.alerts) != 1 || sender.alerts[0].ReasonCode != "ACTION_TARGET_TIMEOUT" {
		t.Fatal("timeout did not cancel approach and report reason")
	}
}

func TestPermanentDeathClearsActiveActionAndCycle(t *testing.T) {
	world := ecs.NewWorldForTesting()
	player := world.Spawn(1, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, components.EntityStats{Stamina: 10}) })
	registry := actiondefs.NewRegistry([]actiondefs.Definition{{ID: "test_none", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetNone}, Execution: actiondefs.Execution{Ticks: 3, Stamina: 2}}})
	handler := &testActionHandler{status: ActionSucceeded}
	sender := &testActionSender{}
	service, err := NewActionService(world, registry, map[string]ActionHandler{"test_none": handler}, sender)
	if err != nil {
		t.Fatal(err)
	}
	service.Activate(world, 1, player, "test_none")
	(&Shard{actionService: service}).clearPlayerTransientStateForDeath(world, 1, player)
	if service.State(world, player).Phase != "idle" {
		t.Fatal("death left action active")
	}
	if _, cycle := ecs.GetComponent[components.ActiveCyclicAction](world, player); cycle {
		t.Fatal("death left action cycle")
	}
	stats, _ := ecs.GetComponent[components.EntityStats](world, player)
	if stats.Stamina != 10 || handler.startCount != 0 {
		t.Fatal("death charged stamina or ran handler")
	}
}

func TestReconnectCannotReuseCanceledActionGeneration(t *testing.T) {
	world := ecs.NewWorldForTesting()
	player := world.Spawn(1, nil)
	registry := actiondefs.NewRegistry([]actiondefs.Definition{{ID: "test_tile", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetTile}}})
	sender := &testActionSender{}
	service, err := NewActionService(world, registry, map[string]ActionHandler{"test_tile": &testActionHandler{status: ActionSucceeded}}, sender)
	if err != nil {
		t.Fatal(err)
	}
	service.SendList(1)
	service.Activate(world, 1, player, "test_tile")
	previous, _ := ecs.GetComponent[components.ActiveGameAction](world, player)
	service.Cancel(world, 1, player)
	service.SendList(1)
	service.SendState(world, 1, player)
	if len(sender.lists) != 2 || len(sender.lists[1].Actions) != 1 || sender.lists[1].Actions[0].Id != "test_tile" {
		t.Fatalf("re-entry did not receive a fresh catalog: %#v", sender.lists)
	}
	if sender.states[len(sender.states)-1].Phase != "idle" {
		t.Fatal("re-entry reused the old action state")
	}
	service.Activate(world, 1, player, "test_tile")
	current, _ := ecs.GetComponent[components.ActiveGameAction](world, player)
	if current.Generation == previous.Generation {
		t.Fatal("reconnect reused action generation")
	}
	service.Complete(world, 1, player, previous.Generation, true, "")
	if service.State(world, player).Phase != "selecting" {
		t.Fatal("old completion changed new action")
	}
}
