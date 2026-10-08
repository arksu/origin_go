package game

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestObjectDestructionShutdownDeadlineReportsAndRetainsPendingState(t *testing.T) {
	f := newDestructionServiceFixture(t)
	root := f.addContainer(1, 0, lootItem(20, 1, 1))
	f.chunks.accept = false
	_, err := f.damage.Apply(f.target, 100)
	require.NoError(t, err)
	f.service.StopAdmission()
	logCore, logs := observer.New(zap.ErrorLevel)
	s := &Shard{world: f.w, objectDestruction: f.service, logger: zap.New(logCore)}
	before := *ecs.GetResource[ecs.TimeState](f.w)
	// Exercise the deadline branch without adding a thirty-second CI delay.
	s.drainObjectDestructionFor(0)
	require.Equal(t, before, *ecs.GetResource[ecs.TimeState](f.w))
	require.Len(t, logs.FilterMessage("Shutdown object persistence incomplete").All(), 1)
	require.Equal(t, 1, f.service.PendingCount())
	require.True(t, f.w.Alive(f.target))
	require.True(t, f.w.Alive(root))
	health, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
	require.Zero(t, health.HP)
	require.True(t, ecs.ObjectDestructionPending(f.w, f.target))
	require.Empty(t, f.chunks.inserted)
	require.Empty(t, f.persister.calls)
	require.Equal(t, 1, f.chunks.pins[f.service.operations[0].source])
}
