package game

import (
	"errors"
	"math"
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

type reservationChunksFixture struct {
	*destructionChunksFixture
	call, failAt int
	err          error
}

func (c *reservationChunksFixture) PinPersistence(coord types.ChunkCoord) error {
	c.call++
	if c.call == c.failAt {
		return c.err
	}
	return c.destructionChunksFixture.PinPersistence(coord)
}

func TestObjectDamagePlanReservesWithoutPublishingAndFinalizesAfterCommit(t *testing.T) {
	f := newDestructionServiceFixture(t)
	before, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
	writes := 0
	f.w.AddComponentObserver(components.ObjectInternalStateComponentID, func(types.Handle) { writes++ })
	commit, err := f.damage.prepareDamage(f.target, 100)
	require.NoError(t, err)
	require.True(t, commit.result.EnteredDestruction)
	require.NoError(t, f.damage.reserveDamage(&commit))
	after, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
	require.Equal(t, before, after)
	require.Zero(t, writes)
	require.False(t, ecs.ObjectDestructionPending(f.w, f.target))
	require.True(t, ecs.HasComponent[components.Collider](f.w, f.target))
	f.service.Update()
	require.Empty(t, f.chunks.jobs, "reserved operations are invisible to workers")
	f.damage.commitDamage(commit)
	after, _ = ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
	require.Zero(t, after.HP)
	require.True(t, after.IsDirty)
	require.Equal(t, 1, writes)
	require.True(t, ecs.ObjectDestructionPending(f.w, f.target))
	require.True(t, ecs.HasComponent[components.Collider](f.w, f.target))
	f.service.Update()
	require.Empty(t, f.chunks.jobs, "committed operations await quarantine before worker capture")
	f.damage.finalizeDamage(commit)
	require.False(t, ecs.HasComponent[components.Collider](f.w, f.target))
	f.service.Update()
	require.Len(t, f.chunks.jobs, 1)
}

func TestObjectDamageReservationFailureRollsBackEarlierPins(t *testing.T) {
	f := newDestructionServiceFixture(t)
	second := f.targetWithID(t, 2)
	root := f.addContainer(1, 0, lootItem(100, 1, 2))
	blocked := errors.New("second source pin rejected")
	f.service.deps.Chunks = &reservationChunksFixture{destructionChunksFixture: f.chunks, failAt: 2, err: blocked}
	firstBefore, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
	secondBefore, _ := ecs.GetComponent[components.ObjectInternalState](f.w, second)
	first, err := f.damage.prepareDamage(f.target, 100)
	require.NoError(t, err)
	require.NoError(t, f.damage.reserveDamage(&first))
	next, err := f.damage.prepareDamage(second, 100)
	require.NoError(t, err)
	require.ErrorIs(t, f.damage.reserveDamage(&next), blocked)
	f.damage.abortDamage(first)
	f.damage.abortDamage(next)
	firstAfter, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
	secondAfter, _ := ecs.GetComponent[components.ObjectInternalState](f.w, second)
	require.Equal(t, firstBefore, firstAfter)
	require.Equal(t, secondBefore, secondAfter)
	require.True(t, f.w.Alive(root))
	require.True(t, ecs.HasComponent[components.Collider](f.w, f.target))
	require.True(t, ecs.HasComponent[components.Collider](f.w, second))
	require.False(t, ecs.ObjectDestructionPending(f.w, f.target))
	require.False(t, ecs.ObjectDestructionPending(f.w, second))
	require.Zero(t, f.service.PendingCount())
	require.Equal(t, ObjectDestructionQueueCapacity, f.service.freeCount)
	for _, pins := range f.chunks.pins {
		require.Zero(t, pins)
	}
	f.service.Update()
	require.Empty(t, f.chunks.jobs)
}

func TestObjectDamageReservationQueueLimitCanBeCanceledCompletely(t *testing.T) {
	f := newDestructionServiceFixture(t)
	var plans [ObjectDestructionQueueCapacity]objectDamageCommit
	for i := range plans {
		target := f.target
		if i > 0 {
			target = f.targetWithID(t, types.EntityID(i+1))
		}
		var err error
		plans[i], err = f.damage.prepareDamage(target, 100)
		require.NoError(t, err)
		require.NoError(t, f.damage.reserveDamage(&plans[i]))
	}
	overflow := f.targetWithID(t, ObjectDestructionQueueCapacity+1)
	plan, err := f.damage.prepareDamage(overflow, 100)
	require.NoError(t, err)
	require.ErrorIs(t, f.damage.reserveDamage(&plan), ErrObjectDestructionQueueFull)
	f.service.Update()
	require.Empty(t, f.chunks.jobs)
	for _, plan := range plans {
		f.damage.abortDamage(plan)
		health, _ := ecs.GetComponent[components.ObjectInternalState](f.w, plan.target)
		require.Equal(t, float64(100), health.HP)
		require.False(t, health.IsDirty)
		require.False(t, ecs.ObjectDestructionPending(f.w, plan.target))
	}
	require.Zero(t, f.service.PendingCount())
	require.Equal(t, ObjectDestructionQueueCapacity, f.service.freeCount)
	for _, pins := range f.chunks.pins {
		require.Zero(t, pins)
	}
}

func TestObjectDamageReservationTicketsRejectDuplicateAndReusedTokens(t *testing.T) {
	f := newDestructionServiceFixture(t)
	first, err := f.damage.prepareDamage(f.target, 100)
	require.NoError(t, err)
	require.NoError(t, f.damage.reserveDamage(&first))
	_, err = f.service.reserve(f.target)
	require.ErrorIs(t, err, ErrObjectDestructionPending)
	require.True(t, f.service.cancelReservation(first.reservation))
	require.False(t, f.service.cancelReservation(first.reservation))
	second := f.targetWithID(t, 2)
	next, err := f.damage.prepareDamage(second, 100)
	require.NoError(t, err)
	require.NoError(t, f.damage.reserveDamage(&next))
	require.Equal(t, first.reservation.slot, next.reservation.slot)
	require.NotEqual(t, first.reservation.ticket, next.reservation.ticket)
	require.False(t, f.service.commitReservation(first.reservation))
	require.False(t, f.service.cancelReservation(first.reservation))
	require.Equal(t, 1, f.service.PendingCount())
	f.damage.commitDamage(next)
	require.False(t, f.service.commitReservation(next.reservation))
	require.False(t, f.service.cancelReservation(next.reservation))
	f.damage.finalizeDamage(next)
	require.False(t, f.service.finalizeReservation(next.reservation))
	for _, invalid := range []objectDestructionReservation{
		{}, {service: f.service, slot: ObjectDestructionQueueCapacity, ticket: next.reservation.ticket},
		{service: &ObjectDestructionService{}, slot: next.reservation.slot, ticket: next.reservation.ticket},
	} {
		require.False(t, f.service.cancelReservation(invalid))
		require.False(t, f.service.commitReservation(invalid))
		require.False(t, f.service.finalizeReservation(invalid))
	}
	require.Equal(t, 1, f.service.PendingCount())
}

func TestObjectDamageReservationStoppedAndOverflowLeavePlanUnchanged(t *testing.T) {
	for _, expected := range []error{ErrObjectDestructionStopped, ErrObjectDestructionOverflow} {
		f := newObjectDamageFixture(t, nil)
		if expected == ErrObjectDestructionStopped {
			f.destruction.StopAdmission()
		} else {
			f.destruction.sequence = math.MaxUint64
		}
		plan, err := f.service.prepareDamage(f.target, 100)
		require.NoError(t, err)
		before := plan
		require.ErrorIs(t, f.service.reserveDamage(&plan), expected)
		require.Equal(t, before, plan)
		require.Equal(t, float64(100), f.hp().HP)
		require.False(t, f.hp().IsDirty)
		require.Zero(t, f.destruction.PendingCount())
	}
}

func TestObjectDestructionFreeSlotReturnsAfterWorkerCompletion(t *testing.T) {
	f := newDestructionServiceFixture(t)
	var previous objectDestructionReservation
	for id := types.EntityID(1); id <= 5; id++ {
		target := f.target
		if id != 1 {
			target = f.targetWithID(t, id)
		}
		plan, err := f.damage.prepareDamage(target, 100)
		require.NoError(t, err)
		require.NoError(t, f.damage.reserveDamage(&plan))
		require.Zero(t, plan.reservation.slot, "completed slots are reused without scanning occupied records")
		require.False(t, f.service.cancelReservation(previous))
		require.False(t, f.service.commitReservation(previous))
		f.damage.commitDamage(plan)
		f.damage.finalizeDamage(plan)
		f.service.Update()
		f.chunks.runOne(t)
		f.service.Update()
		require.False(t, f.w.Alive(target))
		require.Zero(t, f.service.PendingCount())
		require.Equal(t, ObjectDestructionQueueCapacity, f.service.freeCount)
		require.Empty(t, f.service.reserved, "despawn releases prepared reservation metadata")
		previous = plan.reservation
	}
	require.Equal(t, []types.EntityID{1, 2, 3, 4, 5}, f.chunks.removed)
}

func TestObjectDamagePreparationAndReservationAllocations(t *testing.T) {
	f := newObjectDamageFixture(t, nil)
	var plan objectDamageCommit
	var err error
	for _, draw := range []float64{0, 1, 100, -1} {
		allocs := testing.AllocsPerRun(1000, func() {
			plan, err = f.service.prepareDamage(f.target, draw)
			if err == nil {
				err = f.service.reserveDamage(&plan)
				f.service.abortDamage(plan)
			}
		})
		require.Zero(t, allocs)
		if draw >= 0 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
		require.Equal(t, float64(100), f.hp().HP)
		require.Zero(t, f.destruction.PendingCount())
	}
}
