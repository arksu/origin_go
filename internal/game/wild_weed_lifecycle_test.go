package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"origin/internal/actiondefs"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

type wildWeedDestructionFixture struct {
	*destructionPersisterFixture
	deletions []gameworld.WildWeedObjectDeletion
	commit    gameworld.SpotStateCommit
	fail      error
}

func (p *wildWeedDestructionFixture) ReplaceWildWeedWithDroppedItems(ctx context.Context, region int, deletion gameworld.WildWeedObjectDeletion, maxID types.EntityID,
	next func([]inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error),
) (gameworld.SpotStateCommit, error) {
	p.deletions = append(p.deletions, deletion)
	if p.fail != nil {
		err := p.fail
		p.fail = nil
		return gameworld.SpotStateCommit{}, err
	}
	if err := p.ReplaceObjectWithDroppedItems(ctx, region, deletion.EntityID, maxID, next); err != nil {
		return gameworld.SpotStateCommit{}, err
	}
	return p.commit, nil
}

func setLifecycleWildWeed(w *ecs.World, handle types.Handle, spotID int64, stage int) {
	ecs.MutateComponent(w, handle, func(state *components.ObjectInternalState) bool {
		components.SetBehaviorState(state, "wild_weed", &components.WildWeedBehaviorState{SpotID: spotID, Stage: stage})
		return true
	})
	ecs.MutateComponent(w, handle, func(info *components.EntityInfo) bool {
		info.Behaviors = []string{"wild_weed"}
		return true
	})
	ecs.MutateComponent(w, handle, func(collider *components.Collider) bool {
		collider.Layer, collider.Mask = 1, 1
		if stage < 3 {
			collider.Layer, collider.Mask = 0, 0
		}
		return true
	})
}

func TestWildWeedDestructionUsesCapturedSpotAndPublishesOnOwner(t *testing.T) {
	for stage := 0; stage <= 3; stage++ {
		t.Run(string(rune('0'+stage)), func(t *testing.T) {
			f := newDestructionServiceFixture(t)
			setLifecycleWildWeed(f.w, f.target, 123, stage)
			p := &wildWeedDestructionFixture{destructionPersisterFixture: f.persister, commit: gameworld.SpotStateCommit{
				SpotID: 123, Snapshot: gameworld.SpotStateSnapshot{State: json.RawMessage(`{"www":{"objects":{}}}`), Revision: 2, LastRuntimeSeconds: 10},
			}}
			f.service.deps.Persister = p
			applied := 0
			f.service.deps.SpotStateCommitted = func(commit gameworld.SpotStateCommit) error {
				require.True(t, f.w.Alive(f.target), "apply the durable manifest before despawn")
				require.Equal(t, p.commit, commit)
				applied++
				return nil
			}
			f.service.deps.Quarantine = func(handle types.Handle) {
				ecs.RemoveComponent[components.Collider](f.w, handle)
				ecs.MutateComponent(f.w, handle, func(info *components.EntityInfo) bool {
					info.Behaviors = nil
					return true
				})
			}

			// Sector targeting must still see the passable stages, without changes
			// to combat filtering or additional collider memberships.
			core.AttachColliderSpatial(f.w)
			attacker := f.w.Spawn(999, nil)
			ecs.AddComponent(f.w, attacker, components.Transform{X: 40, Y: 50})
			resolver, err := NewSectorResolver(f.w)
			require.NoError(t, err)
			hits, err := resolver.ResolveBoundedInto(attacker, 0, actiondefs.Sector{Range: 18, Angle: math.Pi}, make([]SectorHit, 0, MeleeMaxContacts))
			require.NoError(t, err)
			require.Len(t, hits, 1)
			require.Equal(t, f.target, hits[0].Handle)

			_, err = f.damage.Apply(f.target, 100)
			require.NoError(t, err)
			f.service.Update()
			f.chunks.runOne(t)
			require.Zero(t, applied, "the worker must not publish live spot state")
			require.True(t, f.w.Alive(f.target))
			require.Equal(t, []gameworld.WildWeedObjectDeletion{{SpotID: 123, EntityID: 1, RuntimeSeconds: 10}}, p.deletions)
			f.service.Update()
			require.Equal(t, 1, applied)
			require.False(t, f.w.Alive(f.target))
			require.Zero(t, f.service.PendingCount())

			// Reusing the same existing operation slot for an ordinary object
			// must not retain the weed's captured deletion or confirmation.
			ordinary := f.targetWithID(t, 2)
			_, err = f.damage.Apply(ordinary, 100)
			require.NoError(t, err)
			f.service.Update()
			f.chunks.runOne(t)
			f.service.Update()
			require.False(t, f.w.Alive(ordinary))
			require.Len(t, p.deletions, 1)
			require.Equal(t, 1, applied)
		})
	}
}

func TestWildWeedDestructionRejectsMissingAtomicCapabilityBeforeDamage(t *testing.T) {
	for _, withPersister := range []bool{false, true} {
		f := newDestructionServiceFixture(t)
		setLifecycleWildWeed(f.w, f.target, 123, 0)
		if withPersister {
			f.service.deps.Persister = &wildWeedDestructionFixture{destructionPersisterFixture: f.persister}
		} else {
			f.service.deps.SpotStateCommitted = func(gameworld.SpotStateCommit) error { return nil }
		}
		_, err := f.damage.Apply(f.target, 100)
		require.ErrorIs(t, err, ErrInvalidObjectDestructionService)
		health, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
		require.Equal(t, float64(100), health.HP)
		require.False(t, ecs.ObjectDestructionPending(f.w, f.target))
		require.Zero(t, f.service.PendingCount())
		require.Empty(t, f.chunks.jobs)
	}
}

func TestWildWeedDestructionRetryKeepsIdentityAndDefersPublication(t *testing.T) {
	f := newDestructionServiceFixture(t)
	setLifecycleWildWeed(f.w, f.target, 123, 3)
	p := &wildWeedDestructionFixture{destructionPersisterFixture: f.persister, fail: errors.New("uncertain commit"),
		commit: gameworld.SpotStateCommit{SpotID: 123, Snapshot: gameworld.SpotStateSnapshot{State: json.RawMessage(`{"www":{"objects":{}}}`), Revision: 2}}}
	f.service.deps.Persister = p
	applied := 0
	f.service.deps.SpotStateCommitted = func(gameworld.SpotStateCommit) error { applied++; return nil }
	_, err := f.damage.Apply(f.target, 100)
	require.NoError(t, err)
	f.service.Update()
	f.chunks.runOne(t)
	f.service.Update()
	require.Zero(t, applied)
	require.True(t, f.w.Alive(f.target))
	now := ecs.GetResource[ecs.TimeState](f.w)
	now.Now = now.Now.Add(2 * time.Second)
	now.RuntimeSecondsTotal = 999
	f.service.Update()
	f.chunks.runOne(t)
	f.service.Update()
	require.Len(t, p.deletions, 2)
	require.Equal(t, p.deletions[0], p.deletions[1])
	require.EqualValues(t, 10, p.deletions[1].RuntimeSeconds)
	require.Equal(t, 1, applied)
	require.False(t, f.w.Alive(f.target))
}

type wildWeedAdminPersisterFixture struct {
	*wildWeedDestructionFixture
	deletions []gameworld.WildWeedObjectDeletion
	commit    gameworld.SpotStateCommit
	err       error
	generic   []types.EntityID
}

func (p *wildWeedAdminPersisterFixture) DeleteObject(_ int, id types.EntityID) error {
	p.generic = append(p.generic, id)
	return p.err
}

func (p *wildWeedAdminPersisterFixture) DeleteWildWeedObject(_ context.Context, _ int, deletion gameworld.WildWeedObjectDeletion) (gameworld.SpotStateCommit, error) {
	p.deletions = append(p.deletions, deletion)
	return p.commit, p.err
}

func newWildWeedAdminFixture(t *testing.T) (*destructionServiceFixture, *wildWeedAdminPersisterFixture, wildWeedAdminDeleter) {
	t.Helper()
	f := newDestructionServiceFixture(t)
	setLifecycleWildWeed(f.w, f.target, 123, 1)
	p := &wildWeedAdminPersisterFixture{
		wildWeedDestructionFixture: &wildWeedDestructionFixture{destructionPersisterFixture: f.persister},
		commit:                     gameworld.SpotStateCommit{SpotID: 123},
	}
	f.service.deps.Persister = p
	f.service.deps.SpotStateCommitted = func(gameworld.SpotStateCommit) error { return nil }
	return f, p, wildWeedAdminDeleter{destruction: f.service, fallback: p}
}

func TestWildWeedAdminDeletionPreservesSynchronousSuccessAndFallback(t *testing.T) {
	f, p, deleter := newWildWeedAdminFixture(t)
	applied := 0
	f.service.deps.SpotStateCommitted = func(commit gameworld.SpotStateCommit) error {
		require.Equal(t, p.commit, commit)
		require.True(t, f.w.Alive(f.target))
		require.Equal(t, 1, f.chunks.pins[types.ChunkCoord{}])
		applied++
		return nil
	}
	require.NoError(t, deleter.DeleteObject(1, 1))
	require.Equal(t, 1, applied)
	require.True(t, f.w.Alive(f.target), "the synchronous admin caller performs cleanup")
	require.False(t, ecs.ObjectDestructionPending(f.w, f.target))
	require.Zero(t, f.service.PendingCount())
	require.Zero(t, f.chunks.pins[types.ChunkCoord{}])
	require.Empty(t, f.chunks.jobs)
	require.Empty(t, p.generic)
	require.Equal(t, []gameworld.WildWeedObjectDeletion{{SpotID: 123, EntityID: 1, RuntimeSeconds: 10}}, p.deletions)

	setLifecycleWildWeed(f.w, f.target, 0, 3)
	require.NoError(t, deleter.DeleteObject(1, 1))
	require.Equal(t, []types.EntityID{1}, p.generic)
	ordinary := f.targetWithID(t, 2)
	require.NoError(t, deleter.DeleteObject(1, 2))
	require.True(t, f.w.Alive(ordinary))
	require.Equal(t, []types.EntityID{1, 2}, p.generic)
	require.Equal(t, 1, applied)
}

func TestWildWeedAdminDeletionRetainsFailureUntilOwnerConfirms(t *testing.T) {
	for _, failApply := range []bool{false, true} {
		t.Run(map[bool]string{false: "uncertain_commit", true: "owner_apply"}[failApply], func(t *testing.T) {
			f, p, deleter := newWildWeedAdminFixture(t)
			failure := errors.New("uncertain commit")
			if !failApply {
				p.err = failure
			}
			applied := 0
			f.service.deps.SpotStateCommitted = func(commit gameworld.SpotStateCommit) error {
				require.True(t, f.w.Alive(f.target))
				require.Equal(t, p.commit, commit)
				applied++
				if failApply && applied == 1 {
					return failure
				}
				return nil
			}

			require.ErrorIs(t, deleter.DeleteObject(1, 1), failure)
			require.True(t, ecs.ObjectDestructionPending(f.w, f.target))
			require.True(t, f.w.Alive(f.target))
			_, colliderExists := ecs.GetComponent[components.Collider](f.w, f.target)
			require.False(t, colliderExists, "failed deletion must quarantine the source")
			require.Equal(t, 1, f.service.PendingCount())
			require.Equal(t, 1, f.chunks.pins[types.ChunkCoord{}])
			require.ErrorIs(t, deleter.DeleteObject(1, 1), ErrObjectDestructionPending)
			require.Len(t, p.deletions, 1)

			p.err = nil
			now := ecs.GetResource[ecs.TimeState](f.w)
			now.RuntimeSecondsTotal = 999
			f.service.Update()
			beforeWorker := applied
			f.chunks.runOne(t)
			require.Equal(t, beforeWorker, applied, "worker must not mutate spot state")
			require.True(t, f.w.Alive(f.target))
			f.service.Update()
			require.False(t, f.w.Alive(f.target))
			require.Zero(t, f.service.PendingCount())
			require.Zero(t, f.chunks.pins[types.ChunkCoord{}])
			require.Empty(t, f.chunks.inserted, "admin deletion must not create loot")
			require.Empty(t, f.persister.calls)
			require.Zero(t, f.ids.calls)
			if failApply {
				require.Len(t, p.deletions, 1, "known commit must only retry owner application")
				require.Equal(t, 2, applied)
			} else {
				require.Equal(t, 1, applied)
				require.Len(t, p.deletions, 2)
				require.Equal(t, p.deletions[0], p.deletions[1], "retry must preserve captured identity and runtime")
			}

			// The same slot must resume the ordinary loot path after release.
			ordinary := f.targetWithID(t, 2)
			f.addContainer(2, 0, lootItem(20, 1, 1))
			_, err := f.damage.Apply(ordinary, 100)
			require.NoError(t, err)
			f.service.Update()
			f.chunks.runOne(t)
			f.service.Update()
			require.False(t, f.w.Alive(ordinary))
			require.Len(t, f.chunks.inserted, 1)
			require.Len(t, f.persister.calls, 1)
		})
	}
}

func TestWildWeedAdminDeletionRejectsBeforeDatabaseWhenAdmissionUnavailable(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*destructionServiceFixture)
		err   error
	}{
		{name: "full", setup: func(f *destructionServiceFixture) { f.service.freeCount = 0 }, err: ErrObjectDestructionQueueFull},
		{name: "stopped", setup: func(f *destructionServiceFixture) { f.service.StopAdmission() }, err: ErrObjectDestructionStopped},
		{name: "busy_chunk", setup: func(f *destructionServiceFixture) { f.chunks.pinErr = gameworld.ErrChunkPersistenceBusy }, err: gameworld.ErrChunkPersistenceBusy},
		{name: "missing_capability", setup: func(f *destructionServiceFixture) { f.service.deps.Persister = f.persister }, err: ErrInvalidObjectDestructionService},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, p, deleter := newWildWeedAdminFixture(t)
			test.setup(f)
			require.ErrorIs(t, deleter.DeleteObject(1, 1), test.err)
			require.Empty(t, p.deletions)
			require.Empty(t, f.chunks.jobs)
			require.Zero(t, f.service.PendingCount())
			require.False(t, ecs.ObjectDestructionPending(f.w, f.target))
			_, colliderExists := ecs.GetComponent[components.Collider](f.w, f.target)
			require.True(t, colliderExists)
			require.Zero(t, f.chunks.pins[types.ChunkCoord{}])
		})
	}
}
