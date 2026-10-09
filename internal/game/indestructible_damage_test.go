package game

import (
	"maps"
	"math"
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestIndestructibleObjectRejectsDamageWithoutMutationOrAllocations(t *testing.T) {
	for _, prepared := range []bool{false, true} {
		name := "unprepared"
		if prepared {
			name = "prepared"
		}
		t.Run(name, func(t *testing.T) {
			f := newObjectDamageFixture(t, nil)
			if !prepared {
				f.target = f.w.Spawn(2, func(w *ecs.World, target types.Handle) {
					ecs.AddComponent(w, target, components.EntityInfo{TypeID: 99, Region: 1})
					ecs.AddComponent(w, target, components.Transform{X: 50, Y: 50})
					ecs.AddComponent(w, target, components.ChunkRef{})
					ecs.AddComponent(w, target, components.ObjectInternalState{HP: 100, HasHP: true})
				})
			}
			ecs.WithComponent(f.w, f.target, func(info *components.EntityInfo) { info.Indestructible = true })
			before := f.hp()
			pending := maps.Clone(f.service.state.Pending)
			freeSlots := f.destruction.freeCount
			writes := 0
			f.w.AddComponentObserver(components.ObjectInternalStateComponentID, func(types.Handle) { writes++ })
			for _, draw := range []float64{0, .49, 100, math.MaxFloat64} {
				result, err := f.service.Apply(f.target, draw)
				require.ErrorIs(t, err, ErrObjectDamageTargetIndestructible)
				require.Zero(t, result)
				commit, err := f.service.prepareDamage(f.target, draw)
				require.ErrorIs(t, err, ErrObjectDamageTargetIndestructible)
				require.Zero(t, commit)
			}
			require.ErrorIs(t, f.service.PrepareTarget(f.target), ErrObjectDamageTargetIndestructible)
			var got error
			require.Zero(t, testing.AllocsPerRun(1000, func() {
				_, got = f.service.Apply(f.target, math.MaxFloat64)
			}))
			require.ErrorIs(t, got, ErrObjectDamageTargetIndestructible)
			require.Equal(t, before, f.hp())
			require.Zero(t, writes)
			require.Zero(t, f.destruction.PendingCount())
			require.Equal(t, pending, f.service.state.Pending)
			require.Equal(t, prepared, f.service.state.Prepared[f.target])
			require.Equal(t, freeSlots, f.destruction.freeCount)
			require.Zero(t, f.quarantines)

			// A destination definition can make the same live object damageable again.
			ecs.WithComponent(f.w, f.target, func(info *components.EntityInfo) { info.Indestructible = false })
			require.NoError(t, f.service.PrepareTarget(f.target))
			result, err := f.service.Apply(f.target, .5)
			require.NoError(t, err)
			require.Equal(t, 99.5, result.AfterHP)
			require.Equal(t, 1, writes)
		})
	}
}

func TestMeleeSkipsIndestructibleObjectsBeforeHitSelection(t *testing.T) {
	for _, action := range []string{"axe_strike", "axe_sweep"} {
		t.Run(action, func(t *testing.T) {
			f := newMeleeFixture(t)
			immune := f.object(t, 3, 54, 50, 1)
			ordinary := f.object(t, 4, 60, 50, 100)
			f.creature(t, 5, 62, 50, false)
			f.start(action, 0)
			// Eligibility is checked at completion, after any definition refresh.
			ecs.WithComponent(f.world, immune, func(info *components.EntityInfo) { info.Indestructible = true })
			before, _ := ecs.GetComponent[components.ObjectInternalState](f.world, immune)
			pending := maps.Clone(f.objects.state.Pending)
			f.finish()
			require.Len(t, f.sender.attacks, 1)
			var ids []uint64
			for _, hit := range f.sender.attacks[0].hits {
				ids = append(ids, hit.id)
			}
			want := []uint64{4}
			if action == "axe_sweep" {
				want = append(want, 5)
			}
			require.Equal(t, want, ids)
			after, _ := ecs.GetComponent[components.ObjectInternalState](f.world, immune)
			require.Equal(t, before, after)
			ordinaryHP, _ := ecs.GetComponent[components.ObjectInternalState](f.world, ordinary)
			require.Less(t, ordinaryHP.HP, 100.0)
			require.Empty(t, f.quarantined)
			require.Equal(t, pending, f.objects.state.Pending)
			require.Zero(t, f.chunks.calls)
			require.Equal(t, 940.0, f.stamina())
		})
	}
}

func TestMeleeOnlyIndestructibleTargetsIsOrdinaryMiss(t *testing.T) {
	for _, action := range []string{"axe_strike", "axe_sweep"} {
		t.Run(action, func(t *testing.T) {
			f := newMeleeFixture(t)
			immune := f.object(t, 3, 54, 50, 1)
			ecs.WithComponent(f.world, immune, func(info *components.EntityInfo) { info.Indestructible = true })
			delete(f.objects.state.Prepared, immune)
			before, _ := ecs.GetComponent[components.ObjectInternalState](f.world, immune)
			pending := maps.Clone(f.objects.state.Pending)
			definition, _ := f.definitions.Get(action)
			prepared, err := f.catalog.PrepareMeleeAction(definition)
			require.NoError(t, err)
			var got error
			require.Zero(t, testing.AllocsPerRun(100, func() {
				got = f.execution.prepare(f.owner, 1, definition, prepared, 0)
				f.execution.abort()
			}))
			require.NoError(t, got)
			f.start(action, 0)
			f.finish()
			require.Len(t, f.sender.attacks, 1)
			require.Empty(t, f.sender.attacks[0].hits)
			require.Equal(t, 940.0, f.stamina())
			require.Equal(t, "idle", f.actions.State(f.world, f.owner).Phase)
			require.Equal(t, int64(3600), f.actions.State(f.world, f.owner).Cooldowns[0].ExpiresAtMs)
			after, _ := ecs.GetComponent[components.ObjectInternalState](f.world, immune)
			require.Equal(t, before, after)
			require.Equal(t, pending, f.objects.state.Pending)
			require.Empty(t, f.quarantined)
			require.Zero(t, f.chunks.calls)
		})
	}
}
