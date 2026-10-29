package game

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"origin/internal/actiondefs"
	"origin/internal/combatpresentation"
	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"
)

type biteTestChunks map[types.ChunkCoord]*core.Chunk

func (c biteTestChunks) GetChunkFast(coord types.ChunkCoord) *core.Chunk { return c[coord] }

type biteHealthPolicy struct{}

func (biteHealthPolicy) NPCProfileParameters()                {}
func (biteHealthPolicy) MinimumCombatHealthFraction() float64 { return .25 }

type biteFixture struct {
	*meleeFixture
	executor *NPCAbilityExecutor
	actor    contracts.EntityRef
	target   contracts.EntityRef
	chunk    *core.Chunk
}

func newBiteFixture(t *testing.T) *biteFixture {
	t.Helper()
	f := newMeleeFixture(t)
	previous := objectdefs.Global()
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 99, Key: "bite_actor", HP: 110, Behaviors: map[string]json.RawMessage{"npc_ai": json.RawMessage(`{}`)}, NPCConfig: &contracts.NPCBehaviorConfig{Abilities: []string{"fox_bite"}, Parameters: biteHealthPolicy{}}}}))
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, constt.ChunkSize)
	tiles := make([]byte, constt.ChunkSize*constt.ChunkSize)
	for i := range tiles {
		tiles[i] = types.TileGrass
	}
	require.NoError(t, chunk.RestoreTiles(tiles, 0, 0))
	chunk.SetState(types.ChunkStateActive)
	combatpresentation.Attach(f.world, nil)
	executor, err := NewNPCAbilityExecutor(f.world, f.actions, biteTestChunks{{}: chunk})
	require.NoError(t, err)
	actor := f.object(t, 20, 40, 50, 110)
	target := contracts.EntityRef{Handle: f.owner, EntityID: 1}
	ecs.AddComponent(f.world, actor, components.NPCBrain{Target: target, DamageSource: target})
	ecs.AddComponent(f.world, actor, components.Movement{State: constt.StateMoving, TargetType: constt.TargetPoint, TargetX: 100, TargetY: 100})
	ecs.WithComponent(f.world, actor, func(c *components.Collider) { c.Layer = 1; c.Mask = 1 })
	ecs.GetResource[ecs.TimeState](f.world).Delta = .1
	ecs.GetResource[ecs.TimeState](f.world).TickPeriod = 100 * time.Millisecond
	return &biteFixture{meleeFixture: f, executor: executor, actor: contracts.EntityRef{Handle: actor, EntityID: 20}, target: target, chunk: chunk}
}
func (f *biteFixture) request(generation uint64) contracts.NPCAbilityRequest {
	return contracts.NPCAbilityRequest{Actor: f.actor, AbilityID: "fox_bite", Generation: generation, Target: contracts.NPCTarget{Kind: contracts.NPCTargetEntity, Entity: f.target}}
}
func (f *biteFixture) startBite(t *testing.T, generation uint64) {
	t.Helper()
	state := f.executor.TryStart(f.request(generation))
	require.Equal(t, contracts.NPCExecutionAccepted, state.Status)
	require.True(t, state.Running())
	ecs.WithComponent(f.world, f.actor.Handle, func(brain *components.NPCBrain) {
		brain.MovementLease = components.NPCMovementLease{Owner: components.NPCMovementAbility, Generation: generation}
	})
}
func (f *biteFixture) finishBite() {
	for i := 0; i < 4; i++ {
		f.tick()
	}
}

func TestNPCBiteUsesSharedCycleDamageAndCooldown(t *testing.T) {
	f := newBiteFixture(t)
	bystander := f.creature(t, 3, 48, 50, false)
	object := f.object(t, 4, 47, 50, 100)
	beforeStats, _ := ecs.GetComponent[components.EntityStats](f.world, f.owner)
	f.startBite(t, 7)
	for _, check := range []bool{ecs.HasComponent[components.CharacterProfile](f.world, f.actor.Handle), ecs.HasComponent[components.EntityStats](f.world, f.actor.Handle), ecs.HasComponent[components.EntityHealth](f.world, f.actor.Handle)} {
		require.False(t, check)
	}
	active, _ := ecs.GetComponent[components.ActiveGameAction](f.world, f.actor.Handle)
	cycle, _ := ecs.GetComponent[components.ActiveCyclicAction](f.world, f.actor.Handle)
	require.Equal(t, uint64(7), active.Generation)
	require.Equal(t, uint64(7), cycle.ActionGeneration)
	require.Equal(t, uint32(4), cycle.CycleDurationTicks)
	movement, _ := ecs.GetComponent[components.Movement](f.world, f.actor.Handle)
	require.Equal(t, constt.StateIdle, movement.State)
	require.True(t, combatpresentation.Snapshot(f.world, f.actor.Handle).Active)
	// The admission tick is not counted toward a 400ms windup.
	f.cyclic.Update(f.world, .1)
	require.Empty(t, f.sender.attacks)
	for i := 0; i < 3; i++ {
		f.tick()
		require.Empty(t, f.sender.attacks)
	}
	f.tick()
	require.Equal(t, contracts.NPCExecutionCompleted, f.executor.State(f.actor, 7).Status)
	require.False(t, combatpresentation.Snapshot(f.world, f.actor.Handle).Active)
	require.False(t, ecs.HasComponent[components.ActiveGameAction](f.world, f.actor.Handle))
	require.False(t, ecs.HasComponent[components.ActiveCyclicAction](f.world, f.actor.Handle))
	require.Len(t, f.sender.attacks, 1)
	require.Equal(t, []meleeHitSample{{id: 1, damage: 6}}, f.sender.attacks[0].hits)
	health, _ := ecs.GetComponent[components.EntityHealth](f.world, bystander)
	require.Equal(t, 25.0, health.SHP)
	state, _ := ecs.GetComponent[components.ObjectInternalState](f.world, object)
	require.Equal(t, 100.0, state.HP)
	afterStats, _ := ecs.GetComponent[components.EntityStats](f.world, f.owner)
	require.Equal(t, beforeStats, afterStats)
	cooldowns, _ := ecs.GetComponent[components.ActionCooldowns](f.world, f.actor.Handle)
	require.Equal(t, int64(1400), cooldowns.ByAction["fox_bite"].StartedAtMs)
	require.Equal(t, int64(2900), cooldowns.ByAction["fox_bite"].ExpiresAtMs)
	require.Empty(t, f.sender.states)
	require.Empty(t, f.sender.progress)
	require.Empty(t, f.sender.finished)
	require.Equal(t, contracts.NPCExecutionBusy, f.executor.TryStart(f.request(8)).Status)
	f.world.Despawn(f.owner)
	require.Equal(t, contracts.NPCExecutionCompleted, f.executor.State(f.actor, 7).Status, "terminal result survives target retirement")
}

func TestNPCBiteMissKeepsOriginalAimAndPaysCooldown(t *testing.T) {
	for _, scenario := range []string{"moved_out", "new_attacker", "wall_after_start"} {
		t.Run(scenario, func(t *testing.T) {
			f := newBiteFixture(t)
			f.startBite(t, 1)
			switch scenario {
			case "moved_out":
				ecs.WithComponent(f.world, f.owner, func(tr *components.Transform) { tr.X = 40; tr.Y = 60 })
			case "new_attacker":
				other := f.creature(t, 3, 45, 50, false)
				ecs.WithComponent(f.world, f.actor.Handle, func(brain *components.NPCBrain) {
					brain.DamageSource = contracts.EntityRef{Handle: other, EntityID: 3}
					brain.Target = brain.DamageSource
				})
				ecs.WithComponent(f.world, f.actor.Handle, func(tr *components.Transform) { tr.Direction = math.Pi })
			case "wall_after_start":
				wall := f.object(t, 3, 45, 50, 100)
				ecs.WithComponent(f.world, wall, func(c *components.Collider) { c.Layer = 1; c.Mask = 1 })
			}
			f.finishBite()
			require.Equal(t, contracts.NPCExecutionCompleted, f.executor.State(f.actor, 1).Status)
			require.True(t, f.actions.isOnCooldown(f.world, f.actor.Handle, "fox_bite"))
			require.Len(t, f.sender.attacks, 1)
			if scenario == "new_attacker" {
				require.Equal(t, []meleeHitSample{{id: 1, damage: 6}}, f.sender.attacks[0].hits)
			} else {
				require.Empty(t, f.sender.attacks[0].hits)
			}
		})
	}
}

func TestNPCBitePerTickEligibilityAndLifecycle(t *testing.T) {
	for _, scenario := range []string{"quarter_hp", "ko_target", "lying_target", "dead_target", "target_reuse", "transformed_actor", "quarantine", "unload", "brain_removed"} {
		t.Run(scenario, func(t *testing.T) {
			f := newBiteFixture(t)
			f.startBite(t, 11)
			cycle, _ := ecs.GetComponent[components.ActiveCyclicAction](f.world, f.actor.Handle)
			switch scenario {
			case "quarter_hp":
				ecs.WithComponent(f.world, f.actor.Handle, func(hp *components.ObjectInternalState) { hp.HP = 27.5 })
			case "ko_target":
				ecs.WithComponent(f.world, f.owner, func(hp *components.EntityHealth) { hp.KOUntilUnixMs = 60000 })
			case "lying_target":
				ecs.WithComponent(f.world, f.owner, func(hp *components.EntityHealth) { hp.IsLying = true })
			case "dead_target":
				ecs.WithComponent(f.world, f.owner, func(hp *components.EntityHealth) { hp.SHP = 0; hp.HHP = 0 })
			case "target_reuse":
				f.world.Despawn(f.owner)
				replacement := f.creature(t, 1, 50, 50, false)
				require.NotEqual(t, f.owner, replacement)
			case "transformed_actor":
				ecs.WithComponent(f.world, f.actor.Handle, func(info *components.EntityInfo) { info.TypeID = 100 })
			case "quarantine":
				ecs.GetResource[ecs.ObjectDestructionState](f.world).Pending[f.actor.Handle] = true
			case "unload":
				f.world.Despawn(f.actor.Handle)
			case "brain_removed":
				ecs.RemoveComponent[components.NPCBrain](f.world, f.actor.Handle)
			}
			f.tick()
			f.actions.AdvanceCycle(f.world, 20, f.actor.Handle, cycle, nil)
			require.Empty(t, f.sender.attacks)
			require.False(t, ecs.HasComponent[components.ActiveGameAction](f.world, f.actor.Handle))
			require.False(t, f.actions.isOnCooldown(f.world, f.actor.Handle, "fox_bite"))
			if scenario == "unload" || scenario == "brain_removed" {
				require.Empty(t, f.executor.records)
			} else {
				require.Equal(t, contracts.NPCExecutionCanceled, f.executor.State(f.actor, 11).Status)
			}
		})
	}
}

func TestNPCBiteOrdinaryDamageAndStaleGeneration(t *testing.T) {
	f := newBiteFixture(t)
	f.startBite(t, 2)
	_, err := f.objects.Apply(f.actor.Handle, 1)
	require.NoError(t, err)
	f.executor.Cancel(f.actor, 1, contracts.NPCRetireRequested)
	require.True(t, f.executor.State(f.actor, 2).Running())
	f.finishBite()
	require.Len(t, f.sender.attacks, 1)
	require.Equal(t, contracts.NPCExecutionCompleted, f.executor.TryStart(f.request(2)).Status, "same request cannot replay damage")
	ecs.GetResource[ecs.TimeState](f.world).UnixMs = 2900
	f.startBite(t, 3)
	f.executor.Cancel(f.actor, 2, contracts.NPCRetireRequested)
	f.executor.Cancel(f.actor, 2, contracts.NPCRetireTransform)
	f.executor.Cancel(contracts.EntityRef{Handle: f.actor.Handle, EntityID: 999}, 3, contracts.NPCRetireQuarantine)
	require.True(t, f.executor.State(f.actor, 3).Running())
	f.executor.Cancel(f.actor, 3, contracts.NPCRetireRequested)
	require.Equal(t, contracts.NPCExecutionCanceled, f.executor.State(f.actor, 3).Status)
	require.False(t, f.actions.isOnCooldown(f.world, f.actor.Handle, "fox_bite"))
	f.finishBite()
	require.Len(t, f.sender.attacks, 1)
}

func TestNPCBiteAdmissionAndObstruction(t *testing.T) {
	for _, scenario := range []string{"peaceful_target", "wall", "terrain", "unloaded_chunk", "far", "low_hp", "bad_identity"} {
		t.Run(scenario, func(t *testing.T) {
			f := newBiteFixture(t)
			request := f.request(1)
			switch scenario {
			case "peaceful_target":
				ecs.WithComponent(f.world, f.actor.Handle, func(brain *components.NPCBrain) { brain.DamageSource = contracts.EntityRef{} })
			case "wall":
				wall := f.object(t, 3, 45, 50, 100)
				ecs.WithComponent(f.world, wall, func(c *components.Collider) { c.Layer = 1; c.Mask = 1 })
			case "terrain":
				f.chunk.SetTile(3, 4, types.TileDeepWater)
			case "unloaded_chunk":
				f.chunk.SetState(types.ChunkStateInactive)
			case "far":
				ecs.WithComponent(f.world, f.owner, func(tr *components.Transform) { tr.X = 100 })
			case "low_hp":
				ecs.WithComponent(f.world, f.actor.Handle, func(hp *components.ObjectInternalState) { hp.HP = 27.5 })
			case "bad_identity":
				request.Actor.EntityID = 21
			}
			state := f.executor.TryStart(request)
			require.False(t, state.Running())
			require.False(t, ecs.HasComponent[components.ActiveCyclicAction](f.world, f.actor.Handle))
			require.Empty(t, f.executor.records)
		})
	}
}

func TestNPCBiteClientCannotActivateOrDiscover(t *testing.T) {
	f := newBiteFixture(t)
	f.start("axe_sweep", 0)
	original, _ := ecs.GetComponent[components.ActiveGameAction](f.world, f.owner)
	f.actions.ActivateRequest(f.world, 1, f.owner, &netproto.C2S_ActivateAction{ActionId: "fox_bite"})
	f.actions.StartTargetedOnce(f.world, 1, f.owner, "fox_bite", 20, f.actor.Handle, 40, 50)
	after, _ := ecs.GetComponent[components.ActiveGameAction](f.world, f.owner)
	require.Equal(t, original, after)
	f.actions.SendList(1)
	for _, def := range f.sender.lists[0].Actions {
		require.NotEqual(t, "fox_bite", def.Id)
	}
	require.Empty(t, f.executor.records)
}

func TestNPCBiteWindupUsesTickRate(t *testing.T) {
	f := newBiteFixture(t)
	definition, _ := f.definitions.Get("fox_bite")
	ecs.GetResource[ecs.TimeState](f.world).Delta = .05
	require.Equal(t, uint32(8), actionDurationTicks(f.world, definition))
	ecs.GetResource[ecs.TimeState](f.world).Delta = .15
	require.Equal(t, uint32(3), actionDurationTicks(f.world, definition))
}

func TestMeleeDamageObserverRunsAfterWholeBatchAndSkipsDead(t *testing.T) {
	f := newBiteFixture(t)
	ecs.WithComponent(f.world, f.actor.Handle, func(tr *components.Transform) { tr.X = 60 })
	survivor := f.object(t, 21, 62, 50, 100)
	dead := f.object(t, 22, 63, 50, 1)
	count := 0
	f.execution.SetDamageObserver(func(target, source contracts.EntityRef) {
		require.Equal(t, contracts.EntityRef{Handle: f.owner, EntityID: 1}, source)
		require.NotEqual(t, dead, target.Handle)
		health, _ := ecs.GetComponent[components.ObjectInternalState](f.world, survivor)
		require.Less(t, health.HP, 100.0)
		deadHealth, _ := ecs.GetComponent[components.ObjectInternalState](f.world, dead)
		require.Zero(t, deadHealth.HP)
		require.True(t, ecs.ObjectDestructionPending(f.world, dead))
		count++
	})
	f.start("axe_sweep", 0)
	f.finish()
	require.Equal(t, 2, count)
}

func TestNaturalMeleeCompilerRejectsMixedSources(t *testing.T) {
	f := newBiteFixture(t)
	definition, _ := f.definitions.Get("fox_bite")
	for _, change := range []func(*actiondefs.Definition){
		func(d *actiondefs.Definition) { d.Audience = actiondefs.AudiencePlayer },
		func(d *actiondefs.Definition) { d.Combat.DamageMultiplier = 1 },
		func(d *actiondefs.Definition) {
			d.Requirements.Equipment = []actiondefs.EquipmentRequirement{{DamageSource: true}}
		},
		func(d *actiondefs.Definition) { d.Execution.Stamina = 1 },
	} {
		copy := *definition
		combat := *definition.Combat
		copy.Combat = &combat
		change(&copy)
		_, err := f.catalog.PrepareMeleeAction(&copy)
		require.Error(t, err)
	}
}

func TestNPCBiteReadsArmorAtImpactAndAvailabilityIsReadOnly(t *testing.T) {
	f := newBiteFixture(t)
	request := f.request(1)
	ready := f.executor.Availability(f.actor, "fox_bite", request.Target)
	require.True(t, ready.Ready)
	require.Equal(t, 12.0, ready.Range)
	require.Empty(t, f.executor.records)
	f.startBite(t, 1)
	require.False(t, f.executor.Availability(f.actor, "fox_bite", request.Target).Ready)
	f.equip(combatTestItem(100, 7, 40, netproto.EquipSlot_EQUIP_SLOT_CHEST))
	f.finishBite()
	require.Len(t, f.sender.attacks, 1)
	require.Len(t, f.sender.attacks[0].hits, 1)
	require.InDelta(t, 36.0/14.0, f.sender.attacks[0].hits[0].damage, 1e-12)
	cooldown := f.executor.Availability(f.actor, "fox_bite", request.Target)
	require.False(t, cooldown.Ready)
	require.Equal(t, 12.0, cooldown.Range)
}

func TestNPCNaturalPreparedHitAllocationsRemainBounded(t *testing.T) {
	f := newBiteFixture(t)
	f.startBite(t, 1)
	handler := f.actions.handlers["fox_bite"].(*MeleeActionHandler)
	active, _ := ecs.GetComponent[components.ActiveGameAction](f.world, f.actor.Handle)
	var reason string
	require.Zero(t, testing.AllocsPerRun(100, func() {
		reason = handler.PrepareCompletion(f.world, 20, f.actor.Handle, actionTarget(active), 1)
		handler.AbortPrepared()
	}))
	require.Empty(t, reason)
}

func TestNPCBiteStationaryTurnPublishesMovementUpdate(t *testing.T) {
	f := newBiteFixture(t)
	attacker := f.creature(t, 3, 30, 50, false)
	f.target = contracts.EntityRef{Handle: attacker, EntityID: 3}
	ecs.WithComponent(f.world, f.actor.Handle, func(brain *components.NPCBrain) { brain.DamageSource = f.target; brain.Target = f.target })
	ecs.AddComponent(f.world, f.actor.Handle, components.Movement{State: constt.StateIdle})
	f.startBite(t, 1)
	transform, _ := ecs.GetComponent[components.Transform](f.world, f.actor.Handle)
	movement, _ := ecs.GetComponent[components.Movement](f.world, f.actor.Handle)
	require.Equal(t, math.Pi, transform.Direction)
	require.Equal(t, constt.StateIdle, movement.State)
	require.True(t, movement.Direction.UpdatePending, "an idle bite turn must reach current observers")
}

func TestMeleeDamageObserverIgnoresRoundedHealthNoop(t *testing.T) {
	f := newMeleeFixture(t)
	const hugeHP = float64(1 << 60)
	object := f.object(t, 3, 60, 50, hugeHP)
	creature := f.creature(t, 4, 62, 50, false)
	ecs.AddComponent(f.world, creature, components.EntityHealth{SHP: hugeHP, HHP: hugeHP})
	count := 0
	f.execution.SetDamageObserver(func(contracts.EntityRef, contracts.EntityRef) { count++ })
	f.start("axe_sweep", 0)
	f.finish()
	require.Len(t, f.sender.attacks, 1)
	require.Len(t, f.sender.attacks[0].hits, 2)
	for _, hit := range f.sender.attacks[0].hits {
		require.Positive(t, hit.damage, "calculated positive damage rounds away when deducted from these pools")
	}
	objectHealth, _ := ecs.GetComponent[components.ObjectInternalState](f.world, object)
	creatureHealth, _ := ecs.GetComponent[components.EntityHealth](f.world, creature)
	require.Equal(t, hugeHP, objectHealth.HP)
	require.Equal(t, hugeHP, creatureHealth.SHP)
	require.Equal(t, hugeHP, creatureHealth.HHP)
	require.Zero(t, count, "without an actual HP decrease there is no aggression stimulus")
}
