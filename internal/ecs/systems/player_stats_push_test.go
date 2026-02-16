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
	calls []playerStatsPushCall
}

func (m *playerStatsPushSenderMock) SendPlayerStatsDeltaIfChanged(w *ecs.World, entityID types.EntityID, handle types.Handle) bool {
	_ = w
	m.calls = append(m.calls, playerStatsPushCall{entityID: entityID, handle: handle})
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

	if len(sender.calls) != 1 {
		t.Fatalf("expected one push call, got %d", len(sender.calls))
	}
	if sender.calls[0].entityID != entityID || sender.calls[0].handle != handle {
		t.Fatalf("unexpected push call payload: %+v", sender.calls[0])
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
	if len(sender.calls) != 0 {
		t.Fatalf("expected no push before due time, got %d", len(sender.calls))
	}

	ecs.SetResource(world, ecs.TimeState{UnixMs: 2000})
	system.Update(world, 0)
	if len(sender.calls) != 1 {
		t.Fatalf("expected one push at due time, got %d", len(sender.calls))
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

	if len(sender.calls) != 0 {
		t.Fatalf("expected no push calls for invalid entity, got %d", len(sender.calls))
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

	if len(sender.calls) != 0 {
		t.Fatalf("expected zero calls without dirty entries, got %d", len(sender.calls))
	}
}
