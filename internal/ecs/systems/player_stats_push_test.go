package systems

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/types"
)

type playerStatsPushCall struct {
	entityID types.EntityID
	handle   types.Handle
}

type playerStatsPushSenderMock struct {
	statsCalls []playerStatsPushCall
	modeCalls  []playerStatsPushCall
}

func (m *playerStatsPushSenderMock) SendPlayerStatsDeltaIfChanged(w *ecs.World, entityID types.EntityID, handle types.Handle) bool {
	_ = w
	m.statsCalls = append(m.statsCalls, playerStatsPushCall{entityID: entityID, handle: handle})
	return true
}

func (m *playerStatsPushSenderMock) SendMovementModeDeltaIfChanged(w *ecs.World, entityID types.EntityID, handle types.Handle) bool {
	_ = w
	m.modeCalls = append(m.modeCalls, playerStatsPushCall{entityID: entityID, handle: handle})
	return true
}

func TestPlayerStatsPushSystem_DispatchesDueEntries(t *testing.T) {
	world := ecs.NewWorldForTesting()
	entityID := types.EntityID(101)
	handle := world.Spawn(entityID, nil)
	state := ecs.GetResource[ecs.EntityStatsUpdateState](world)
	if !state.MarkPlayerDirty(entityID, 1000, 1000) {
		t.Fatalf("expected MarkPlayerDirty to succeed")
	}
	ecs.SetResource(world, ecs.TimeState{UnixMs: 1000})

	sender := &playerStatsPushSenderMock{}
	system := NewPlayerStatsPushSystem(sender)
	system.Update(world, 0)

	if len(sender.statsCalls) != 1 {
		t.Fatalf("expected one push call, got %d", len(sender.statsCalls))
	}
	if sender.statsCalls[0].entityID != entityID || sender.statsCalls[0].handle != handle {
		t.Fatalf("unexpected push call payload: %+v", sender.statsCalls[0])
	}
}

func TestPlayerStatsPushSystem_RespectsDueTime(t *testing.T) {
	world := ecs.NewWorldForTesting()
	entityID := types.EntityID(102)
	_ = world.Spawn(entityID, nil)
	state := ecs.GetResource[ecs.EntityStatsUpdateState](world)

	if !state.MarkPlayerSent(entityID, 1000) {
		t.Fatalf("expected MarkPlayerSent to succeed")
	}
	if !state.MarkPlayerDirty(entityID, 1100, 1000) {
		t.Fatalf("expected MarkPlayerDirty to succeed")
	}

	sender := &playerStatsPushSenderMock{}
	system := NewPlayerStatsPushSystem(sender)

	ecs.SetResource(world, ecs.TimeState{UnixMs: 1500})
	system.Update(world, 0)
	if len(sender.statsCalls) != 0 {
		t.Fatalf("expected no push before due time, got %d", len(sender.statsCalls))
	}

	ecs.SetResource(world, ecs.TimeState{UnixMs: 2000})
	system.Update(world, 0)
	if len(sender.statsCalls) != 1 {
		t.Fatalf("expected one push at due time, got %d", len(sender.statsCalls))
	}
}

func TestPlayerStatsPushSystem_DropsInvalidEntitySilently(t *testing.T) {
	world := ecs.NewWorldForTesting()
	entityID := types.EntityID(103)
	state := ecs.GetResource[ecs.EntityStatsUpdateState](world)
	if !state.MarkPlayerDirty(entityID, 0, 1000) {
		t.Fatalf("expected MarkPlayerDirty to succeed")
	}
	ecs.SetResource(world, ecs.TimeState{UnixMs: 0})

	sender := &playerStatsPushSenderMock{}
	system := NewPlayerStatsPushSystem(sender)
	system.Update(world, 0)

	if len(sender.statsCalls) != 0 {
		t.Fatalf("expected no push calls for invalid entity, got %d", len(sender.statsCalls))
	}
	if state.PendingPlayerPushCount() != 0 {
		t.Fatalf("expected no pending pushes after invalid entity drop, got %d", state.PendingPlayerPushCount())
	}
}

func TestPlayerStatsPushSystem_DoesNotSendWithoutDirty(t *testing.T) {
	world := ecs.NewWorldForTesting()
	ecs.SetResource(world, ecs.TimeState{UnixMs: 1000})

	sender := &playerStatsPushSenderMock{}
	system := NewPlayerStatsPushSystem(sender)
	system.Update(world, 0)
	system.Update(world, 0)

	if len(sender.statsCalls) != 0 {
		t.Fatalf("expected zero calls without dirty entries, got %d", len(sender.statsCalls))
	}
	if len(sender.modeCalls) != 0 {
		t.Fatalf("expected zero mode calls without dirty entries, got %d", len(sender.modeCalls))
	}
}

func TestPlayerStatsPushSystem_DispatchesMovementModeDirty(t *testing.T) {
	world := ecs.NewWorldForTesting()
	entityID := types.EntityID(104)
	handle := world.Spawn(entityID, nil)
	state := ecs.GetResource[ecs.EntityStatsUpdateState](world)
	if !state.MarkMovementModeDirty(entityID) {
		t.Fatalf("expected MarkMovementModeDirty to succeed")
	}

	sender := &playerStatsPushSenderMock{}
	system := NewPlayerStatsPushSystem(sender)
	system.Update(world, 0)

	if len(sender.modeCalls) != 1 {
		t.Fatalf("expected one mode push call, got %d", len(sender.modeCalls))
	}
	if sender.modeCalls[0].entityID != entityID || sender.modeCalls[0].handle != handle {
		t.Fatalf("unexpected mode push call payload: %+v", sender.modeCalls[0])
	}
}
