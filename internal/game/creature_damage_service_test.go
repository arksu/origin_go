package game

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"origin/internal/characterattrs"
	"origin/internal/combat"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/playerstate"
	"origin/internal/types"
)

type creatureDamageFixture struct {
	*combatEquipmentFixture
	service *CreatureDamageService
	stats   *ecs.EntityStatsUpdateState
	visual  *ecs.CharacterVisualDirtyQueue
}

func newCreatureDamageFixture(t testing.TB) *creatureDamageFixture {
	t.Helper()
	return creatureDamageFixtureInWorld(t, ecs.NewWorldForTesting())
}

func creatureDamageFixtureInWorld(t testing.TB, world *ecs.World) *creatureDamageFixture {
	t.Helper()
	equipment := combatFixtureInWorld(t, world, combatTestDefinitions())
	ecs.AddComponent(world, equipment.owner, components.EntityHealth{SHP: 10, HHP: 50})
	ecs.AddComponent(world, equipment.owner, components.EntityStats{Energy: 900})
	attributes := characterattrs.Default()
	attributes[characterattrs.CON] = 4
	ecs.AddComponent(world, equipment.owner, components.CharacterProfile{Attributes: attributes})
	ecs.GetResource[ecs.TimeState](world).UnixMs = 1000
	service, err := NewCreatureDamageService(world, equipment.resolver)
	require.NoError(t, err)
	require.NoError(t, service.PrepareTarget(equipment.owner))
	return &creatureDamageFixture{
		combatEquipmentFixture: equipment, service: service,
		stats:  ecs.GetResource[ecs.EntityStatsUpdateState](world),
		visual: ecs.GetResource[ecs.CharacterVisualDirtyQueue](world),
	}
}

func (fixture *creatureDamageFixture) health() components.EntityHealth {
	health, _ := ecs.GetComponent[components.EntityHealth](fixture.world, fixture.owner)
	return health
}

func (fixture *creatureDamageFixture) setHealth(health components.EntityHealth) {
	ecs.AddComponent(fixture.world, fixture.owner, health)
}

func (fixture *creatureDamageFixture) assertRejected(t *testing.T, draw float64, expected error) {
	t.Helper()
	before := fixture.health()
	movement, _ := ecs.GetComponent[components.Movement](fixture.world, fixture.owner)
	statsPending, visualPending := fixture.stats.PendingPlayerPushCount(), fixture.visual.PendingCount()
	result, err := fixture.service.Apply(fixture.owner, draw)
	if expected == nil {
		require.Error(t, err)
	} else {
		require.ErrorIs(t, err, expected)
	}
	require.Zero(t, result)
	after := fixture.health()
	require.Equal(t, math.Float64bits(before.SHP), math.Float64bits(after.SHP))
	require.Equal(t, math.Float64bits(before.HHP), math.Float64bits(after.HHP))
	before.SHP, before.HHP, after.SHP, after.HHP = 0, 0, 0, 0
	require.Equal(t, before, after)
	actualMovement, _ := ecs.GetComponent[components.Movement](fixture.world, fixture.owner)
	require.Equal(t, movement, actualMovement)
	require.Equal(t, statsPending, fixture.stats.PendingPlayerPushCount())
	require.Equal(t, visualPending, fixture.visual.PendingCount())
}

func TestCreatureDamageServiceConstructionAndPreparation(t *testing.T) {
	fixture := newCreatureDamageFixture(t)
	for _, test := range []struct {
		world    *ecs.World
		resolver *CombatEquipmentResolver
	}{
		{nil, fixture.resolver}, {fixture.world, nil}, {ecs.NewWorldForTesting(), fixture.resolver},
		{fixture.world, &CombatEquipmentResolver{}},
	} {
		service, err := NewCreatureDamageService(test.world, test.resolver)
		require.ErrorIs(t, err, ErrInvalidCreatureDamageService)
		require.Nil(t, service)
	}
	var nilService *CreatureDamageService
	result, err := nilService.Apply(fixture.owner, 1)
	require.ErrorIs(t, err, ErrInvalidCreatureDamageService)
	require.Zero(t, result)
	require.ErrorIs(t, nilService.PrepareTarget(fixture.owner), ErrInvalidCreatureDamageService)
	require.NoError(t, fixture.service.PrepareTarget(fixture.owner), "preparation is idempotent")
	require.True(t, fixture.stats.IsPlayerPrepared(1, fixture.owner))
	require.True(t, fixture.visual.IsPrepared(fixture.owner))

	for _, handle := range []types.Handle{types.InvalidHandle, fixture.world.SpawnWithoutExternalID(), fixture.world.Spawn(42, nil)} {
		require.Error(t, fixture.service.PrepareTarget(handle))
	}
	fixture.setHealth(components.EntityHealth{SHP: 0, HHP: 0})
	require.ErrorIs(t, fixture.service.PrepareTarget(fixture.owner), ErrCreatureTargetDead)
}

func TestCreatureDamageServiceDistribution(t *testing.T) {
	for _, test := range []struct {
		name       string
		before     components.EntityHealth
		draw       float64
		soft, hard float64
		shp, hhp   float64
		enteredKO  bool
		dead       bool
	}{
		{"ordinary", components.EntityHealth{SHP: 40, HHP: 60}, 10, 10, 2, 30, 58, false, false},
		{"overflow", components.EntityHealth{SHP: 10, HHP: 50}, 30, 10, 22, 0, 28, true, false},
		{"active KO with regenerated SHP", components.EntityHealth{SHP: 5, HHP: 28, KOUntilUnixMs: 61000, IsLying: true}, 12, 0, 12, 5, 16, false, false},
		{"lying alone", components.EntityHealth{SHP: 5, HHP: 20, IsLying: true}, 2, 2, .4, 3, 19.6, false, false},
		{"lying repeated KO", components.EntityHealth{SHP: 1, HHP: 20, IsLying: true}, 2, 1, 1.2, 0, 18.8, true, false},
		{"direct death", components.EntityHealth{SHP: 10, HHP: 20}, 30, 10, 22, 0, 0, false, true},
		{"active KO clamps regenerated SHP", components.EntityHealth{SHP: 20, HHP: 20, KOUntilUnixMs: 61000, IsLying: true}, 5, 0, 5, 15, 15, false, false},
		{"fractional living HHP", components.EntityHealth{SHP: .49, HHP: .49}, .1, .1, .02, .39, .47, false, false},
		{"maximum finite hit", components.EntityHealth{SHP: 10, HHP: 50}, math.MaxFloat64, 10, math.MaxFloat64, 0, 0, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCreatureDamageFixture(t)
			fixture.setHealth(test.before)
			result, err := fixture.service.Apply(fixture.owner, test.draw)
			require.NoError(t, err)
			require.Equal(t, test.before, result.Before)
			require.Zero(t, result.Armor)
			require.Equal(t, test.draw, result.Damage)
			require.InDelta(t, test.soft, result.SoftDamage, 1e-14)
			if test.hard == math.MaxFloat64 {
				require.Equal(t, test.hard, result.HardDamage)
			} else {
				require.InDelta(t, test.hard, result.HardDamage, 1e-14)
			}
			require.InDelta(t, test.shp, result.After.SHP, 1e-14)
			require.InDelta(t, test.hhp, result.After.HHP, 1e-14)
			require.Equal(t, test.enteredKO, result.EnteredKO)
			require.Equal(t, test.dead, result.Dead)
			require.Equal(t, result.After, fixture.health())
			if test.enteredKO {
				require.Equal(t, int64(61000), result.After.KOUntilUnixMs)
				require.True(t, result.After.IsLying)
			}
			if test.dead {
				require.Zero(t, result.After.KOUntilUnixMs)
				fixture.assertRejected(t, 1, ErrCreatureTargetDead)
			}
		})
	}
}

func TestCreatureDamageServiceSequentialHitsObserveCommittedKO(t *testing.T) {
	fixture := newCreatureDamageFixture(t)
	first, err := fixture.service.Apply(fixture.owner, 10)
	require.NoError(t, err)
	require.True(t, first.EnteredKO)
	require.Equal(t, 48.0, first.After.HHP)
	second, err := fixture.service.Apply(fixture.owner, 10)
	require.NoError(t, err)
	require.Equal(t, first.After, second.Before)
	require.False(t, second.EnteredKO)
	require.Zero(t, second.SoftDamage)
	require.Equal(t, 38.0, second.After.HHP)
	require.Equal(t, first.After.KOUntilUnixMs, second.After.KOUntilUnixMs)
	require.Equal(t, first.After.LyingRevision, second.After.LyingRevision)
	require.Equal(t, 1, fixture.stats.PendingPlayerPushCount())
	require.Equal(t, 1, fixture.visual.PendingCount())
}

func TestCreatureDamageServiceKnockoutDeadline(t *testing.T) {
	for _, now := range []int64{59999, 60000, 60001} {
		t.Run(time.UnixMilli(now).String(), func(t *testing.T) {
			fixture := newCreatureDamageFixture(t)
			fixture.setHealth(components.EntityHealth{SHP: 5, HHP: 20, KOUntilUnixMs: 60000, IsLying: true, LyingRevision: 7})
			ecs.GetResource[ecs.TimeState](fixture.world).UnixMs = now
			result, err := fixture.service.Apply(fixture.owner, 2)
			require.NoError(t, err)
			if now <= 60000 {
				require.Zero(t, result.SoftDamage)
				require.Equal(t, 18.0, result.After.HHP)
				require.Equal(t, int64(60000), result.After.KOUntilUnixMs)
			} else {
				require.Equal(t, 2.0, result.SoftDamage)
				require.Equal(t, 3.0, result.After.SHP)
				require.Equal(t, 19.6, result.After.HHP)
				require.Zero(t, result.After.KOUntilUnixMs)
			}
			require.Equal(t, uint64(7), result.After.LyingRevision)
			require.True(t, result.After.IsLying)
		})
	}
	fixture := newCreatureDamageFixture(t)
	fixture.setHealth(components.EntityHealth{HHP: 20, KOUntilUnixMs: 60000, IsLying: true, LyingRevision: 3})
	ecs.GetResource[ecs.TimeState](fixture.world).UnixMs = 60001
	result, err := fixture.service.Apply(fixture.owner, 2)
	require.NoError(t, err)
	require.Equal(t, 1.0, result.SoftDamage, "strictly expired KO grants its one-time minimum before this later hit")
	require.Equal(t, 18.8, result.After.HHP)
	require.Zero(t, result.After.SHP)
	require.True(t, result.EnteredKO)
	require.Equal(t, int64(120001), result.After.KOUntilUnixMs)
	require.Equal(t, uint64(3), result.After.LyingRevision, "continuing to lie does not change visual revision")
}

func TestCreatureDamageServiceDeathAtDeadlineHasPriorityAndRunsOnce(t *testing.T) {
	for _, hhp := range []float64{.49, 20} {
		fixture := newCreatureDamageFixture(t)
		fixture.setHealth(components.EntityHealth{HHP: hhp, KOUntilUnixMs: 60000, IsLying: true})
		ecs.GetResource[ecs.TimeState](fixture.world).UnixMs = 60000
		ecs.GetResource[ecs.TimeState](fixture.world).Tick = 1
		ecs.GetResource[ecs.CharacterEntities](fixture.world).Add(1, fixture.owner, time.UnixMilli(60000))
		handler := &testPlayerDeathHandler{}
		system := NewPlayerDeathSystem(handler, PlayerDeathSystemConfig{})
		result, err := fixture.service.Apply(fixture.owner, hhp)
		require.NoError(t, err)
		require.True(t, result.Dead)
		require.Zero(t, result.After.SHP)
		require.Zero(t, result.After.KOUntilUnixMs)
		require.Empty(t, handler.calls, "damage commit does not execute the death handler")
		system.Update(fixture.world, 0)
		system.Update(fixture.world, 0)
		require.Len(t, handler.calls, 1)
		require.Equal(t, fixture.owner, handler.calls[0].handle)
		require.Zero(t, fixture.health().SHP, "completion cannot restore a dead target")
	}
}

func TestCreatureDamageServiceLiveArmorAndDefinitionComposition(t *testing.T) {
	fixture := newCreatureDamageFixture(t)
	for _, test := range []struct {
		items  []components.InvItem
		draw   float64
		armor  float64
		damage float64
	}{
		{nil, 6, 0, 6},
		{[]components.InvItem{combatTestItem(100, 7, 10, netproto.EquipSlot_EQUIP_SLOT_CHEST)}, 6, 4, 3.6},
		{[]components.InvItem{combatTestItem(100, 7, 10, netproto.EquipSlot_EQUIP_SLOT_CHEST)}, 9, 4, 81.0 / 13},
		{[]components.InvItem{combatTestItem(100, 7, 40, netproto.EquipSlot_EQUIP_SLOT_CHEST)}, 6, 8, 36.0 / 14},
		{[]components.InvItem{combatTestItem(100, 7, 10, netproto.EquipSlot_EQUIP_SLOT_CHEST), combatTestItem(101, 8, 10, netproto.EquipSlot_EQUIP_SLOT_HEAD)}, 6, 12, 2},
	} {
		fixture.setHealth(components.EntityHealth{SHP: 10, HHP: 50})
		fixture.equip(test.items...)
		result, err := fixture.service.Apply(fixture.owner, test.draw)
		require.NoError(t, err)
		require.Equal(t, test.armor, result.Armor)
		require.InDelta(t, test.damage, result.Damage, 1e-14)
	}
	// The same melee selector and damage API compose with every supported type.
	attacker := fixture.world.Spawn(2, nil)
	container := fixture.world.SpawnWithoutExternalID()
	ecs.AddComponent(fixture.world, container, components.InventoryContainer{OwnerID: 2, Kind: constt.InventoryEquipment})
	ecs.GetResource[ecs.InventoryRefIndex](fixture.world).Add(constt.InventoryEquipment, 2, 0, container)
	for _, typeID := range []uint32{1, 2, 3, 4} {
		ecs.WithComponent(fixture.world, container, func(inventory *components.InventoryContainer) {
			inventory.Items = []components.InvItem{combatTestItem(200, typeID, 10, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND)}
		})
		weapon, err := fixture.resolver.ResolveMeleeWeapon(attacker, fixture.action, 1.25)
		require.NoError(t, err)
		fixture.setHealth(components.EntityHealth{SHP: 10, HHP: 50})
		result, err := fixture.service.Apply(fixture.owner, weapon.RawDamage)
		require.NoError(t, err)
		expected, err := combat.DamageAfterArmor(weapon.RawDamage, 12)
		require.NoError(t, err)
		require.Equal(t, expected, result.Damage)
	}
}

func TestCreatureDamageServiceRejectsInvalidStateAtomically(t *testing.T) {
	for _, draw := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		fixture := newCreatureDamageFixture(t)
		fixture.assertRejected(t, draw, combat.ErrInvalidInput)
	}
	for _, health := range []components.EntityHealth{
		{SHP: -1, HHP: 50}, {SHP: math.NaN(), HHP: 50}, {SHP: math.Inf(1), HHP: 50},
		{SHP: 10, HHP: -1}, {SHP: 10, HHP: math.NaN()}, {SHP: 10, HHP: math.Inf(1)},
		{SHP: 51, HHP: 50}, {SHP: 10, HHP: 50, KOUntilUnixMs: -1},
	} {
		fixture := newCreatureDamageFixture(t)
		fixture.setHealth(health)
		fixture.assertRejected(t, 1, ErrInvalidCreatureHealth)
		fixture.assertRejected(t, 0, ErrInvalidCreatureHealth)
	}
	for _, change := range []func(*creatureDamageFixture){
		func(f *creatureDamageFixture) {
			f.equip(combatTestItem(100, 7, 0, netproto.EquipSlot_EQUIP_SLOT_CHEST))
		},
		func(f *creatureDamageFixture) {
			f.equip(combatTestItem(100, 999, 10, netproto.EquipSlot_EQUIP_SLOT_CHEST))
		},
		func(f *creatureDamageFixture) {
			f.equip(combatTestItem(100, 7, 10, netproto.EquipSlot_EQUIP_SLOT_HEAD))
		},
		func(f *creatureDamageFixture) { f.world.Despawn(f.container) },
		func(f *creatureDamageFixture) {
			f.equip(combatTestItem(100, 7, 10, netproto.EquipSlot_EQUIP_SLOT_CHEST), combatTestItem(100, 8, 10, netproto.EquipSlot_EQUIP_SLOT_HEAD))
		},
	} {
		fixture := newCreatureDamageFixture(t)
		change(fixture)
		fixture.assertRejected(t, 1, nil)
		fixture.assertRejected(t, 0, nil)
	}
	fixture := newCreatureDamageFixture(t)
	fixture.world.AddComponentObserver(components.EntityHealthComponentID, func(types.Handle) { t.Fatal("rejected damage notified an observer") })
	fixture.assertRejected(t, math.NaN(), combat.ErrInvalidInput)
}

func TestCreatureDamageServiceTimeValidation(t *testing.T) {
	for _, now := range []int64{-1, math.MaxInt64 - playerstate.KnockoutDurationMs + 1} {
		fixture := newCreatureDamageFixture(t)
		ecs.GetResource[ecs.TimeState](fixture.world).UnixMs = now
		fixture.assertRejected(t, 10, ErrInvalidCreatureDamageTime)
	}
	fixture := newCreatureDamageFixture(t)
	ecs.GetResource[ecs.TimeState](fixture.world).UnixMs = math.MaxInt64 - playerstate.KnockoutDurationMs
	result, err := fixture.service.Apply(fixture.owner, 10)
	require.NoError(t, err)
	require.Equal(t, int64(math.MaxInt64), result.After.KOUntilUnixMs)
}

func TestCreatureDamageServicePreparationReleaseAndGeneration(t *testing.T) {
	for _, release := range []func(*creatureDamageFixture){
		func(f *creatureDamageFixture) { f.stats.ReleasePlayer(1, f.owner) },
		func(f *creatureDamageFixture) { f.visual.Forget(f.owner) },
	} {
		fixture := newCreatureDamageFixture(t)
		release(fixture)
		fixture.assertRejected(t, 1, ErrCreatureTargetUnprepared)
		require.NoError(t, fixture.service.PrepareTarget(fixture.owner))
		_, err := fixture.service.Apply(fixture.owner, 1)
		require.NoError(t, err)
	}
	fixture := newCreatureDamageFixture(t)
	stale := fixture.owner
	fixture.world.Despawn(stale)
	require.False(t, fixture.stats.IsPlayerPrepared(1, stale))
	require.False(t, fixture.visual.IsPrepared(stale))
	replacement := fixture.world.Spawn(1, nil)
	require.Equal(t, stale.Index(), replacement.Index())
	require.NotEqual(t, stale, replacement)
	ecs.AddComponent(fixture.world, replacement, components.EntityHealth{SHP: 12, HHP: 24})
	result, err := fixture.service.Apply(stale, 1)
	require.ErrorIs(t, err, ErrInvalidCreatureTarget)
	require.Zero(t, result)
	result, err = fixture.service.Apply(replacement, 1)
	require.ErrorIs(t, err, ErrCreatureTargetUnprepared)
	require.Zero(t, result)
	require.NoError(t, fixture.service.PrepareTarget(replacement))
	result, err = fixture.service.Apply(replacement, 1)
	require.NoError(t, err)
	require.Equal(t, 11.0, result.After.SHP)

	fixture = newCreatureDamageFixture(t)
	ecs.WithComponent(fixture.world, fixture.owner, func(identity *ecs.ExternalID) { identity.ID = 42 })
	fixture.assertRejected(t, 1, ErrInvalidCreatureTarget)
}

func TestCreatureDamageServiceZeroDamageAndObservers(t *testing.T) {
	fixture := newCreatureDamageFixture(t)
	fixture.setHealth(components.EntityHealth{HHP: 20, KOUntilUnixMs: 60000, IsLying: true})
	ecs.GetResource[ecs.TimeState](fixture.world).UnixMs = 60001
	before := fixture.health()
	observations := 0
	var observedHealth components.EntityHealth
	fixture.world.AddComponentObserver(components.EntityHealthComponentID, func(handle types.Handle) {
		require.Equal(t, fixture.owner, handle)
		observations++
		observedHealth = fixture.health()
	})
	result, err := fixture.service.Apply(fixture.owner, 0)
	require.NoError(t, err)
	require.Equal(t, before, result.Before)
	require.Equal(t, before, result.After)
	require.Equal(t, before, fixture.health())
	require.Zero(t, observations)
	require.Zero(t, fixture.stats.PendingPlayerPushCount())
	require.Zero(t, fixture.visual.PendingCount())
	result, err = fixture.service.Apply(fixture.owner, 2)
	require.NoError(t, err)
	require.Equal(t, 1, observations, "observer sees one complete health commit")
	require.Equal(t, result.After, observedHealth)
	require.Equal(t, result.After, fixture.health())
}

func TestCreatureDamageServiceMovementAndDirtyNotifications(t *testing.T) {
	for _, state := range []constt.MoveState{constt.StateMoving, constt.StateStunned} {
		fixture := newCreatureDamageFixture(t)
		ecs.AddComponent(fixture.world, fixture.owner, components.Movement{State: state, TargetType: constt.TargetPoint, TargetX: 42, VelocityX: 3})
		fixture.stats.MarkPlayerSent(1, 1000)
		_, err := fixture.service.Apply(fixture.owner, 10)
		require.NoError(t, err)
		movement, _ := ecs.GetComponent[components.Movement](fixture.world, fixture.owner)
		require.Equal(t, constt.TargetNone, movement.TargetType)
		require.Zero(t, movement.VelocityX)
		require.True(t, movement.Direction.UpdatePending)
		if state == constt.StateStunned {
			require.Equal(t, constt.StateStunned, movement.State)
		} else {
			require.Equal(t, constt.StateIdle, movement.State)
		}
		require.Equal(t, []types.EntityID{1}, fixture.stats.PopDuePlayerStatsPush(1000, make([]types.EntityID, 0, 1)), "KO notification bypasses health throttle")
		require.Equal(t, []types.Handle{fixture.owner}, fixture.visual.Drain(1, make([]types.Handle, 0, 1)))
	}
}

func TestCreatureDamageServiceArmorOverflowIsAtomic(t *testing.T) {
	definitions := combatTestDefinitions()
	definitions[6].Armor = &itemdefs.ArmorDef{BaseArmor: math.MaxFloat64}
	fixture := newCombatEquipmentFixture(t, definitions)
	ecs.AddComponent(fixture.world, fixture.owner, components.EntityHealth{SHP: 10, HHP: 50})
	service, err := NewCreatureDamageService(fixture.world, fixture.resolver)
	require.NoError(t, err)
	require.NoError(t, service.PrepareTarget(fixture.owner))
	fixture.equip(combatTestItem(100, 7, 40, netproto.EquipSlot_EQUIP_SLOT_CHEST))
	result, err := service.Apply(fixture.owner, 1)
	require.ErrorIs(t, err, combat.ErrNonFiniteResult)
	require.Zero(t, result)
	health, _ := ecs.GetComponent[components.EntityHealth](fixture.world, fixture.owner)
	require.Equal(t, components.EntityHealth{SHP: 10, HHP: 50}, health)
}

func TestCreatureDamageServiceDoesNotRequireCreatureProfile(t *testing.T) {
	fixture := newCreatureDamageFixture(t)
	ecs.RemoveComponent[components.CharacterProfile](fixture.world, fixture.owner)
	ecs.RemoveComponent[components.EntityStats](fixture.world, fixture.owner)
	_, err := fixture.service.Apply(fixture.owner, 1)
	require.NoError(t, err, "the receiver does not invent attributes for future creatures")
}

func TestCreatureDamageServiceHealthRemovalRetiresPreparation(t *testing.T) {
	fixture := newCreatureDamageFixture(t)
	_, err := fixture.service.Apply(fixture.owner, 10)
	require.NoError(t, err)
	require.True(t, fixture.stats.IsPlayerPrepared(1, fixture.owner))
	require.True(t, fixture.visual.IsPrepared(fixture.owner))
	ecs.RemoveComponent[components.EntityHealth](fixture.world, fixture.owner)
	require.False(t, fixture.stats.IsPlayerPrepared(1, fixture.owner))
	require.False(t, fixture.visual.IsPrepared(fixture.owner))
	require.Zero(t, fixture.stats.PendingPlayerPushCount())
	require.Zero(t, fixture.visual.PendingCount())
	result, err := fixture.service.Apply(fixture.owner, 1)
	require.ErrorIs(t, err, ErrInvalidCreatureTarget)
	require.Zero(t, result)
	fixture.setHealth(components.EntityHealth{SHP: 10, HHP: 50})
	fixture.assertRejected(t, 1, ErrCreatureTargetUnprepared)
	require.NoError(t, fixture.service.PrepareTarget(fixture.owner))
	_, err = fixture.service.Apply(fixture.owner, 1)
	require.NoError(t, err)
}

func TestCreatureDamageServiceVisualRevisionOverflowIsAtomic(t *testing.T) {
	fixture := newCreatureDamageFixture(t)
	fixture.setHealth(components.EntityHealth{SHP: 10, HHP: 50, LyingRevision: math.MaxUint64})
	fixture.assertRejected(t, 10, ErrInvalidCreatureHealth)
	fixture.setHealth(components.EntityHealth{SHP: 10, HHP: 50, IsLying: true, LyingRevision: math.MaxUint64})
	result, err := fixture.service.Apply(fixture.owner, 10)
	require.NoError(t, err, "an unchanged pose does not increment the revision")
	require.Equal(t, uint64(math.MaxUint64), result.After.LyingRevision)

	fixture.setHealth(components.EntityHealth{SHP: 5, HHP: 20, KOUntilUnixMs: 60000, LyingRevision: math.MaxUint64})
	ecs.GetResource[ecs.TimeState](fixture.world).UnixMs = 60001
	fixture.assertRejected(t, 1, ErrInvalidCreatureHealth)
}

var (
	creatureDamageAllocationResult CreatureDamageResult
	creatureDamageAllocationError  error
)

func TestCreatureDamageServiceAllocations(t *testing.T) {
	for _, test := range []struct {
		name       string
		health     components.EntityHealth
		movement   components.Movement
		draw       float64
		pending    bool
		expectErr  error
		invalidate func(*creatureDamageFixture)
	}{
		{name: "ordinary fresh dirty", health: components.EntityHealth{SHP: 10, HHP: 50}, draw: 1},
		{name: "ordinary already dirty", health: components.EntityHealth{SHP: 10, HHP: 50}, draw: 1, pending: true},
		{name: "KO moving fresh", health: components.EntityHealth{SHP: 10, HHP: 50}, movement: components.Movement{State: constt.StateMoving, TargetType: constt.TargetPoint, VelocityX: 3}, draw: 10},
		{name: "KO stunned fresh", health: components.EntityHealth{SHP: 10, HHP: 50}, movement: components.Movement{State: constt.StateStunned, TargetType: constt.TargetPoint, VelocityX: 3}, draw: 10},
		{name: "dead fresh", health: components.EntityHealth{SHP: 10, HHP: 20}, draw: 30},
		{name: "expired KO", health: components.EntityHealth{HHP: 20, KOUntilUnixMs: 1, IsLying: true}, draw: 2},
		{name: "zero damage", health: components.EntityHealth{SHP: 10, HHP: 50}, draw: 0},
		{name: "invalid numeric input", health: components.EntityHealth{SHP: 10, HHP: 50}, draw: math.NaN(), expectErr: combat.ErrInvalidInput},
		{name: "invalid health", health: components.EntityHealth{SHP: math.NaN(), HHP: 50}, draw: 1, expectErr: ErrInvalidCreatureHealth},
		{name: "dead unavailable", health: components.EntityHealth{}, draw: 1, expectErr: ErrCreatureTargetDead},
		{name: "unprepared", health: components.EntityHealth{SHP: 10, HHP: 50}, draw: 1, expectErr: ErrCreatureTargetUnprepared, invalidate: func(f *creatureDamageFixture) { f.visual.Forget(f.owner) }},
		{name: "invalid equipment", health: components.EntityHealth{SHP: 10, HHP: 50}, draw: 1, expectErr: ErrInvalidCombatEquipment, invalidate: func(f *creatureDamageFixture) {
			f.equip(combatTestItem(100, 999, 10, netproto.EquipSlot_EQUIP_SLOT_CHEST))
		}},
		{name: "invalid armor quality", health: components.EntityHealth{SHP: 10, HHP: 50}, draw: 1, expectErr: combat.ErrInvalidInput, invalidate: func(f *creatureDamageFixture) {
			f.equip(combatTestItem(100, 7, 0, netproto.EquipSlot_EQUIP_SLOT_CHEST))
		}},
		{name: "KO deadline overflow", health: components.EntityHealth{SHP: 10, HHP: 50}, draw: 10, expectErr: ErrInvalidCreatureDamageTime, invalidate: func(f *creatureDamageFixture) {
			ecs.GetResource[ecs.TimeState](f.world).UnixMs = math.MaxInt64 - playerstate.KnockoutDurationMs + 1
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCreatureDamageFixture(t)
			ecs.AddComponent(fixture.world, fixture.owner, test.movement)
			health := ecs.GetOrCreateStorage[components.EntityHealth](fixture.world)
			movement := ecs.GetOrCreateStorage[components.Movement](fixture.world)
			if test.invalidate != nil {
				test.invalidate(fixture)
			}
			statsBuffer := make([]types.EntityID, 0, 1)
			visualBuffer := make([]types.Handle, 0, 1)
			allocations := testing.AllocsPerRun(1000, func() {
				health.Set(fixture.owner, test.health)
				movement.Set(fixture.owner, test.movement)
				creatureDamageAllocationResult, creatureDamageAllocationError = fixture.service.Apply(fixture.owner, test.draw)
				if !test.pending {
					statsBuffer = fixture.stats.PopDuePlayerStatsPush(math.MaxInt64, statsBuffer[:0])
					visualBuffer = fixture.visual.Drain(0, visualBuffer[:0])
				}
			})
			require.Equal(t, 0.0, allocations)
			require.Equal(t, test.expectErr, creatureDamageAllocationError)
		})
	}
	t.Run("stale generation", func(t *testing.T) {
		fixture := newCreatureDamageFixture(t)
		stale := fixture.owner
		fixture.world.Despawn(stale)
		replacement := fixture.world.Spawn(1, nil)
		replacementHealth := components.EntityHealth{SHP: 10, HHP: 50}
		ecs.AddComponent(fixture.world, replacement, replacementHealth)
		require.NoError(t, fixture.service.PrepareTarget(replacement))
		allocations := testing.AllocsPerRun(1000, func() {
			creatureDamageAllocationResult, creatureDamageAllocationError = fixture.service.Apply(stale, 1)
		})
		require.Zero(t, allocations)
		require.ErrorIs(t, creatureDamageAllocationError, ErrInvalidCreatureTarget)
		health, _ := ecs.GetComponent[components.EntityHealth](fixture.world, replacement)
		require.Equal(t, replacementHealth, health)
	})
}
