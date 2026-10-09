package game

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"origin/internal/actionanimationdefs"
	"origin/internal/actiondefs"
	"origin/internal/characterattrs"
	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/cyclicaction"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

type meleeHitSample struct {
	id     uint64
	damage float64
}
type meleeResultSample struct {
	event uint64
	hits  []meleeHitSample
}
type meleeTestSender struct {
	testActionSender
	attacks []meleeResultSample
}

func (s *meleeTestSender) SendAttackResult(_ types.Handle, _ types.EntityID, event uint64, hits []netproto.AttackHit) {
	sample := meleeResultSample{event: event, hits: make([]meleeHitSample, len(hits))}
	for i := range hits {
		sample.hits[i] = meleeHitSample{hits[i].TargetId, hits[i].Damage}
	}
	s.attacks = append(s.attacks, sample)
}

type meleeTestChunks struct {
	damageTestChunks
	calls, pins, failAt int
}

var errMeleeTestPin = errors.New("test pin rejected")

func (c *meleeTestChunks) PinPersistence(types.ChunkCoord) error {
	c.calls++
	if c.pinErr != nil {
		return c.pinErr
	}
	if c.calls == c.failAt {
		return errMeleeTestPin
	}
	c.pins++
	return nil
}
func (c *meleeTestChunks) UnpinPersistence(types.ChunkCoord) { c.pins-- }

type meleeFixture struct {
	*creatureDamageFixture
	execution   *MeleeExecutionService
	objects     *ObjectDamageService
	actions     *ActionService
	definitions *actiondefs.Registry
	sender      *meleeTestSender
	chunks      *meleeTestChunks
	quarantined []types.Handle
	cyclic      *CyclicActionSystem
}

func newMeleeFixture(t testing.TB) *meleeFixture {
	t.Helper()
	return meleeFixtureInWorld(t, ecs.NewWorldForTesting())
}

func meleeFixtureInWorld(t testing.TB, w *ecs.World) *meleeFixture {
	t.Helper()
	core.AttachColliderSpatial(w)
	f := &meleeFixture{creatureDamageFixture: creatureDamageFixtureInWorld(t, w), sender: &meleeTestSender{}, chunks: &meleeTestChunks{}}
	registry := itemdefs.NewRegistry(combatTestDefinitions())
	previous := itemdefs.Global()
	itemdefs.SetGlobalForTesting(registry)
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	var err error
	f.definitions, err = actiondefs.LoadFromDirectory("../../data/actions", zap.NewNop())
	require.NoError(t, err)
	ecs.AddComponent(w, f.owner, components.CharacterProfile{Attributes: characterattrs.Default()})
	ecs.AddComponent(w, f.owner, components.EntityHealth{SHP: 50, HHP: 50})
	ecs.AddComponent(w, f.owner, components.EntityStats{Stamina: 1000, Energy: 900})
	ecs.AddComponent(w, f.owner, components.Transform{X: 50, Y: 50})
	ecs.AddComponent(w, f.owner, components.Collider{HalfWidth: 1, HalfHeight: 1})
	f.equip(combatTestItem(100, 1, 10, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND))
	destruction, err := NewObjectDestructionService(w, ObjectDestructionDependencies{
		Chunks: f.chunks, Persister: &damageTestPersister{}, IDs: &damageTestIDs{}, Items: registry,
		WithWorldRead: func(fn func(*ecs.World)) { fn(w) }, Quarantine: func(h types.Handle) {
			f.quarantined = append(f.quarantined, h)
			ecs.RemoveComponent[components.Collider](w, h)
		}, Region: 1, MaxX: 2000, MaxY: 2000,
	})
	require.NoError(t, err)
	f.objects, err = NewObjectDamageService(w, destruction)
	require.NoError(t, err)
	sectors, err := NewSectorResolver(w)
	require.NoError(t, err)
	f.execution, err = NewMeleeExecutionService(w, f.resolver, sectors, f.service, f.objects, &AttackEventSequence{}, f.sender)
	require.NoError(t, err)
	handlers := make(map[string]ActionHandler)
	for _, definition := range f.definitions.All() {
		if definition.Combat == nil {
			continue
		}
		prepared, err := f.catalog.PrepareMeleeAction(definition)
		require.NoError(t, err)
		handlers[definition.ID], err = NewMeleeActionHandler(f.execution, definition, prepared)
		require.NoError(t, err)
	}
	f.actions, err = NewActionService(w, f.definitions, handlers, f.sender)
	require.NoError(t, err)
	f.cyclic = NewCyclicActionSystem(nil, f.sender, zap.NewNop())
	f.cyclic.SetActionService(f.actions)
	return f
}

func TestMeleePreparedCompletionRejectsReentryWithoutAbortingOuterPlan(t *testing.T) {
	f := newMeleeFixture(t)
	target := f.object(t, 3, 60, 50, 1)
	handler := f.actions.handlers["axe_sweep"].(*MeleeActionHandler)
	strike := f.actions.handlers["axe_strike"].(*MeleeActionHandler)
	targeting := ActionTarget{AimAngle: 0}
	require.Empty(t, handler.PrepareCompletion(f.world, 1, f.owner, targeting, 1))
	require.Equal(t, 1, f.chunks.pins)
	for _, nested := range []*MeleeActionHandler{handler, strike} {
		require.Equal(t, "ACTION_COMBAT_BUSY", nested.PrepareCompletion(f.world, 1, f.owner, targeting, 2))
		require.True(t, f.execution.active)
		require.Equal(t, 1, f.execution.count)
		require.Equal(t, 1, f.chunks.pins)
	}
	handler.AbortPrepared()
	require.Zero(t, f.chunks.pins)
	require.False(t, f.objects.state.Pending[target])
	state, _ := ecs.GetComponent[components.ObjectInternalState](f.world, target)
	require.Equal(t, 1.0, state.HP)
	f.start("axe_sweep", 0)
	f.finish()
	require.True(t, f.objects.state.Pending[target])
	require.Len(t, f.sender.attacks, 1)
}

func TestMeleeLiveArmorAndSequentialKO(t *testing.T) {
	f := newMeleeFixture(t)
	target := f.creature(t, 3, 60, 50, true)
	f.start("axe_sweep", 0)
	container := f.world.GetHandleByEntityID(10003)
	ecs.WithComponent(f.world, container, func(c *components.InventoryContainer) { c.Items[0].Quality = 40 })
	f.finish()
	require.InDelta(t, 36.0/14, f.sender.attacks[0].hits[0].damage, 1e-12)

	// Two completed actions in the same wall time read each other's committed
	// health: the second hit observes active KO and removes only hard health.
	for _, attack := range []int{0, 1} {
		ecs.RemoveComponent[components.ActionCooldowns](f.world, f.owner)
		if attack == 0 {
			ecs.WithComponent(f.world, container, func(c *components.InventoryContainer) { c.Items = nil })
			ecs.WithComponent(f.world, target, func(h *components.EntityHealth) { *h = components.EntityHealth{SHP: 6, HHP: 50} })
		}
		f.start("axe_sweep", 0)
		for i := 0; i < 6; i++ {
			ecs.GetResource[ecs.TimeState](f.world).Tick++
			f.cyclic.Update(f.world, .1)
		}
		health, _ := ecs.GetComponent[components.EntityHealth](f.world, target)
		require.Zero(t, health.SHP)
		require.Equal(t, int64(61600), health.KOUntilUnixMs)
		want := 48.8
		if attack == 1 {
			want = 42.8
		}
		require.InDelta(t, want, health.HHP, 1e-12)
	}
	// The health pass, rather than the action receiver, owns permanent death.
	ecs.GetResource[ecs.CharacterEntities](f.world).Add(3, target, time.Now())
	ecs.WithComponent(f.world, target, func(h *components.EntityHealth) { h.HHP = .49 })
	ecs.RemoveComponent[components.ActionCooldowns](f.world, f.owner)
	f.start("axe_sweep", 0)
	f.finish()
	handler := &testPlayerDeathHandler{}
	death := NewPlayerDeathSystem(handler, PlayerDeathSystemConfig{})
	require.Empty(t, handler.calls)
	death.Update(f.world, 0)
	death.Update(f.world, 0)
	require.Len(t, handler.calls, 1)
	require.Equal(t, target, handler.calls[0].handle)
}

func TestMeleePreparationAllocationFailuresAndMiss(t *testing.T) {
	for _, scenario := range []string{"miss", "unprepared", "invalid_health", "missing_weapon", "pin_failure"} {
		t.Run(scenario, func(t *testing.T) {
			f := newMeleeFixture(t)
			definition, _ := f.definitions.Get("axe_sweep")
			prepared, err := f.catalog.PrepareMeleeAction(definition)
			require.NoError(t, err)
			switch scenario {
			case "unprepared":
				target := f.creature(t, 3, 60, 50, false)
				f.service.activity.Release(target)
			case "invalid_health":
				target := f.creature(t, 3, 60, 50, false)
				ecs.WithComponent(f.world, target, func(h *components.EntityHealth) { h.HHP = math.NaN() })
			case "missing_weapon":
				f.equip()
			case "pin_failure":
				f.object(t, 3, 60, 50, 1)
				f.chunks.pinErr = errMeleeTestPin
			}
			var got error
			require.Zero(t, testing.AllocsPerRun(100, func() {
				got = f.execution.prepare(f.owner, 1, definition, prepared, 0)
				f.execution.abort()
			}))
			if scenario == "miss" {
				require.NoError(t, got)
			} else {
				require.Error(t, got)
			}
		})
	}
}

func (f *meleeFixture) creature(t testing.TB, id types.EntityID, x, y float64, armor bool) types.Handle {
	t.Helper()
	h := f.world.Spawn(id, nil)
	ecs.AddComponent(f.world, h, components.Transform{X: x, Y: y})
	ecs.AddComponent(f.world, h, components.Collider{HalfWidth: 1, HalfHeight: 1})
	ecs.AddComponent(f.world, h, components.EntityHealth{SHP: 25, HHP: 25})
	ecs.AddComponent(f.world, h, components.Movement{})
	if armor {
		container := f.world.Spawn(id+10000, nil)
		ecs.AddComponent(f.world, container, components.InventoryContainer{OwnerID: id, Kind: constt.InventoryEquipment,
			Items: []components.InvItem{combatTestItem(id+20000, 7, 10, netproto.EquipSlot_EQUIP_SLOT_CHEST)}})
		ecs.GetResource[ecs.InventoryRefIndex](f.world).Add(constt.InventoryEquipment, id, 0, container)
	}
	require.NoError(t, f.service.PrepareTarget(h))
	return h
}

func (f *meleeFixture) object(t testing.TB, id types.EntityID, x, y, hp float64) types.Handle {
	t.Helper()
	h := f.world.Spawn(id, nil)
	ecs.AddComponent(f.world, h, components.EntityInfo{TypeID: 99, Region: 1})
	ecs.AddComponent(f.world, h, components.Transform{X: x, Y: y})
	ecs.AddComponent(f.world, h, components.ChunkRef{})
	ecs.AddComponent(f.world, h, components.ObjectInternalState{HP: hp, HasHP: true})
	ecs.AddComponent(f.world, h, components.Collider{HalfWidth: 1, HalfHeight: 1})
	require.NoError(t, f.objects.PrepareTarget(h))
	return h
}

func (f *meleeFixture) start(id string, angle float32) {
	ecs.WithComponent(f.world, f.owner, func(transform *components.Transform) { transform.Direction = float64(angle) })
	f.actions.ActivateRequest(f.world, 1, f.owner, &netproto.C2S_ActivateAction{ActionId: id, StreamEpoch: 1})
}
func (f *meleeFixture) tick() {
	clock := ecs.GetResource[ecs.TimeState](f.world)
	clock.Tick++
	clock.UnixMs += 100
	f.cyclic.Update(f.world, .1)
}
func (f *meleeFixture) finish() {
	for i := 0; i < 6; i++ {
		f.tick()
	}
}
func (f *meleeFixture) stamina() float64 {
	stats, _ := ecs.GetComponent[components.EntityStats](f.world, f.owner)
	return stats.Stamina
}

func TestMeleeTimedPresetAndMiss(t *testing.T) {
	for _, id := range []string{"axe_sweep", "axe_strike"} {
		t.Run(id, func(t *testing.T) {
			f := newMeleeFixture(t)
			target := f.creature(t, 3, 60, 50, true)
			f.start(id, 0)
			for i := 0; i < 5; i++ {
				f.tick()
			}
			health, _ := ecs.GetComponent[components.EntityHealth](f.world, target)
			require.Equal(t, 25.0, health.SHP)
			require.Equal(t, 1000.0, f.stamina())
			require.Empty(t, f.sender.attacks)
			f.tick()
			want := 3.6
			if id == "axe_strike" {
				want = 81.0 / 13
			}
			health, _ = ecs.GetComponent[components.EntityHealth](f.world, target)
			require.InDelta(t, 25-want, health.SHP, 1e-12)
			require.InDelta(t, 25-.2*want, health.HHP, 1e-12)
			require.Equal(t, 940.0, f.stamina())
			require.Len(t, f.sender.attacks, 1)
			require.InDelta(t, want, f.sender.attacks[0].hits[0].damage, 1e-12)
			require.Equal(t, int64(3600), f.actions.State(f.world, f.owner).Cooldowns[0].ExpiresAtMs)
			require.Equal(t, "idle", f.actions.State(f.world, f.owner).Phase)
			f.tick()
			require.Len(t, f.sender.attacks, 1)
		})
	}
	f := newMeleeFixture(t)
	f.start("axe_sweep", 0)
	f.finish()
	require.Equal(t, 940.0, f.stamina())
	require.Len(t, f.sender.attacks, 1)
	require.Empty(t, f.sender.attacks[0].hits)
	require.Equal(t, 50.0, f.health().SHP, "self never receives melee damage")
}

func TestMeleeChopAnimationUsesAcceptedDirectionAndTimedLifecycle(t *testing.T) {
	animations, err := actionanimationdefs.LoadFromDirectory("../../data/action_animations", nil)
	require.NoError(t, err)
	previous := actionanimationdefs.Global()
	actionanimationdefs.SetGlobalForTesting(animations)
	t.Cleanup(func() { actionanimationdefs.SetGlobalForTesting(previous) })
	for _, id := range []string{"axe_sweep", "axe_strike"} {
		t.Run(id, func(t *testing.T) {
			f := newMeleeFixture(t)
			ecs.AddComponent(f.world, f.owner, components.Appearance{Resource: "player"})
			ecs.GetResource[ecs.TimeState](f.world).TickPeriod = 100 * time.Millisecond
			f.start(id, -math.Pi/2)
			state, err := cyclicaction.Snapshot(f.world, f.owner)
			require.NoError(t, err)
			binding, exists := animations.Get(state.AnimationKey)
			require.True(t, exists)
			require.Equal(t, []string{"chop_r", "chop_l"}, []string{binding.Variants[0].Clip, binding.Variants[1].Clip})
			require.NotNil(t, state.FacingAngle)
			require.InDelta(t, 1.5*math.Pi, *state.FacingAngle, 1e-6)
			require.Nil(t, state.TargetPosition)
			require.Equal(t, uint32(6), state.TotalTicks)
			ecs.WithComponent(f.world, f.owner, func(transform *components.Transform) { transform.X += 10 })
			for range 5 {
				f.tick()
			}
			next, err := cyclicaction.Snapshot(f.world, f.owner)
			require.NoError(t, err)
			require.Equal(t, state.FacingAngle, next.FacingAngle)
			require.Equal(t, uint32(5), next.ElapsedTicks)
			f.tick()
			idle, err := cyclicaction.Snapshot(f.world, f.owner)
			require.NoError(t, err)
			require.Empty(t, idle.AnimationKey)
			require.Nil(t, idle.FacingAngle)
			require.Len(t, f.sender.attacks, 1)
		})
	}
}

func TestMeleeNearestEligibilityAndDeterminism(t *testing.T) {
	f := newMeleeFixture(t)
	dead := f.creature(t, 10, 53, 50, false)
	ecs.WithComponent(f.world, dead, func(h *components.EntityHealth) { h.SHP = 0; h.HHP = 0 })
	first := f.object(t, 20, 60, 50, 100)
	f.creature(t, 30, 60, 50, false)
	f.start("axe_strike", 0)
	f.finish()
	require.Len(t, f.sender.attacks[0].hits, 1)
	require.Equal(t, uint64(20), f.sender.attacks[0].hits[0].id)
	health, _ := ecs.GetComponent[components.ObjectInternalState](f.world, first)
	require.Equal(t, 91.0, health.HP)
}

func TestMeleeSweepPlansAllBeforeCommit(t *testing.T) {
	f := newMeleeFixture(t)
	creature := f.creature(t, 10, 55, 50, false)
	first := f.object(t, 20, 60, 50, 2)
	second := f.object(t, 30, 62, 50, 2)
	f.objects.destruction.deps.Quarantine = func(h types.Handle) {
		for _, other := range []types.Handle{first, second} {
			health, _ := ecs.GetComponent[components.ObjectInternalState](f.world, other)
			require.Zero(t, health.HP)
			require.True(t, f.objects.state.Pending[other])
		}
		require.Equal(t, "idle", f.actions.State(f.world, f.owner).Phase)
		f.quarantined = append(f.quarantined, h)
	}
	f.start("axe_sweep", 0)
	f.finish()
	require.Len(t, f.sender.attacks[0].hits, 3)
	require.Equal(t, []uint64{10, 20, 30}, []uint64{f.sender.attacks[0].hits[0].id, f.sender.attacks[0].hits[1].id, f.sender.attacks[0].hits[2].id})
	health, _ := ecs.GetComponent[components.EntityHealth](f.world, creature)
	require.Equal(t, 19.0, health.SHP)
	require.Equal(t, 2, f.chunks.pins)
	require.Len(t, f.quarantined, 2)
}

func TestMeleeFailuresLeaveNoPartialDamage(t *testing.T) {
	for _, failure := range []string{"invalidLastHealth", "secondPin", "queueFull", "normalizedStamina", "cooldownOverflow", "knockoutOverflow", "negativeTime", "eventOverflow"} {
		t.Run(failure, func(t *testing.T) {
			f := newMeleeFixture(t)
			creature := f.creature(t, 10, 55, 50, false)
			first := f.object(t, 20, 60, 50, 2)
			second := f.object(t, 30, 62, 50, 2)
			f.start("axe_sweep", 0)
			switch failure {
			case "invalidLastHealth":
				ecs.WithComponent(f.world, second, func(h *components.ObjectInternalState) { h.HP = math.NaN() })
			case "secondPin":
				f.chunks.failAt = 2
			case "queueFull":
				f.objects.destruction.count = ObjectDestructionQueueCapacity
				f.objects.destruction.freeCount = 0
			case "normalizedStamina":
				ecs.WithComponent(f.world, f.owner, func(p *components.CharacterProfile) { p.Attributes[characterattrs.CON] = 0 })
				// A large raw value remains affordable at CON=1; force a valid
				// cost above normalized max while preserving raw admission.
				definition, _ := f.definitions.Get("axe_sweep")
				definition.Execution.Stamina = 1100
				ecs.WithComponent(f.world, f.owner, func(s *components.EntityStats) { s.Stamina = 2000 })
			case "cooldownOverflow":
				ecs.GetResource[ecs.TimeState](f.world).UnixMs = math.MaxInt64 - 1000
			case "knockoutOverflow":
				last := f.creature(t, 40, 63, 50, false)
				ecs.WithComponent(f.world, last, func(h *components.EntityHealth) { h.SHP = 1 })
				ecs.GetResource[ecs.TimeState](f.world).UnixMs = math.MaxInt64 - 5000
			case "negativeTime":
				ecs.GetResource[ecs.TimeState](f.world).UnixMs = -1000
			case "eventOverflow":
				f.execution.events.next.Store(math.MaxUint64)
			}
			beforeStamina := f.stamina()
			f.finish()
			health, _ := ecs.GetComponent[components.EntityHealth](f.world, creature)
			require.Equal(t, 25.0, health.SHP)
			state, _ := ecs.GetComponent[components.ObjectInternalState](f.world, first)
			require.Equal(t, 2.0, state.HP)
			require.False(t, f.objects.state.Pending[first])
			require.False(t, f.objects.state.Pending[second])
			require.Equal(t, beforeStamina, f.stamina())
			if cooldowns, exists := ecs.GetComponent[components.ActionCooldowns](f.world, f.owner); exists {
				require.Empty(t, cooldowns.ByAction)
			}
			require.Empty(t, f.sender.attacks)
			require.Empty(t, f.quarantined)
			require.Zero(t, f.chunks.pins)
			require.False(t, f.execution.active)
		})
	}
}

func TestMeleeSecondReservationFailureCanRetryExactlyOnce(t *testing.T) {
	f := newMeleeFixture(t)
	first := f.object(t, 20, 60, 50, 2)
	second := f.object(t, 30, 62, 50, 2)
	f.chunks.failAt = 2
	f.start("axe_sweep", 0)
	f.finish()
	require.Empty(t, f.sender.attacks)
	require.Zero(t, f.chunks.pins)
	require.Equal(t, ObjectDestructionQueueCapacity, f.objects.destruction.freeCount)
	f.chunks.failAt = 0
	f.start("axe_sweep", 0)
	f.finish()
	require.Len(t, f.sender.attacks, 1)
	require.Len(t, f.sender.attacks[0].hits, 2)
	require.Equal(t, 940.0, f.stamina())
	require.Equal(t, 2, f.chunks.pins)
	for _, target := range []types.Handle{first, second} {
		state, _ := ecs.GetComponent[components.ObjectInternalState](f.world, target)
		require.Zero(t, state.HP)
		require.True(t, f.objects.state.Pending[target])
	}
	f.tick()
	require.Len(t, f.sender.attacks, 1)
}

func TestMeleeZeroDamageStillReportsContact(t *testing.T) {
	f := newMeleeFixture(t)
	target := f.creature(t, 3, 60, 50, true)
	definition, _ := f.definitions.Get("axe_sweep")
	// A numerically valid tiny Draw underflows to zero after armor. This tests
	// the receiver boundary without adding a zero-damage balance definition.
	definition.Combat.DamageMultiplier = math.SmallestNonzeroFloat64
	prepared, err := f.catalog.PrepareMeleeAction(definition)
	require.NoError(t, err)
	f.actions.handlers[definition.ID], err = NewMeleeActionHandler(f.execution, definition, prepared)
	require.NoError(t, err)
	f.start("axe_sweep", 0)
	f.finish()
	require.Len(t, f.sender.attacks, 1)
	require.Equal(t, []meleeHitSample{{id: 3, damage: 0}}, f.sender.attacks[0].hits)
	health, _ := ecs.GetComponent[components.EntityHealth](f.world, target)
	require.Equal(t, components.EntityHealth{SHP: 25, HHP: 25}, health)
	require.Equal(t, 940.0, f.stamina())
}

func TestMeleeCancellationAndLiveInputs(t *testing.T) {
	for _, cancel := range []string{"movement", "equipment", "knockout", "stun", "explicit"} {
		t.Run(cancel, func(t *testing.T) {
			f := newMeleeFixture(t)
			target := f.creature(t, 3, 60, 50, false)
			f.start("axe_strike", 0)
			f.tick()
			stale, _ := ecs.GetComponent[components.ActiveCyclicAction](f.world, f.owner)
			switch cancel {
			case "movement":
				f.actions.CancelForPointMovement(f.world, 1, f.owner)
			case "equipment":
				f.equip()
			case "knockout":
				ecs.WithComponent(f.world, f.owner, func(h *components.EntityHealth) { h.KOUntilUnixMs = 100000; h.IsLying = true })
			case "stun":
				ecs.AddComponent(f.world, f.owner, components.Movement{State: constt.StateStunned})
			case "explicit":
				f.actions.Cancel(f.world, 1, f.owner)
			}
			f.finish()
			f.actions.AdvanceCycle(f.world, 1, f.owner, stale, f.sender)
			health, _ := ecs.GetComponent[components.EntityHealth](f.world, target)
			require.Equal(t, 25.0, health.SHP)
			require.Equal(t, 1000.0, f.stamina())
			require.Empty(t, f.sender.attacks)
		})
	}
	f := newMeleeFixture(t)
	target := f.creature(t, 3, 60, 50, false)
	f.start("axe_strike", 0)
	// Move victim out of fixed aim, change quality and STR before completion.
	ecs.WithComponent(f.world, target, func(p *components.Transform) { p.Y = 80 })
	f.equip(combatTestItem(100, 1, 160, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND))
	ecs.WithComponent(f.world, f.owner, func(p *components.CharacterProfile) { p.Attributes[characterattrs.STR] = 16 })
	f.finish()
	require.Empty(t, f.sender.attacks[0].hits)
	clock := ecs.GetResource[ecs.TimeState](f.world)
	clock.UnixMs += 2001
	ecs.WithComponent(f.world, target, func(p *components.Transform) { p.Y = 50 })
	f.start("axe_strike", 0)
	f.finish()
	require.Equal(t, 36.0, f.sender.attacks[1].hits[0].damage)
}

func TestMeleeHitBudgetAndPreparedAllocations(t *testing.T) {
	f := newMeleeFixture(t)
	for i := 0; i < MeleeMaxHits; i++ {
		f.creature(t, types.EntityID(100+i), 60, 50, false)
	}
	definition, _ := f.definitions.Get("axe_sweep")
	prepared, err := f.catalog.PrepareMeleeAction(definition)
	require.NoError(t, err)
	require.NoError(t, f.execution.prepare(f.owner, 1, definition, prepared, 0))
	require.Equal(t, MeleeMaxHits, f.execution.count)
	f.execution.abort()
	require.Zero(t, testing.AllocsPerRun(100, func() {
		if err := f.execution.prepare(f.owner, 1, definition, prepared, 0); err != nil {
			panic(err)
		}
		f.execution.abort()
	}))
	f.creature(t, 999, 60, 50, false)
	require.ErrorIs(t, f.execution.prepare(f.owner, 1, definition, prepared, 0), ErrMeleeExecutionBusy)
	f.execution.abort()
	require.Zero(t, testing.AllocsPerRun(100, func() {
		if err := f.execution.prepare(f.owner, 1, definition, prepared, 0); err != ErrMeleeExecutionBusy {
			panic("expected limit")
		}
		f.execution.abort()
	}))
	f.start("axe_sweep", 0)
	f.finish()
	require.Equal(t, 1000.0, f.stamina())
	require.Empty(t, f.sender.attacks)
}

func TestMeleeSpatialVisitBudgetFailsWholeTimedAction(t *testing.T) {
	for _, overflow := range []bool{false, true} {
		t.Run(map[bool]string{false: "exact_limit", true: "overflow"}[overflow], func(t *testing.T) {
			f := newMeleeFixture(t)
			target := f.creature(t, 3, 60, 50, false)
			// Actor and victim each use four raw cell memberships. Remaining
			// contacts have no health model, but still consume spatial work.
			passive := MeleeMaxQueryVisits/4 - 2
			if overflow {
				passive++
			}
			for i := 0; i < passive; i++ {
				spawnSectorCollider(f.world, types.EntityID(100+i), 60, 50, 1, 1)
			}
			f.start("axe_sweep", 0)
			f.finish()
			health, _ := ecs.GetComponent[components.EntityHealth](f.world, target)
			if overflow {
				require.Equal(t, 25.0, health.SHP)
				require.Equal(t, 1000.0, f.stamina())
				require.Empty(t, f.sender.attacks)
			} else {
				require.Equal(t, 19.0, health.SHP)
				require.Equal(t, 940.0, f.stamina())
				require.Len(t, f.sender.attacks[0].hits, 1)
			}
			spatial := ecs.GetResource[*core.WorldColliderSpatial](f.world)
			require.Equal(t, MeleeMaxQueryVisits, (*spatial).Index.LastQueryStats().Visits)
		})
	}
}

func TestMeleeBootstrapAndPreparationLifecycle(t *testing.T) {
	f := newMeleeFixture(t)
	compiled, err := NewCombatDefinitions(itemdefs.NewRegistry(combatTestDefinitions()), f.definitions)
	require.NoError(t, err)
	require.Len(t, compiled.actions, 2)
	_, err = NewCombatDefinitions(nil, f.definitions)
	require.Error(t, err)
	bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1})
	t.Cleanup(func() { require.NoError(t, bus.Shutdown(context.Background())) })
	shard := &Shard{world: f.world, creatureDamage: f.service, logger: zap.NewNop(), eventBus: bus}
	h, err := shard.spawnPlayerLocked(999, 70, 50, func(w *ecs.World, h types.Handle) error {
		ecs.AddComponent(w, h, components.EntityHealth{SHP: 20, HHP: 20})
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, shard.prepareCreatureCombatTarget(h))
	require.True(t, f.service.activity.IsPrepared(h, 999))
	ecs.GetResource[ecs.DetachedEntities](f.world).AddDetachedEntity(999, h, time.Now().Add(time.Hour), time.Now())
	require.True(t, f.service.activity.IsPrepared(h, 999))
	f.world.Despawn(h)
	require.False(t, f.service.activity.IsPrepared(h, 999))
	_, err = shard.spawnPlayerLocked(1000, 70, 50, func(w *ecs.World, h types.Handle) error {
		ecs.AddComponent(w, h, components.EntityHealth{SHP: math.NaN(), HHP: 20})
		return nil
	})
	require.Error(t, err)
	require.Equal(t, types.InvalidHandle, f.world.GetHandleByEntityID(1000))
}
