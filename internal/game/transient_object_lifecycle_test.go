package game

import (
	"context"
	"testing"
	"time"

	"origin/internal/config"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	gameworld "origin/internal/game/world"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type npcQuarantineRetirementProbe func(types.Handle, contracts.NPCRetireReason)

func (probe npcQuarantineRetirementProbe) Retire(h types.Handle, reason contracts.NPCRetireReason) {
	probe(h, reason)
}

func TestTransientDamageDoesNotReservePersistenceAndFinalizesAfterCommit(t *testing.T) {
	f := newDestructionServiceFixture(t)
	ecs.WithComponent(f.w, f.target, func(info *components.EntityInfo) { info.Transient = true })
	f.service.freeCount = 0 // Durable queue capacity must not constrain wildlife.
	f.chunks.accept = false
	plan, err := f.damage.prepareDamage(f.target, 100)
	require.NoError(t, err)
	require.NoError(t, f.damage.reserveDamage(&plan))
	require.True(t, plan.transient)
	require.Zero(t, f.service.PendingCount())
	require.Empty(t, f.chunks.pins)
	require.True(t, f.w.Alive(f.target))
	require.False(t, ecs.ObjectDestructionPending(f.w, f.target))
	f.damage.commitDamage(plan)
	health, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
	require.Zero(t, health.HP)
	require.True(t, ecs.ObjectDestructionPending(f.w, f.target))
	require.True(t, f.w.Alive(f.target))
	f.damage.finalizeDamage(plan)
	require.False(t, f.w.Alive(f.target))
	require.False(t, ecs.ObjectDestructionPending(f.w, f.target))
	f.service.Update()
	require.Empty(t, f.chunks.jobs)
	require.Empty(t, f.chunks.removed)
	require.Empty(t, f.persister.calls)
}

func TestTransientDamagePreflightDoesNotAllocate(t *testing.T) {
	f := newObjectDamageFixture(t, nil)
	ecs.WithComponent(f.w, f.target, func(info *components.EntityInfo) { info.Transient = true })
	var prepareErr, reserveErr error
	allocs := testing.AllocsPerRun(1000, func() {
		var plan objectDamageCommit
		plan, prepareErr = f.service.prepareDamage(f.target, 100)
		reserveErr = f.service.reserveDamage(&plan)
		f.service.abortDamage(plan)
	})
	require.NoError(t, prepareErr)
	require.NoError(t, reserveErr)
	require.Zero(t, allocs)
	require.Zero(t, f.destruction.PendingCount())
	require.Equal(t, ObjectDestructionQueueCapacity, f.destruction.freeCount)
}

func TestTransientRemovalRejectsInventoriesWithoutMutation(t *testing.T) {
	f := newDestructionServiceFixture(t)
	ecs.WithComponent(f.w, f.target, func(info *components.EntityInfo) { info.Transient = true })
	root := f.addContainer(1, 0, lootItem(100, 1, 2))
	_, err := f.damage.Apply(f.target, 100)
	require.ErrorIs(t, err, ErrInvalidTransientRemoval)
	require.ErrorIs(t, f.service.RemoveTransient(f.target), ErrInvalidTransientRemoval)
	health, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
	require.Equal(t, 100.0, health.HP)
	require.True(t, f.w.Alive(root))
	require.False(t, ecs.ObjectDestructionPending(f.w, f.target))
	require.Empty(t, f.chunks.pins)
	require.Zero(t, f.service.PendingCount())
}

func TestAdminDestroyTransientUsesSharedRemovalWithoutDatabaseDeleter(t *testing.T) {
	f := newDestructionServiceFixture(t)
	ecs.WithComponent(f.w, f.target, func(info *components.EntityInfo) { info.Transient = true })
	chat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}
	handler := NewChatAdminCommandHandler(nil, nil, chat, nil, nil, nil, nil, nil, nil, zap.NewNop())
	handler.SetTransientObjectRemover(f.service.RemoveTransient)
	ecs.GetResource[ecs.PendingAdminDestroy](f.w).Set(2000)
	handler.ExecutePendingDestroy(f.w, 2000, 1)
	require.Equal(t, "Destroyed object 1.", chat.messages[2000])
	require.False(t, f.w.Alive(f.target))
	require.False(t, ecs.GetResource[ecs.PendingAdminDestroy](f.w).Get(2000))
	require.Zero(t, f.service.PendingCount())
	require.Empty(t, f.chunks.pins)
	require.Empty(t, f.chunks.removed)
	require.Empty(t, f.persister.calls)
}

func TestMeleeSweepTransientAndPersistentLethalsCommitAllHealthBeforeRemoval(t *testing.T) {
	f := newMeleeFixture(t)
	creature := f.creature(t, 10, 55, 50, false)
	transient := f.object(t, 20, 60, 50, 2)
	persistent := f.object(t, 30, 62, 50, 2)
	ecs.WithComponent(f.world, transient, func(info *components.EntityInfo) { info.Transient = true })
	f.objects.destruction.deps.Quarantine = func(h types.Handle) {
		health, _ := ecs.GetComponent[components.EntityHealth](f.world, creature)
		require.Equal(t, 19.0, health.SHP)
		state, _ := ecs.GetComponent[components.ObjectInternalState](f.world, persistent)
		require.Zero(t, state.HP)
		require.True(t, f.objects.state.Pending[persistent])
		if h == transient {
			state, _ = ecs.GetComponent[components.ObjectInternalState](f.world, transient)
			require.Zero(t, state.HP)
			require.True(t, f.objects.state.Pending[transient])
		}
		require.Equal(t, "idle", f.actions.State(f.world, f.owner).Phase)
		f.quarantined = append(f.quarantined, h)
	}
	f.start("axe_sweep", 0)
	f.finish()
	require.False(t, f.world.Alive(transient))
	require.True(t, f.world.Alive(persistent), "durable removal must still wait for persistence")
	require.Len(t, f.sender.attacks, 1)
	require.Len(t, f.sender.attacks[0].hits, 3)
	require.Equal(t, 1, f.chunks.calls)
	require.Equal(t, 1, f.chunks.pins)
	require.Equal(t, 1, f.objects.destruction.PendingCount())
	require.Equal(t, []types.Handle{transient, persistent}, f.quarantined)
}

func TestMeleeSweepTransientSurvivesLaterPersistentReservationFailure(t *testing.T) {
	f := newMeleeFixture(t)
	transient := f.object(t, 20, 60, 50, 2)
	persistent := f.object(t, 30, 62, 50, 2)
	ecs.WithComponent(f.world, transient, func(info *components.EntityInfo) { info.Transient = true })
	f.chunks.failAt = 1
	f.start("axe_sweep", 0)
	stamina := f.stamina()
	f.finish()
	for _, target := range []types.Handle{transient, persistent} {
		require.True(t, f.world.Alive(target))
		health, _ := ecs.GetComponent[components.ObjectInternalState](f.world, target)
		require.Equal(t, 2.0, health.HP)
		require.False(t, f.objects.state.Pending[target])
	}
	require.Equal(t, stamina, f.stamina())
	require.Empty(t, f.sender.attacks)
	require.Empty(t, f.quarantined)
	require.Zero(t, f.chunks.pins)
	require.Zero(t, f.objects.destruction.PendingCount())
}

func TestTransientRemovalUsesShardQuarantineForMovementReferencesAndVisibility(t *testing.T) {
	for _, admin := range []bool{false, true} {
		t.Run(map[bool]string{false: "damage", true: "admin"}[admin], func(t *testing.T) {
			f := newMeleeFixture(t)
			w := f.world
			target := f.object(t, 20, 60, 50, 2)
			ecs.WithComponent(w, target, func(info *components.EntityInfo) { info.Transient = true })
			ecs.AddComponent(w, target, components.Movement{State: constt.StateMoving, TargetType: constt.TargetPoint, TargetX: 100, TargetY: 50, VelocityX: 10})
			cfg := &config.Config{Game: config.GameConfig{ChunkLRUCapacity: 1, ChunkLRUTTL: 60, LoadWorkers: 1, WorldWidthChunks: 1, WorldHeightChunks: 1}}
			chunks := gameworld.NewChunkManager(cfg, nil, w, nil, 0, 1, nil, nil, nil, zap.NewNop())
			t.Cleanup(chunks.Stop)
			chunks.RegisterEntity(1, 50, 50, false)
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			require.NoError(t, chunks.WaitPreloaded(ctx, types.ChunkCoord{}))
			chunks.Update(0)
			chunk := chunks.GetChunkFast(types.ChunkCoord{})
			require.NotNil(t, chunk)
			chunk.Spatial().AddDynamic(target, 60, 50)
			s := &Shard{world: w, chunkManager: chunks, actionService: f.actions}
			retired := false
			ecs.InitResource(w, contracts.NPCLifecycle{Controller: npcQuarantineRetirementProbe(func(handle types.Handle, reason contracts.NPCRetireReason) {
				require.Equal(t, target, handle)
				require.Equal(t, contracts.NPCRetireQuarantine, reason)
				require.True(t, ecs.ObjectDestructionPending(w, handle), "retirement must work after lethal damage commits")
				require.True(t, ecs.HasComponent[components.Movement](w, handle), "cancel execution before removing its runtime components")
				retired = true
			})})
			f.objects.destruction.deps.Quarantine = func(handle types.Handle) {
				s.quarantineDestroyedObject(handle)
				require.False(t, ecs.HasComponent[components.Movement](w, handle), "quarantine must stop movement before despawn")
			}
			ecs.AddComponent(w, f.owner, components.Movement{State: constt.StateMoving, TargetType: constt.TargetEntity, TargetHandle: target, VelocityX: 10})
			ecs.AddComponent(w, f.owner, components.PendingContextAction{TargetEntityID: 20, TargetHandle: target, ActionID: "open"})
			links := ecs.GetResource[ecs.LinkState](w)
			links.SetIntent(1, 20, target, time.Now())
			require.True(t, ecs.ScheduleBehaviorTick(w, 20, "npc_ai", 100))
			visibility := ecs.GetResource[ecs.VisibilityState](w)
			visibility.ObserversByVisibleTarget[target] = map[types.Handle]struct{}{f.owner: {}}
			visibility.VisibleByObserver[f.owner] = ecs.ObserverVisibility{Known: map[types.Handle]types.EntityID{target: 20}}
			if admin {
				chat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}
				handler := NewChatAdminCommandHandler(nil, nil, chat, nil, nil, nil, nil, nil, nil, zap.NewNop())
				handler.SetTransientObjectRemover(f.objects.destruction.RemoveTransient)
				ecs.GetResource[ecs.PendingAdminDestroy](w).Set(1)
				handler.ExecutePendingDestroy(w, 1, 20)
				require.Equal(t, "Destroyed object 20.", chat.messages[1])
			} else {
				_, err := f.objects.Apply(target, 100)
				require.NoError(t, err)
			}
			require.False(t, w.Alive(target))
			require.True(t, retired)
			require.Zero(t, chunk.Spatial().DynamicCount())
			require.Empty(t, chunk.GetDeletedObjectIDs())
			require.False(t, chunk.IsDirty(w))
			require.False(t, ecs.HasComponent[components.PendingContextAction](w, f.owner))
			movement, _ := ecs.GetComponent[components.Movement](w, f.owner)
			require.Equal(t, constt.TargetNone, movement.TargetType)
			require.Equal(t, constt.StateIdle, movement.State)
			require.Empty(t, links.IntentByPlayer)
			require.Empty(t, visibility.ObserversByVisibleTarget)
			require.Empty(t, visibility.VisibleByObserver[f.owner].Known)
			require.Zero(t, ecs.GetResource[ecs.BehaviorTickSchedule](w).PendingCount())
			require.Zero(t, f.chunks.calls)
			require.Zero(t, f.objects.destruction.PendingCount())
		})
	}
}
