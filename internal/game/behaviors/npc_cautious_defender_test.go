package behaviors

import (
	"math"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func cautiousTestConfig() contracts.NPCBehaviorConfig {
	cfg := *npcTestConfig()
	cfg.Profile, cfg.Abilities = "cautious_defender", []string{"fox_bite"}
	cfg.Parameters = defaultCautiousParameters()
	return cfg
}

func cautiousTestObservation() NPCObservation {
	return NPCObservation{Actor: contracts.EntityRef{EntityID: 100, Handle: 10}, Position: contracts.NPCPoint{X: 300, Y: 300}, Home: contracts.NPCPoint{X: 300, Y: 300}, Tick: 10, TicksPerSecond: 10, HP: 110, MaxHP: 110, HasHP: true, PerceptionKnown: true}
}

func cautiousTestPlayer(x, y float64) NPCObservedEntity {
	return NPCObservedEntity{Ref: contracts.EntityRef{EntityID: 200, Handle: 20}, Position: contracts.NPCPoint{X: x, Y: y}, Valid: true, HasHP: true, HP: 100}
}

func TestCautiousEncounterWatchesOnceAndHabituationSurvivesWander(t *testing.T) {
	cfg, obs := cautiousTestConfig(), cautiousTestObservation()
	p := defaultCautiousParameters()
	p.RetreatProbability = 1
	cfg.Parameters = p
	obs.NearbyPlayer = cautiousTestPlayer(330, 300)
	initial := components.NPCPolicyState{RandomState: 4}
	d := decideCautiousDefender(initial, obs, cfg)
	require.Equal(t, components.NPCWatch, d.State.Activity)
	require.Equal(t, contracts.NPCIntentFace, d.Intent.Kind)
	require.True(t, d.State.Cautious.RetreatChosen)
	require.GreaterOrEqual(t, d.State.Cautious.WatchUntilTick, obs.Tick+20)
	require.LessOrEqual(t, d.State.Cautious.WatchUntilTick, obs.Tick+40)
	rolled := d.State.RandomState
	obs.Encounter, obs.Tick = obs.NearbyPlayer, obs.Tick+5
	d = decideCautiousDefender(d.State, obs, cfg)
	require.Equal(t, rolled, d.State.RandomState, "polling never rerolls the encounter")
	obs.Tick = d.State.Cautious.WatchUntilTick
	d = decideCautiousDefender(d.State, obs, cfg)
	require.Equal(t, components.NPCIdle, d.State.Activity)
	require.True(t, d.State.Cautious.Habituated)
	obs.Tick = d.State.Cautious.IdleUntilTick
	d = decideCautiousDefender(d.State, obs, cfg)
	require.Equal(t, components.NPCWander, d.State.Activity)
	obs.Tick++
	obs.Moving = false
	d = decideCautiousDefender(d.State, obs, cfg)
	require.Equal(t, components.NPCIdle, d.State.Activity)
	require.True(t, d.State.Cautious.Habituated, "ordinary wandering cannot rearm an encounter")
	obs.NearbyPlayer = NPCObservedEntity{}
	obs.Tick++
	d = decideCautiousDefender(d.State, obs, cfg)
	start := obs.Tick
	obs.Tick = start + 49
	d = decideCautiousDefender(d.State, obs, cfg)
	require.True(t, d.State.Cautious.Habituated)
	obs.PerceptionKnown = false
	obs.Tick++
	d = decideCautiousDefender(d.State, obs, cfg)
	require.True(t, d.State.Cautious.Habituated, "exhausted perception cannot count as player absence")
	obs.PerceptionKnown = true
	obs.Tick++
	d = decideCautiousDefender(d.State, obs, cfg)
	obs.Tick += 50
	d = decideCautiousDefender(d.State, obs, cfg)
	require.False(t, d.State.Cautious.Habituated)
}

func TestCautiousRareRetreatRequiresContinuedApproach(t *testing.T) {
	cfg, obs := cautiousTestConfig(), cautiousTestObservation()
	p := defaultCautiousParameters()
	p.RetreatProbability = 1
	cfg.Parameters = p
	obs.NearbyPlayer = cautiousTestPlayer(317, 300)
	d := decideCautiousDefender(components.NPCPolicyState{RandomState: 7}, obs, cfg)
	require.Equal(t, components.NPCWatch, d.State.Activity, "starting inside the trigger radius is not continued approach")
	obs.Encounter, obs.Tick = obs.NearbyPlayer, obs.Tick+5
	d = decideCautiousDefender(d.State, obs, cfg)
	require.Equal(t, contracts.NPCIntentFace, d.Intent.Kind, "a stationary nearby player causes no retreat")
	obs.Encounter.Position.X, obs.Encounter.Moving, obs.Tick = 316, true, obs.Tick+5
	d = decideCautiousDefender(d.State, obs, cfg)
	require.Equal(t, components.NPCRetreat, d.State.Activity)
	require.Equal(t, contracts.NPCIntentMove, d.Intent.Kind)
	require.Equal(t, contracts.NPCPoint{X: 264, Y: 300}, d.Intent.Move.Candidates[0])
	require.Zero(t, d.Intent.Move.HomeRadius, "flight is not constrained by wander's home radius")
	require.False(t, d.State.Cautious.RetreatChosen)
}

func TestCautiousDefensePrioritiesAndAbilityOwnership(t *testing.T) {
	cfg, obs := cautiousTestConfig(), cautiousTestObservation()
	obs.Target, obs.DamageRevision = cautiousTestPlayer(330, 300), 1
	d := decideCautiousDefender(components.NPCPolicyState{Activity: components.NPCWatch, RandomState: 8}, obs, cfg)
	require.Equal(t, components.NPCDefend, d.State.Activity)
	require.Equal(t, contracts.NPCIntentMove, d.Intent.Kind)
	require.Equal(t, obs.Target.Ref, d.Intent.Move.Target)
	obs.Target.Position.X, obs.Tick = 310, obs.Tick+5
	obs.AvailableAbility = contracts.NPCAbilityAvailabilityState{Range: 12, Ready: true}
	d = decideCautiousDefender(d.State, obs, cfg)
	require.Equal(t, contracts.NPCIntentUseAbility, d.Intent.Kind)
	require.Equal(t, "fox_bite", d.Intent.Ability.AbilityID)
	obs.Ability = contracts.NPCAbilityState{Actor: obs.Actor, AbilityID: "fox_bite", Generation: 15, Phase: contracts.NPCAbilityWindup, Status: contracts.NPCExecutionAccepted}
	obs.AbilityTarget = obs.Target
	obs.DamageRevision++
	obs.Target = cautiousTestPlayer(600, 300)
	obs.Target.Ref.EntityID++
	d = decideCautiousDefender(d.State, obs, cfg)
	require.Equal(t, contracts.NPCIntentContinue, d.Intent.Kind, "a new attacker cannot cancel or redirect an accepted bite")
	obs.HP = 27.5
	d = decideCautiousDefender(d.State, obs, cfg)
	require.Equal(t, components.NPCRetreat, d.State.Activity)
	require.Equal(t, contracts.NPCIntentCancelAbility, d.Intent.Kind)
	feedback := cautiousDefenderResult(d.State, obs, cfg, contracts.NPCExecutionResult{Status: contracts.NPCExecutionCanceled})
	require.Equal(t, contracts.NPCIntentMove, feedback.Intent.Kind)
	require.Equal(t, 264.0, feedback.Intent.Move.Candidates[0].X)
	obs.Ability = contracts.NPCAbilityState{}
	obs.Target.Position.X, obs.DamageRevision, obs.Tick = 290, obs.DamageRevision+1, obs.Tick+1
	d = decideCautiousDefender(feedback.State, obs, cfg)
	require.Equal(t, 336.0, d.Intent.Move.Candidates[0].X, "a new low-health hit starts a fresh short retreat")
}

func TestCautiousDefenseWaitsOnCooldownAndStopsOnEligibility(t *testing.T) {
	cfg, obs := cautiousTestConfig(), cautiousTestObservation()
	obs.Target, obs.DamageRevision = cautiousTestPlayer(310, 300), 1
	obs.AvailableAbility = contracts.NPCAbilityAvailabilityState{Range: 12}
	d := decideCautiousDefender(components.NPCPolicyState{}, obs, cfg)
	require.Equal(t, contracts.NPCIntentFace, d.Intent.Kind, "cooldown stops the chase instead of pushing into the target")
	for _, scenario := range []string{"lost", "incapacitated", "far", "home"} {
		t.Run(scenario, func(t *testing.T) {
			observation := obs
			observation.Moving, observation.ActivityDeadlineTick = true, 999
			switch scenario {
			case "lost":
				observation.Target.Valid = false
			case "incapacitated":
				observation.Target.Incapacitated = true
			case "far":
				observation.Target.Position.X = 373
			case "home":
				observation.Home.X = 100
			}
			result := decideCautiousDefender(d.State, observation, cfg)
			require.NotEqual(t, components.NPCDefend, result.State.Activity)
			require.NotEqual(t, contracts.NPCIntentContinue, result.Intent.Kind, "the old chase must stop immediately")
		})
	}
}

func TestCautiousFailedChaseAndBlockedReturnAreBounded(t *testing.T) {
	cfg, obs := cautiousTestConfig(), cautiousTestObservation()
	obs.Target, obs.DamageRevision, obs.Home.X = cautiousTestPlayer(340, 300), 1, 200
	d := decideCautiousDefender(components.NPCPolicyState{}, obs, cfg)
	obs.Tick += 29
	d = decideCautiousDefender(d.State, obs, cfg)
	require.Equal(t, components.NPCDefend, d.State.Activity)
	obs.Tick++
	obs.Moving, obs.ActivityDeadlineTick = true, 999
	d = decideCautiousDefender(d.State, obs, cfg)
	require.Equal(t, components.NPCReturnHome, d.State.Activity)
	require.Equal(t, contracts.NPCIntentMove, d.Intent.Kind)
	require.InDelta(t, 24, math.Sqrt(npcDistanceSquared(obs.Position, d.Intent.Move.Candidates[0])), .001)
	require.Zero(t, d.Intent.Move.HomeRadius)
	d = cautiousDefenderResult(d.State, obs, cfg, contracts.NPCExecutionResult{Status: contracts.NPCExecutionBlocked})
	require.Equal(t, components.NPCReturnHome, d.State.Activity)
	require.Equal(t, obs.Tick+10, d.NextDecisionTick)
	obs.Tick, obs.Moving = d.NextDecisionTick, false
	d = decideCautiousDefender(d.State, obs, cfg)
	require.Equal(t, contracts.NPCIntentMove, d.Intent.Kind, "an obstacle does not strand the animal in passive wander outside home")
}

func TestCautiousBlockedBiteAbandonsDefenseAfterThreeSeconds(t *testing.T) {
	f := cautiousTestWorld(t)
	h := f.spawn(t, 100, 1001)
	player := cautiousSpawnPlayer(f.world, 200, 310, 300)
	require.True(t, f.controller.NotifyDamage(npcExtensionRef(f.world, h), npcExtensionRef(f.world, player)))
	for tick := uint64(10); tick < 40; tick += 5 {
		// The target is in reach, but a line-of-attack obstacle rejects admission.
		f.executor.nextOutcome = contracts.NPCExecutionBlocked
		brain := f.tick(t, h, tick)
		require.Equal(t, components.NPCDefend, brain.Activity)
		require.True(t, brain.Cautious.ChaseTracking)
		require.Equal(t, uint64(10), brain.Cautious.ChaseFailedSinceTick)
	}
	brain := f.tick(t, h, 40)
	require.Equal(t, components.NPCIdle, brain.Activity, "the actor is already home when the failed defense ends")
	require.Len(t, f.executor.starts, 6, "no seventh bite may bypass the unsuccessful pursuit limit")
}

func TestCautiousBiteCooldownAndSuccessResetFailureTracking(t *testing.T) {
	cfg, obs := cautiousTestConfig(), cautiousTestObservation()
	obs.Target, obs.DamageRevision, obs.AvailableAbility = cautiousTestPlayer(310, 300), 1, contracts.NPCAbilityAvailabilityState{Range: 12, Ready: true}
	d := decideCautiousDefender(components.NPCPolicyState{}, obs, cfg)
	d = cautiousDefenderResult(d.State, obs, cfg, contracts.NPCExecutionResult{Status: contracts.NPCExecutionBlocked})
	require.True(t, d.State.Cautious.ChaseTracking)
	obs.Tick, obs.AvailableAbility.Ready = 35, false
	d = decideCautiousDefender(d.State, obs, cfg)
	require.Equal(t, contracts.NPCIntentFace, d.Intent.Kind)
	require.False(t, d.State.Cautious.ChaseTracking)
	obs.Tick = 100
	d = decideCautiousDefender(d.State, obs, cfg)
	require.Equal(t, components.NPCDefend, d.State.Activity, "cooldown waiting does not count as blocked pursuit")
	obs.AvailableAbility.Ready = true
	d = decideCautiousDefender(d.State, obs, cfg)
	require.Equal(t, contracts.NPCIntentUseAbility, d.Intent.Kind)
	d = cautiousDefenderResult(d.State, obs, cfg, contracts.NPCExecutionResult{Status: contracts.NPCExecutionAccepted})
	require.False(t, d.State.Cautious.ChaseTracking, "accepted bites reset the failed attempt timer")
	obs.Tick = 200
	d = decideCautiousDefender(d.State, obs, cfg)
	require.Equal(t, contracts.NPCIntentUseAbility, d.Intent.Kind)
	require.Equal(t, uint64(200), d.State.Cautious.ChaseFailedSinceTick)
}

func TestCautiousInvalidAcceptedTargetKeepsLatestAggressor(t *testing.T) {
	for _, failure := range []string{"KO", "lost", "far", "corpse"} {
		t.Run(failure, func(t *testing.T) {
			cfg, obs := cautiousTestConfig(), cautiousTestObservation()
			obs.Target, obs.DamageRevision = cautiousTestPlayer(310, 300), 1
			obs.AvailableAbility = contracts.NPCAbilityAvailabilityState{Range: 12, Ready: true}
			d := decideCautiousDefender(components.NPCPolicyState{}, obs, cfg)
			obs.Ability = contracts.NPCAbilityState{Actor: obs.Actor, AbilityID: "fox_bite", Generation: 2, Phase: contracts.NPCAbilityWindup, Status: contracts.NPCExecutionAccepted}
			obs.AbilityTarget = obs.Target
			obs.Target.Ref.EntityID++
			obs.DamageRevision++
			d = decideCautiousDefender(d.State, obs, cfg)
			require.Equal(t, contracts.NPCIntentContinue, d.Intent.Kind)
			require.Equal(t, obs.DamageRevision, d.State.Cautious.DamageRevision, "the new hit is already consumed")
			switch failure {
			case "KO":
				obs.AbilityTarget.Incapacitated = true
			case "lost":
				obs.AbilityTarget.Valid = false
			case "far":
				obs.AbilityTarget.Position.X = 400
			case "corpse":
				obs.AbilityTarget.HasHP = false
			}
			obs.Tick++
			d = decideCautiousDefender(d.State, obs, cfg)
			require.Equal(t, contracts.NPCIntentCancelAbility, d.Intent.Kind)
			require.Equal(t, components.NPCDefend, d.State.Activity)
			d = cautiousDefenderResult(d.State, obs, cfg, contracts.NPCExecutionResult{Status: contracts.NPCExecutionCanceled})
			require.Equal(t, contracts.NPCIntentUseAbility, d.Intent.Kind)
			require.Equal(t, obs.Target.Ref, d.Intent.Ability.Target.Entity, "the latest eligible aggressor needs no additional hit")
		})
	}
}

func TestCautiousCollisionStoppedReturnRetriesAfterOneSecond(t *testing.T) {
	f := cautiousTestWorld(t)
	h := f.spawn(t, 100, 1001)
	ecs.WithComponent(f.world, h, func(tr *components.Transform) { tr.X = 400 })
	ecs.WithComponent(f.world, h, func(brain *components.NPCBrain) { brain.Activity = components.NPCReturnHome })
	brain := f.tick(t, h, 10)
	require.Equal(t, components.NPCMovementAI, brain.MovementLease.Owner)
	// Model a collision stop reported through the production movement guard.
	ecs.WithComponent(f.world, h, func(move *components.Movement) { move.ClearTarget() })
	ecs.AddComponent(f.world, h, components.CollisionResult{FinalX: 400, FinalY: 300, PerpendicularOscillation: true})
	ecs.GetResource[ecs.MovedEntities](f.world).Add(h, 400, 300)
	ecs.GetResource[ecs.TimeState](f.world).Tick = 11
	NewNPCMovementGuard(f.controller).Update(f.world, .1)
	brain, _ = ecs.GetComponent[components.NPCBrain](f.world, h)
	require.Equal(t, contracts.NPCExecutionBlocked, brain.LastResult)
	brain = f.tick(t, h, 11)
	require.Equal(t, components.NPCReturnHome, brain.Activity)
	require.Equal(t, uint64(21), brain.Cautious.ReturnRetryTick)
	require.Equal(t, components.NPCMovementUnowned, brain.MovementLease.Owner)
	for _, tick := range []uint64{12, 20} {
		brain = f.tick(t, h, tick)
		require.Equal(t, uint64(21), brain.Cautious.ReturnRetryTick, "early wakes cannot extend the delay")
		require.Equal(t, components.NPCMovementUnowned, brain.MovementLease.Owner)
	}
	brain = f.tick(t, h, 21)
	require.Zero(t, brain.Cautious.ReturnRetryTick)
	require.Equal(t, components.NPCMovementAI, brain.MovementLease.Owner)
	move, _ := ecs.GetComponent[components.Movement](f.world, h)
	require.Equal(t, 376.0, move.TargetX)
}

func TestCautiousCorpseConversionEndsWatchChaseAndBite(t *testing.T) {
	for _, activity := range []string{"watch", "chase", "bite"} {
		t.Run(activity, func(t *testing.T) {
			f := cautiousTestWorld(t)
			h := f.spawn(t, 100, 1001)
			x := 310.0
			if activity == "chase" {
				x = 330
			}
			player := cautiousSpawnPlayer(f.world, 200, x, 300)
			if activity != "watch" {
				require.True(t, f.controller.NotifyDamage(npcExtensionRef(f.world, h), npcExtensionRef(f.world, player)))
			}
			_ = f.tick(t, h, 10)
			// Permanent death keeps identity/transform while removing player state.
			ecs.RemoveComponent[components.CharacterProfile](f.world, player)
			ecs.RemoveComponent[components.EntityHealth](f.world, player)
			require.True(t, f.world.Alive(player))
			observed := f.controller.observeEntity(npcExtensionRef(f.world, player))
			require.True(t, observed.Valid, "generic profiles may still observe non-player entities")
			require.False(t, observed.HasHP)
			brain := f.tick(t, h, 11)
			require.Equal(t, components.NPCIdle, brain.Activity)
			require.Zero(t, brain.Ability.Generation)
			move, _ := ecs.GetComponent[components.Movement](f.world, h)
			require.Equal(t, constt.StateIdle, move.State)
			brain = f.tick(t, h, 20)
			require.NotEqual(t, components.NPCDefend, brain.Activity)
			require.NotEqual(t, components.NPCWatch, brain.Activity)
		})
	}
}

type cautiousTestExecutor struct{ *npcExtensionExecutor }

func (f cautiousTestExecutor) Availability(contracts.EntityRef, string, contracts.NPCTarget) contracts.NPCAbilityAvailabilityState {
	return contracts.NPCAbilityAvailabilityState{Range: 12, Ready: true}
}

func cautiousTestWorld(t *testing.T) npcExtensionWorld {
	t.Helper()
	npcExtensionInstall(t, DefaultNPCCatalog(), cautiousTestConfig())
	w := ecs.NewWorldForTesting()
	core.AttachColliderSpatial(w)
	ecs.GetResource[ecs.TimeState](w).TickPeriod = 100 * time.Millisecond
	fake := newNPCExtensionExecutor(w)
	c, err := AttachNPCController(w, NPCControllerConfig{Chunks: npcTestChunks{{}: npcTestLand(t, types.ChunkCoord{})}, Abilities: cautiousTestExecutor{fake}})
	require.NoError(t, err)
	return npcExtensionWorld{world: w, controller: c, executor: fake, behavior: NewNPCAIBehavior(DefaultNPCCatalog())}
}

func cautiousSpawnPlayer(w *ecs.World, id types.EntityID, x, y float64) types.Handle {
	return w.Spawn(id, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: x, Y: y})
		ecs.AddComponent(w, h, components.Collider{HalfWidth: 2, HalfHeight: 2})
		ecs.AddComponent(w, h, components.CharacterProfile{})
		ecs.AddComponent(w, h, components.EntityHealth{SHP: 100, HHP: 100})
	})
}

func TestCautiousControllerSeparatesPerceptionFromConfirmedDamage(t *testing.T) {
	f := cautiousTestWorld(t)
	h := f.spawn(t, 100, 1001)
	player := cautiousSpawnPlayer(f.world, 200, 300, 330)
	brain := f.tick(t, h, 10)
	require.Equal(t, components.NPCWatch, brain.Activity)
	require.Zero(t, brain.DamageRevision)
	require.Empty(t, brain.Target)
	require.Empty(t, brain.DamageSource)
	tr, _ := ecs.GetComponent[components.Transform](f.world, h)
	move, _ := ecs.GetComponent[components.Movement](f.world, h)
	require.Equal(t, math.Pi/2, tr.Direction)
	require.True(t, move.Direction.UpdatePending, "stationary facing uses the ordinary movement update pipeline")
	require.Equal(t, constt.StateIdle, move.State)
	ref := npcExtensionRef(f.world, player)
	require.True(t, f.controller.NotifyDamage(npcExtensionRef(f.world, h), ref))
	brain = f.tick(t, h, 11)
	require.Equal(t, components.NPCDefend, brain.Activity)
	require.Equal(t, ref, brain.DamageSource)
	require.Equal(t, ref, brain.Target)
	other := cautiousSpawnPlayer(f.world, 201, 310, 300)
	require.True(t, f.controller.NotifyDamage(npcExtensionRef(f.world, h), npcExtensionRef(f.world, other)))
	brain = f.tick(t, h, 12)
	require.Equal(t, types.EntityID(201), brain.DamageSource.EntityID)
	require.Equal(t, uint64(2), brain.DamageRevision)
	require.Len(t, f.executor.starts, 1)
	accepted := brain.Ability
	brain = f.tick(t, h, 13)
	require.Equal(t, accepted, brain.Ability)
	require.Len(t, f.executor.starts, 1, "polling cannot restart the running bite")
	require.True(t, f.controller.NotifyDamage(npcExtensionRef(f.world, h), ref))
	brain = f.tick(t, h, 14)
	require.Equal(t, ref, brain.DamageSource)
	require.Equal(t, accepted, brain.Ability, "a different aggressor affects only the next ability")
	require.Empty(t, f.executor.cancels)
	require.Len(t, f.executor.starts, 1)
	// Delayed handles are invalid even if the persistent ID is reused.
	f.world.Despawn(player)
	_ = cautiousSpawnPlayer(f.world, 200, 300, 330)
	require.False(t, f.controller.NotifyDamage(npcExtensionRef(f.world, h), ref))
}

func TestCautiousPerceptionBudgetExhaustionIsUnknown(t *testing.T) {
	f := cautiousTestWorld(t)
	h := f.spawn(t, 100, 1001)
	_ = cautiousSpawnPlayer(f.world, 200, 310, 300)
	for i := 0; i < 256; i++ {
		f.world.Spawn(types.EntityID(1000+i), func(w *ecs.World, handle types.Handle) {
			ecs.AddComponent(w, handle, components.Transform{X: 300, Y: 300})
			ecs.AddComponent(w, handle, components.Collider{HalfWidth: 1, HalfHeight: 1})
		})
	}
	brain, _ := ecs.GetComponent[components.NPCBrain](f.world, h)
	obs := f.controller.observe(h, 100, brain, 20)
	f.controller.observeProfile(cautiousDefenderProfile(), brain, cautiousTestConfig(), 110, &obs)
	require.False(t, obs.PerceptionKnown)
	require.False(t, obs.NearbyPlayer.Valid, "a partial bounded query is never presented as a successful observation")
	brain.Cautious.Habituated, brain.Cautious.AbsenceTracking, brain.Cautious.AbsentSinceTick = true, true, 1
	d := decideCautiousDefender(brain.NPCPolicyState, obs, cautiousTestConfig())
	require.True(t, d.State.Cautious.Habituated)
	require.False(t, d.State.Cautious.AbsenceTracking)
}

func TestCautiousConfigAndPolling(t *testing.T) {
	cfg := cautiousTestConfig()
	require.NoError(t, DefaultNPCCatalog().validate(&cfg))
	p := cfg.Parameters.(contracts.NPCCombatHealthPolicy)
	require.Equal(t, .25, p.MinimumCombatHealthFraction())
	for _, raw := range []string{`{"unknown":1}`, `{"rearmRadiusTiles":7}`, `{"retreatProbability":1.1}`, `{"lowHPFraction":0}`} {
		_, err := compileCautiousParameters([]byte(raw))
		require.Error(t, err)
	}
	seen := map[uint64]bool{}
	for id := types.EntityID(1); id <= 10; id++ {
		obs := cautiousTestObservation()
		obs.Actor.EntityID = id
		next := cautiousNextPoll(obs, false)
		require.Greater(t, next, obs.Tick)
		require.LessOrEqual(t, next, obs.Tick+10)
		seen[next] = true
	}
	require.Len(t, seen, 10, "idle polls are staggered across the entire second")
}

func TestCautiousPerceptionReusesBoundedBuffer(t *testing.T) {
	f := cautiousTestWorld(t)
	h := f.spawn(t, 100, 1001)
	player := cautiousSpawnPlayer(f.world, 200, 320, 300)
	brain, _ := ecs.GetComponent[components.NPCBrain](f.world, h)
	profile, cfg := cautiousDefenderProfile(), cautiousTestConfig()
	var observation NPCObservation
	allocations := testing.AllocsPerRun(100, func() {
		observation = f.controller.observe(h, 100, brain, 10)
		f.controller.observeProfile(profile, brain, cfg, 110, &observation)
	})
	require.Zero(t, allocations)
	require.True(t, observation.PerceptionKnown)
	require.Equal(t, player, observation.NearbyPlayer.Ref.Handle)
	require.True(t, observation.HasHP)
	require.Equal(t, 110.0, observation.HP)
	require.Equal(t, 100.0, observation.NearbyPlayer.HP)
}
