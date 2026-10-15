package game

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"origin/internal/builddefs"
	"origin/internal/characterattrs"
	"origin/internal/config"
	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entitystats"
	"origin/internal/game/behaviors"
	"origin/internal/game/hearth"
	gameworld "origin/internal/game/world"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
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
	require.Equal(t, build.Inputs[0].Count, entry.Inputs[0].Count)
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
	saver          *hearthBuildSaver
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
	state.Items[0].PutItems = []components.BuildPutItemState{{ItemKey: "branch", Count: build.Inputs[0].Count, Quality: 10}}
	site := w.Spawn(200, func(w *ecs.World, handle types.Handle) {
		ecs.AddComponent(w, handle, components.EntityInfo{TypeID: constt.BuildObjectTypeID})
		ecs.AddComponent(w, handle, components.Transform{X: 6, Y: 6})
		ecs.AddComponent(w, handle, components.ChunkRef{})
		internal := components.ObjectInternalState{}
		components.SetBehaviorState(&internal, buildBehaviorStateKey, state)
		ecs.AddComponent(w, handle, internal)
	})
	sender := &lockedBuildSender{}
	registry := behaviors.MustDefaultRegistry()
	saver := &hearthBuildSaver{}
	factory := gameworld.NewObjectFactory(nil)
	chunks := gameworld.NewChunkManager(cfg, nil, w, nil, 0, cfg.Game.Region, factory, registry, nil, zap.NewNop())
	t.Cleanup(chunks.Stop)
	ecs.InitResource(w, hearth.NewRuntime(w, nil, nil, factory, saver, registry, nil, nil))
	service := NewBuildService(w, nil, chunks, nil, nil, nil, registry, nil, sender, nil, nil, nil)
	contextActions := NewContextActionService(w, nil, nil, nil, nil, nil, nil, nil, nil, behaviors.MustDefaultRegistry(), nil)
	contextActions.SetBuildService(service)
	return &hearthBuildRecipeFixture{
		world: w, build: build, result: result, site: site, service: service,
		contextActions: contextActions, system: NewCyclicActionSystem(contextActions, nil, nil),
		tickPeriod: tickPeriod, initialStamina: entitystats.MaxStaminaFromCon(characterattrs.DefaultValue), saver: saver,
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
	require.Equal(t, f.build.Inputs[0].Count-count, state.Items[0].PutCount())
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

func TestHearthBuildRecipeSoloUsesOneCyclePerBranch(t *testing.T) {
	f := newHearthBuildRecipeFixture(t)
	branches := f.build.Inputs[0].Count
	player := f.startBuilder(t, 1)
	for cycle := uint32(1); cycle <= branches; cycle++ {
		f.advanceTicks(int(f.build.TicksRequired) - 1)
		f.assertProgress(t, cycle-1)
		stats, found := ecs.GetComponent[components.EntityStats](f.world, player)
		require.True(t, found)
		require.Equal(t, f.initialStamina-float64(cycle-1)*f.build.StaminaCost, stats.Stamina)
		f.advanceTicks(1)
		stats, _ = ecs.GetComponent[components.EntityStats](f.world, player)
		require.Equal(t, f.initialStamina-float64(cycle)*f.build.StaminaCost, stats.Stamina)
		if cycle < branches {
			f.assertProgress(t, cycle)
		} else {
			f.assertCompleted(t)
		}
	}
	require.Equal(t, time.Duration(branches*f.build.TicksRequired)*f.tickPeriod,
		time.Duration(ecs.GetResource[ecs.TimeState](f.world).Tick)*f.tickPeriod)
	require.False(t, ecs.HasComponent[components.ActiveCyclicAction](f.world, player))
	_, linked := ecs.GetResource[ecs.LinkState](f.world).GetLink(1)
	require.False(t, linked)
	owner, owned := ecs.GetResource[hearth.Runtime](f.world).Index.Owner(hearth.ObjectKey{ID: 200})
	require.True(t, owned)
	require.Equal(t, types.EntityID(1), owner)
	require.Equal(t, 1, f.saver.calls)
	require.Equal(t, owner, f.saver.character)
	require.Nil(t, f.saver.previous)
}

type hearthBuildSaver struct {
	calls     int
	character types.EntityID
	current   *repository.Object
	previous  *repository.Object
}

func (s *hearthBuildSaver) SaveConstruction(_ context.Context, character types.EntityID, current, previous *repository.Object) error {
	s.calls++
	s.character, s.current, s.previous = character, current, previous
	return nil
}

type hearthBuildLookup struct {
	reads int
	home  *repository.GetCharacterHearthRow
}

func (l *hearthBuildLookup) GetCharacterHearth(context.Context, repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
	l.reads++
	if l.home != nil {
		return *l.home, nil
	}
	return repository.GetCharacterHearthRow{}, sql.ErrNoRows
}

type hearthBuildChunks struct {
	chunk  *core.Chunk
	pins   int
	loads  int
	loaded bool
}

func (c *hearthBuildChunks) PinPersistenceBlocking(types.ChunkCoord) error {
	c.pins++
	return nil
}

func (c *hearthBuildChunks) UnpinPersistence(types.ChunkCoord) { c.pins-- }

func (c *hearthBuildChunks) GetChunkFast(types.ChunkCoord) *core.Chunk {
	if c.loaded {
		return c.chunk
	}
	return nil
}

func (c *hearthBuildChunks) WaitPreloaded(context.Context, types.ChunkCoord) error {
	c.loads++
	c.loaded = true
	c.chunk.SetState(types.ChunkStateInactive)
	return nil
}

func TestHearthConstructionUnloadedHomeCompletesOnLastBranch(t *testing.T) {
	f := newHearthBuildRecipeFixture(t)
	internal, _ := ecs.GetComponent[components.ObjectInternalState](f.world, f.site)
	state, found := components.GetBehaviorState[components.BuildBehaviorState](internal, buildBehaviorStateKey)
	require.True(t, found)
	branches := f.build.Inputs[0].Count
	player := f.startBuilder(t, 1)
	raw := &repository.Object{ID: 300, TypeID: f.result.DefID, X: 5000, Y: 200, Quality: 17}
	coord := types.WorldToChunkCoord(raw.X, raw.Y, constt.ChunkSize, constt.CoordPerTile)
	raw.ChunkX, raw.ChunkY = coord.X, coord.Y
	chunk := core.NewChunk(coord, 0, hearth.SurfaceLayer, constt.ChunkSize)
	chunk.SetRawObjects([]*repository.Object{raw})
	chunks := &hearthBuildChunks{chunk: chunk}
	lookup := &hearthBuildLookup{home: &repository.GetCharacterHearthRow{CharacterID: 1, ObjectID: raw.ID, X: raw.X, Y: raw.Y}}
	runtime := ecs.GetResource[hearth.Runtime](f.world)
	*runtime = hearth.NewRuntime(f.world, chunks, lookup, gameworld.NewObjectFactory(nil), f.saver, behaviors.MustDefaultRegistry(), nil, nil)

	f.advanceTicks(int(f.build.TicksRequired*branches) - 1)
	require.Equal(t, branches-1, state.Items[0].BuildCount)
	require.Zero(t, chunks.loads)
	require.Zero(t, lookup.reads)
	f.advanceTicks(1)
	f.assertCompleted(t)
	require.False(t, ecs.HasComponent[components.ActiveCyclicAction](f.world, player))
	require.Equal(t, types.EntityID(200), runtime.Index.Snapshot(1).Home.Object.ID)
	require.Equal(t, 1, chunks.loads)
	require.Equal(t, 1, lookup.reads)
	require.Zero(t, chunks.pins)
	require.Equal(t, types.InvalidHandle, f.world.GetHandleByEntityID(300))
	campfire, found := objectdefs.Global().GetByKey("campfire")
	require.True(t, found)
	converted := chunk.GetRawObjectByID(300)
	require.Equal(t, campfire.DefID, converted.TypeID)
	require.Equal(t, raw.ID, converted.ID)
	require.Equal(t, raw.X, converted.X)
	require.Equal(t, raw.Y, converted.Y)
	require.Equal(t, 1, f.saver.calls)
	require.Equal(t, int64(200), f.saver.current.ID)
	require.Equal(t, converted, f.saver.previous)
	stats, _ := ecs.GetComponent[components.EntityStats](f.world, player)
	require.Equal(t, f.initialStamina-float64(branches)*f.build.StaminaCost, stats.Stamina)
	require.Equal(t, uint64(f.build.TicksRequired*branches), ecs.GetResource[ecs.TimeState](f.world).Tick)
}

func TestHearthConstructionUnknownHomeCompletesInRequiredCycles(t *testing.T) {
	f := newHearthBuildRecipeFixture(t)
	branches := f.build.Inputs[0].Count
	player := f.startBuilder(t, 1)
	lookup := &hearthBuildLookup{}
	// Exercise the real first lookup instead of the fixture's known NoHome.
	runtime := ecs.GetResource[hearth.Runtime](f.world)
	*runtime = hearth.NewRuntime(f.world, nil, lookup, gameworld.NewObjectFactory(nil), f.saver, behaviors.MustDefaultRegistry(), nil, nil)
	sender := &lockedBuildSender{}
	f.service.alerts = sender

	f.advanceTicks(int(f.build.TicksRequired*branches) - 1)
	f.assertProgress(t, branches-1)
	require.Equal(t, hearth.Unknown, runtime.Index.Snapshot(1).State)
	require.Zero(t, lookup.reads)
	stats, _ := ecs.GetComponent[components.EntityStats](f.world, player)
	require.Equal(t, f.initialStamina-float64(branches-1)*f.build.StaminaCost, stats.Stamina)

	f.advanceTicks(1)
	f.assertCompleted(t)
	require.Equal(t, 1, lookup.reads)
	require.Empty(t, sender.alerts)
	require.False(t, ecs.HasComponent[components.ActiveCyclicAction](f.world, player))
	require.Equal(t, f.site, f.world.GetHandleByEntityID(200))
	require.Equal(t, types.EntityID(200), runtime.Index.Snapshot(1).Home.Object.ID)
	stats, _ = ecs.GetComponent[components.EntityStats](f.world, player)
	require.Equal(t, f.initialStamina-float64(branches)*f.build.StaminaCost, stats.Stamina)
	require.Equal(t, uint64(f.build.TicksRequired*branches), ecs.GetResource[ecs.TimeState](f.world).Tick)
	require.Equal(t, 1, f.saver.calls)
	require.Equal(t, types.EntityID(1), f.saver.character)
	require.False(t, runtime.Index.Snapshot(1).Dirty)
}

func TestHearthBuildRecipeHelpersShareProgressAndOwnCosts(t *testing.T) {
	f := newHearthBuildRecipeFixture(t)
	branches := f.build.Inputs[0].Count
	firstShare, helperShare := branches/2, branches-branches/2
	first := f.startBuilder(t, 1)
	// Stagger by one tick so the helper's final cycle follows the first worker's.
	f.advanceTicks(1)
	helper := f.startBuilder(t, 2)
	helperProcessed := uint32(0)
	if firstShare > 0 {
		f.advanceTicks(int(firstShare*f.build.TicksRequired) - 1)
		helperProcessed = firstShare - 1
	}
	f.assertProgress(t, firstShare+helperProcessed)
	firstStats, _ := ecs.GetComponent[components.EntityStats](f.world, first)
	helperStats, _ := ecs.GetComponent[components.EntityStats](f.world, helper)
	require.Equal(t, f.initialStamina-float64(firstShare)*f.build.StaminaCost, firstStats.Stamina)
	require.Equal(t, f.initialStamina-float64(helperProcessed)*f.build.StaminaCost, helperStats.Stamina)

	f.contextActions.cancelActiveCyclicAction(1, first, "")
	finishTick := uint64(helperShare*f.build.TicksRequired) + 1
	f.advanceTicks(int(finishTick - ecs.GetResource[ecs.TimeState](f.world).Tick))
	f.assertCompleted(t)
	firstStats, _ = ecs.GetComponent[components.EntityStats](f.world, first)
	helperStats, _ = ecs.GetComponent[components.EntityStats](f.world, helper)
	require.Equal(t, f.initialStamina-float64(firstShare)*f.build.StaminaCost, firstStats.Stamina)
	require.Equal(t, f.initialStamina-float64(helperShare)*f.build.StaminaCost, helperStats.Stamina)
	require.Equal(t, finishTick, ecs.GetResource[ecs.TimeState](f.world).Tick)
	if branches > 1 {
		require.Less(t, finishTick, uint64(branches*f.build.TicksRequired), "cooperative work must finish faster than solo work")
	}
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
	require.Equal(t, 1, f.saver.calls)
	require.Equal(t, owner, f.saver.character)
}

func TestHearthConstructionReplacesOnlyFinishersLiveHome(t *testing.T) {
	f := newHearthBuildRecipeFixture(t)
	branches := f.build.Inputs[0].Count
	firstShare, helperShare := branches/2, branches-branches/2
	first := f.startBuilder(t, 1)
	index := ecs.GetResource[hearth.Runtime](f.world).Index
	spawnHome := func(character, id types.EntityID, x int) types.Handle {
		handle := gameworld.SpawnEntityFromDef(f.world, f.result, gameworld.DefSpawnParams{EntityID: id, X: float64(x), Y: 24, Quality: 17})
		coord := types.WorldToChunkCoord(x, 24, constt.ChunkSize, constt.CoordPerTile)
		ecs.AddComponent(f.world, handle, components.ChunkRef{CurrentChunkX: coord.X, CurrentChunkY: coord.Y})
		_, err := index.ReplaceHome(character, index.Snapshot(character).Version, hearth.Home{Object: hearth.ObjectKey{ID: id}, X: x, Y: 24})
		require.NoError(t, err)
		return handle
	}
	initiatorHome := spawnHome(1, 300, 60)
	f.advanceTicks(int(firstShare * f.build.TicksRequired))
	f.assertProgress(t, firstShare)
	helper := f.startBuilder(t, 2)
	oldHome := spawnHome(2, 301, 84)
	before, _ := ecs.GetComponent[components.Transform](f.world, oldHome)
	f.contextActions.cancelActiveCyclicAction(1, first, "")
	// The initiator may go offline or die; ownership follows the surviving helper.
	f.world.Despawn(first)
	f.advanceTicks(int(helperShare * f.build.TicksRequired))
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
	require.Equal(t, f.initialStamina-float64(helperShare)*f.build.StaminaCost, stats.Stamina)
	require.Equal(t, 1, f.saver.calls)
	require.Equal(t, types.EntityID(2), f.saver.character)
	require.Equal(t, int64(301), f.saver.previous.ID)
	require.Equal(t, campfire.DefID, f.saver.previous.TypeID)
}

func TestHearthConstructionCompetingFinalCyclesAssignOnce(t *testing.T) {
	f := newHearthBuildRecipeFixture(t)
	branches := f.build.Inputs[0].Count
	first, second := f.startBuilder(t, 1), f.startBuilder(t, 2)
	f.advanceTicks(int(f.build.TicksRequired * ((branches + 1) / 2)))
	f.assertCompleted(t)
	index := ecs.GetResource[hearth.Runtime](f.world).Index
	owner, owned := index.Owner(hearth.ObjectKey{ID: 200})
	require.True(t, owned)
	require.Contains(t, []types.EntityID{1, 2}, owner)
	firstStats, _ := ecs.GetComponent[components.EntityStats](f.world, first)
	secondStats, _ := ecs.GetComponent[components.EntityStats](f.world, second)
	require.Equal(t, 2*f.initialStamina-float64(branches)*f.build.StaminaCost, firstStats.Stamina+secondStats.Stamina)
	version := index.Snapshot(owner).Version
	f.service.HandleBuildCycleComplete(f.world, 2, second, components.ActiveCyclicAction{TargetID: 200, TargetHandle: f.site})
	require.Equal(t, version, index.Snapshot(owner).Version)
	stillOwner, _ := index.Owner(hearth.ObjectKey{ID: 200})
	require.Equal(t, owner, stillOwner)
	require.Equal(t, 1, f.saver.calls)
	require.Equal(t, owner, f.saver.character)
}
