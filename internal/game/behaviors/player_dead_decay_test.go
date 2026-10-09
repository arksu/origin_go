package behaviors

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
)

func TestPlayerDeadDecayInitializesOnceAcrossLifecycleReasons(t *testing.T) {
	for _, reason := range []contracts.ObjectBehaviorInitReason{contracts.ObjectBehaviorInitReasonSpawn, contracts.ObjectBehaviorInitReasonRestore, contracts.ObjectBehaviorInitReasonTransform} {
		t.Run(string(reason), func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			expectedDeadline := int64(500) + CorpseDecaySeconds
			ecs.SetResource(w, ecs.TimeState{Tick: 100, RuntimeSecondsTotal: 500})
			h := w.Spawn(1, nil)
			ecs.AddComponent(w, h, components.ObjectInternalState{HP: 0.49, HasHP: true, Flags: []string{"existing.flag"}})
			ctx := &contracts.BehaviorObjectInitContext{World: w, Handle: h, EntityID: 1, EntityType: 15, Reason: reason}
			if err := (playerDeadBehavior{}).InitObject(ctx); err != nil {
				t.Fatal(err)
			}
			state, _ := ecs.GetComponent[components.ObjectInternalState](w, h)
			decay, ok := components.GetBehaviorState[components.CorpseDecayBehaviorState](state, "player_dead")
			if !ok || decay.DecayAtRuntimeSeconds != expectedDeadline || !state.IsDirty || state.HP != 0.49 || !state.HasHP || len(state.Flags) != 1 {
				t.Fatalf("invalid initialized state: %+v, decay %+v", state, decay)
			}
			ecs.WithComponent(w, h, func(current *components.ObjectInternalState) { current.IsDirty = false })
			ecs.SetResource(w, ecs.TimeState{Tick: 900000, TickRate: 10, RuntimeSecondsTotal: expectedDeadline - 1})
			for _, repeatReason := range []contracts.ObjectBehaviorInitReason{contracts.ObjectBehaviorInitReasonRestore, contracts.ObjectBehaviorInitReasonTransform, contracts.ObjectBehaviorInitReasonSpawn} {
				ctx.Reason = repeatReason
				if err := (playerDeadBehavior{}).InitObject(ctx); err != nil {
					t.Fatal(err)
				}
			}
			state, _ = ecs.GetComponent[components.ObjectInternalState](w, h)
			decay, _ = components.GetBehaviorState[components.CorpseDecayBehaviorState](state, "player_dead")
			if decay.DecayAtRuntimeSeconds != expectedDeadline || state.IsDirty {
				t.Fatalf("repeated init changed restored deadline or dirty state: %+v, %+v", state, decay)
			}
			schedule := ecs.GetResource[ecs.CorpseDecaySchedule](w)
			if schedule.PendingCount() != 1 {
				t.Fatalf("repeated init duplicated queue: %d", schedule.PendingCount())
			}
			if _, due := schedule.PopDue(expectedDeadline - 1); due {
				t.Fatal("decay fired before its configured runtime deadline")
			}
			entry, due := schedule.PopDue(expectedDeadline)
			if !due || entry.Handle != h || entry.EntityID != 1 {
				t.Fatalf("decay not due at exact deadline: %+v", entry)
			}
		})
	}
}

func TestPlayerDeadDecayRestoresOverdueDeadlineWithoutRestartingClock(t *testing.T) {
	w := ecs.NewWorldForTesting()
	ecs.SetResource(w, ecs.TimeState{RuntimeSecondsTotal: 50000, Tick: 2})
	h := w.Spawn(1, nil)
	state := components.ObjectInternalState{HP: 100, HasHP: true}
	components.SetBehaviorState(&state, "player_dead", &components.CorpseDecayBehaviorState{DecayAtRuntimeSeconds: 21600})
	state.IsDirty = false
	ecs.AddComponent(w, h, state)
	if err := (playerDeadBehavior{}).InitObject(&contracts.BehaviorObjectInitContext{World: w, Handle: h, EntityID: 1, EntityType: 15, Reason: contracts.ObjectBehaviorInitReasonRestore}); err != nil {
		t.Fatal(err)
	}
	state, _ = ecs.GetComponent[components.ObjectInternalState](w, h)
	if state.IsDirty {
		t.Fatal("restoring a valid state must not dirty or extend its deadline")
	}
	entry, ok := ecs.GetResource[ecs.CorpseDecaySchedule](w).PopDue(50000)
	if !ok || entry.DueRuntimeSeconds != 21600 {
		t.Fatalf("overdue corpse did not remain due: %+v", entry)
	}
}

func TestPlayerDeadDecayDoesNotInitializeInvalidLifecycleTargets(t *testing.T) {
	w := ecs.NewWorldForTesting()
	h := w.Spawn(1, nil)
	ecs.AddComponent(w, h, components.ObjectInternalState{HP: 100, HasHP: true})
	if err := (playerDeadBehavior{}).InitObject(&contracts.BehaviorObjectInitContext{World: w, Handle: h, EntityID: 1}); err != nil {
		t.Fatal(err)
	}
	if ecs.HasResource[ecs.CorpseDecaySchedule](w) {
		t.Fatal("empty lifecycle reason must not start decay")
	}
	w.Despawn(h)
	if err := (playerDeadBehavior{}).InitObject(&contracts.BehaviorObjectInitContext{World: w, Handle: h, EntityID: 1, Reason: contracts.ObjectBehaviorInitReasonRestore}); err != nil {
		t.Fatal(err)
	}
	if ecs.HasResource[ecs.CorpseDecaySchedule](w) {
		t.Fatal("stale handle must not retain a deadline")
	}
}
