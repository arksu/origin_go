package game

import (
	"path/filepath"
	"testing"
	"time"

	"origin/internal/builddefs"
	"origin/internal/characterattrs"
	"origin/internal/config"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entitystats"
	"origin/internal/game/behaviors"
	"origin/internal/game/hearth"
	gameworld "origin/internal/game/world"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func loadHearthBuildRecipeForTest(t *testing.T) (*builddefs.BuildDef, *objectdefs.ObjectDef) {
	t.Helper()
	previousBuilds, previousObjects, previousItems := builddefs.Global(), objectdefs.Global(), itemdefs.Global()
	t.Cleanup(func() {
		builddefs.SetGlobalForTesting(previousBuilds)
		objectdefs.SetGlobalForTesting(previousObjects)
		itemdefs.SetGlobalForTesting(previousItems)
	})
	dataPath := filepath.Join("..", "..", "data")
	items, err := itemdefs.LoadFromDirectory(filepath.Join(dataPath, "items"), zap.NewNop())
	require.NoError(t, err)
	itemdefs.SetGlobalForTesting(items)
	objects, err := objectdefs.LoadFromDirectory(filepath.Join(dataPath, "objects"), behaviors.MustDefaultRegistry(), zap.NewNop())
	require.NoError(t, err)
	objectdefs.SetGlobalForTesting(objects)
	builds, err := builddefs.LoadFromDirectory(filepath.Join(dataPath, "builds"), zap.NewNop())
	require.NoError(t, err)
	builddefs.SetGlobalForTesting(builds)
	build, found := builds.GetByKey("hearth_fire")
	require.True(t, found)
	result, found := objects.GetByKey(build.ObjectKey)
	require.True(t, found)
	return build, result
}

func TestHearthBuildRecipeVisibleToNewCharacter(t *testing.T) {
	build, result := loadHearthBuildRecipeForTest(t)
	w := ecs.NewWorldForTesting()
	player := w.Spawn(1, func(w *ecs.World, handle types.Handle) {
		ecs.AddComponent(w, handle, components.CharacterProfile{})
	})
	var entry *netproto.BuildRecipeEntry
	for _, candidate := range (&Shard{}).buildVisibleBuildList(w, player) {
		if candidate.BuildKey == build.Key {
			entry = candidate
			break
		}
	}
	require.NotNil(t, entry, "a character with no skills or discoveries must see the recipe")
	require.Equal(t, "Hearth Fire", entry.Name)
	require.Equal(t, "hearth_fire", entry.ObjectKey)
	require.Equal(t, result.Resource, entry.ObjectResourcePath)
	require.Equal(t, float64(8), entry.StaminaCost)
	require.Equal(t, uint32(20), entry.TicksRequired)
	require.Empty(t, entry.RequiredSkills)
	require.Empty(t, entry.RequiredDiscovery)
	require.Len(t, entry.Inputs, 1)
	require.Equal(t, "branch", entry.Inputs[0].GetItemKey())
	require.Equal(t, uint32(5), entry.Inputs[0].Count)
	require.Equal(t, uint32(1), entry.Inputs[0].QualityWeight)
	branch, found := itemdefs.Global().GetByKey("branch")
	require.True(t, found)
	require.Equal(t, branch.ResolveResource(false), entry.Inputs[0].Resource)
}

type hearthBuildRecipeFixture struct {
	world          *ecs.World
	build          *builddefs.BuildDef
	result         *objectdefs.ObjectDef
	site           types.Handle
	service        *BuildService
	contextActions *ContextActionService
	system         *CyclicActionSystem
	tickPeriod     time.Duration
	initialStamina float64
}

func newHearthBuildRecipeFixture(t *testing.T) *hearthBuildRecipeFixture {
	t.Helper()
	build, result := loadHearthBuildRecipeForTest(t)
	t.Setenv("GAME_TICK_RATE", "")
	cfg, err := config.Load(zap.NewNop())
	require.NoError(t, err)
	tickPeriod := time.Second / time.Duration(cfg.Game.TickRate)
	require.Equal(t, 2*time.Second, time.Duration(build.TicksRequired)*tickPeriod)
	w := ecs.NewWorldForTesting()
	timing := ecs.GetResource[ecs.TimeState](w)
	timing.TickRate, timing.TickPeriod, timing.Delta = cfg.Game.TickRate, tickPeriod, tickPeriod.Seconds()
	state := buildStateFromDef(build, result, 6, 6)
	state.Items[0].PutItems = []components.BuildPutItemState{{ItemKey: "branch", Count: 5, Quality: 10}}
	site := w.Spawn(200, func(w *ecs.World, handle types.Handle) {
		ecs.AddComponent(w, handle, components.EntityInfo{TypeID: constt.BuildObjectTypeID})
		ecs.AddComponent(w, handle, components.Transform{X: 6, Y: 6})
		internal := components.ObjectInternalState{}
		components.SetBehaviorState(&internal, buildBehaviorStateKey, state)
		ecs.AddComponent(w, handle, internal)
	})
	sender := &lockedBuildSender{}
	registry := behaviors.MustDefaultRegistry()
	ecs.InitResource(w, hearth.NewRuntime(w, nil, nil, registry, nil, nil))
	service := NewBuildService(w, nil, nil, nil, nil, nil, registry, nil, sender, nil, nil, nil)
	contextActions := NewContextActionService(w, nil, nil, nil, nil, nil, nil, nil, nil, behaviors.MustDefaultRegistry(), nil)
	contextActions.SetBuildService(service)
	return &hearthBuildRecipeFixture{
		world: w, build: build, result: result, site: site, service: service,
		contextActions: contextActions, system: NewCyclicActionSystem(contextActions, nil, nil),
		tickPeriod: tickPeriod, initialStamina: entitystats.MaxStaminaFromCon(characterattrs.DefaultValue),
	}
}

func (f *hearthBuildRecipeFixture) startBuilder(t *testing.T, playerID types.EntityID) types.Handle {
	t.Helper()
	index := ecs.GetResource[hearth.Runtime](f.world).Index
	if version, started := index.BeginLookup(playerID); started {
		require.NoError(t, index.CompleteLookup(playerID, version, nil))
	}
	player := f.world.Spawn(playerID, func(w *ecs.World, handle types.Handle) {
		ecs.AddComponent(w, handle, components.CharacterProfile{})
		ecs.AddComponent(w, handle, components.EntityStats{Stamina: f.initialStamina})
		ecs.AddComponent(w, handle, components.Movement{State: constt.StateIdle})
	})
	ecs.GetResource[ecs.LinkState](f.world).SetLink(ecs.PlayerLink{PlayerID: playerID, TargetID: 200})
	f.service.HandleBuildProgress(f.world, playerID, player, &netproto.C2S_BuildProgress{EntityId: 200})
	action, found := ecs.GetComponent[components.ActiveCyclicAction](f.world, player)
	require.True(t, found)
	require.Equal(t, f.build.TicksRequired, action.CycleDurationTicks)
	return player
}

func (f *hearthBuildRecipeFixture) advanceTicks(count int) {
	timing := ecs.GetResource[ecs.TimeState](f.world)
	for tick := 0; tick < count; tick++ {
		timing.Tick++
		timing.Now = timing.Now.Add(f.tickPeriod)
		timing.UnixMs += f.tickPeriod.Milliseconds()
		f.system.Update(f.world, timing.Delta)
	}
}

func (f *hearthBuildRecipeFixture) assertProgress(t *testing.T, count uint32) {
	t.Helper()
	info, found := ecs.GetComponent[components.EntityInfo](f.world, f.site)
	require.True(t, found)
	require.Equal(t, uint32(constt.BuildObjectTypeID), info.TypeID)
	internal, found := ecs.GetComponent[components.ObjectInternalState](f.world, f.site)
	require.True(t, found)
	state, found := components.GetBehaviorState[components.BuildBehaviorState](internal, buildBehaviorStateKey)
	require.True(t, found)
	require.Equal(t, count, state.Items[0].BuildCount)
	require.Equal(t, 5-count, state.Items[0].PutCount())
}

func (f *hearthBuildRecipeFixture) assertCompleted(t *testing.T) {
	t.Helper()
	info, found := ecs.GetComponent[components.EntityInfo](f.world, f.site)
	require.True(t, found)
	require.Equal(t, uint32(f.result.DefID), info.TypeID)
	internal, found := ecs.GetComponent[components.ObjectInternalState](f.world, f.site)
	require.True(t, found)
	_, found = components.GetBehaviorState[components.BuildBehaviorState](internal, buildBehaviorStateKey)
	require.False(t, found)
}

func TestHearthBuildRecipeSoloUsesFiveOrdinaryCycles(t *testing.T) {
	f := newHearthBuildRecipeFixture(t)
	player := f.startBuilder(t, 1)
	for cycle := uint32(1); cycle <= 5; cycle++ {
		f.advanceTicks(int(f.build.TicksRequired) - 1)
		f.assertProgress(t, cycle-1)
		stats, found := ecs.GetComponent[components.EntityStats](f.world, player)
		require.True(t, found)
		require.Equal(t, f.initialStamina-float64(cycle-1)*8, stats.Stamina)
		f.advanceTicks(1)
		stats, _ = ecs.GetComponent[components.EntityStats](f.world, player)
		require.Equal(t, f.initialStamina-float64(cycle)*8, stats.Stamina)
		if cycle < 5 {
			f.assertProgress(t, cycle)
		} else {
			f.assertCompleted(t)
		}
	}
	require.Equal(t, 10*time.Second, time.Duration(ecs.GetResource[ecs.TimeState](f.world).Tick)*f.tickPeriod)
	require.False(t, ecs.HasComponent[components.ActiveCyclicAction](f.world, player))
	_, linked := ecs.GetResource[ecs.LinkState](f.world).GetLink(1)
	require.False(t, linked)
	owner, owned := ecs.GetResource[hearth.Runtime](f.world).Index.Owner(hearth.ObjectKey{ID: 200})
	require.True(t, owned)
	require.Equal(t, types.EntityID(1), owner)
}

func TestHearthBuildRecipeHelpersShareProgressAndOwnCosts(t *testing.T) {
	f := newHearthBuildRecipeFixture(t)
	first, helper := f.startBuilder(t, 1), f.startBuilder(t, 2)
	f.advanceTicks(int(f.build.TicksRequired) * 2)
	f.assertProgress(t, 4)
	for _, handle := range []types.Handle{first, helper} {
		stats, found := ecs.GetComponent[components.EntityStats](f.world, handle)
		require.True(t, found)
		require.Equal(t, f.initialStamina-16, stats.Stamina)
	}
	f.contextActions.cancelActiveCyclicAction(1, first, "")
	f.advanceTicks(int(f.build.TicksRequired))
	f.assertCompleted(t)
	firstStats, _ := ecs.GetComponent[components.EntityStats](f.world, first)
	helperStats, _ := ecs.GetComponent[components.EntityStats](f.world, helper)
	require.Equal(t, f.initialStamina-16, firstStats.Stamina)
	require.Equal(t, f.initialStamina-24, helperStats.Stamina)
	require.Equal(t, 6*time.Second, time.Duration(ecs.GetResource[ecs.TimeState](f.world).Tick)*f.tickPeriod)
	require.False(t, ecs.HasComponent[components.ActiveCyclicAction](f.world, helper))
	_, firstLinked := ecs.GetResource[ecs.LinkState](f.world).GetLink(1)
	_, helperLinked := ecs.GetResource[ecs.LinkState](f.world).GetLink(2)
	require.False(t, firstLinked)
	require.False(t, helperLinked)
	index := ecs.GetResource[hearth.Runtime](f.world).Index
	owner, owned := index.Owner(hearth.ObjectKey{ID: 200})
	require.True(t, owned)
	require.Equal(t, types.EntityID(2), owner)
	require.Equal(t, hearth.NoHome, index.Snapshot(1).State)
}

func TestHearthConstructionReplacesOnlyFinishersLiveHome(t *testing.T) {
	f := newHearthBuildRecipeFixture(t)
	first, helper := f.startBuilder(t, 1), f.startBuilder(t, 2)
	index := ecs.GetResource[hearth.Runtime](f.world).Index
	spawnHome := func(character, id types.EntityID, x int) types.Handle {
		handle := gameworld.SpawnEntityFromDef(f.world, f.result, gameworld.DefSpawnParams{EntityID: id, X: float64(x), Y: 24, Quality: 17})
		_, err := index.ReplaceHome(character, index.Snapshot(character).Version, hearth.Home{Object: hearth.ObjectKey{ID: id}, X: x, Y: 24})
		require.NoError(t, err)
		return handle
	}
	initiatorHome := spawnHome(1, 300, 60)
	oldHome := spawnHome(2, 301, 84)
	before, _ := ecs.GetComponent[components.Transform](f.world, oldHome)
	f.advanceTicks(int(f.build.TicksRequired) * 2)
	f.contextActions.cancelActiveCyclicAction(1, first, "")
	// The initiator may go offline or die; ownership follows the surviving helper.
	f.world.Despawn(first)
	f.advanceTicks(int(f.build.TicksRequired))
	f.assertCompleted(t)
	require.Equal(t, types.EntityID(200), index.Snapshot(2).Home.Object.ID)
	require.Equal(t, types.EntityID(300), index.Snapshot(1).Home.Object.ID)
	require.Equal(t, oldHome, f.world.GetHandleByEntityID(301))
	info, _ := ecs.GetComponent[components.EntityInfo](f.world, oldHome)
	campfire, found := objectdefs.Global().GetByKey("campfire")
	require.True(t, found)
	require.Equal(t, uint32(campfire.DefID), info.TypeID)
	require.Equal(t, uint32(17), info.Quality)
	require.False(t, info.Indestructible)
	after, _ := ecs.GetComponent[components.Transform](f.world, oldHome)
	require.Equal(t, before, after)
	station, found := ecs.GetComponent[components.StationState](f.world, oldHome)
	require.True(t, found)
	require.Equal(t, "unlit", station.CurrentState)
	initialInfo, _ := ecs.GetComponent[components.EntityInfo](f.world, initiatorHome)
	require.Equal(t, uint32(f.result.DefID), initialInfo.TypeID)
	stats, _ := ecs.GetComponent[components.EntityStats](f.world, helper)
	require.Equal(t, f.initialStamina-24, stats.Stamina)
}

func TestHearthConstructionCompetingFinalCyclesAssignOnce(t *testing.T) {
	f := newHearthBuildRecipeFixture(t)
	first, second := f.startBuilder(t, 1), f.startBuilder(t, 2)
	f.advanceTicks(int(f.build.TicksRequired) * 3)
	f.assertCompleted(t)
	index := ecs.GetResource[hearth.Runtime](f.world).Index
	owner, owned := index.Owner(hearth.ObjectKey{ID: 200})
	require.True(t, owned)
	require.Contains(t, []types.EntityID{1, 2}, owner)
	firstStats, _ := ecs.GetComponent[components.EntityStats](f.world, first)
	secondStats, _ := ecs.GetComponent[components.EntityStats](f.world, second)
	require.Equal(t, 2*f.initialStamina-40, firstStats.Stamina+secondStats.Stamina)
	version := index.Snapshot(owner).Version
	f.service.HandleBuildCycleComplete(f.world, 2, second, components.ActiveCyclicAction{TargetID: 200, TargetHandle: f.site})
	require.Equal(t, version, index.Snapshot(owner).Version)
	stillOwner, _ := index.Owner(hearth.ObjectKey{ID: 200})
	require.Equal(t, owner, stillOwner)
}
