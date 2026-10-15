package hearth_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/game/hearth"
	"origin/internal/game/world"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type constructionChunks struct {
	chunks        map[types.ChunkCoord]*core.Chunk
	pins          map[types.ChunkCoord]int
	pinErr        error
	wait          func(types.ChunkCoord) error
	waited        []types.ChunkCoord
	waitPins      []int
	waitDeadlines []time.Time
}

func (c *constructionChunks) GetChunkFast(coord types.ChunkCoord) *core.Chunk {
	return c.chunks[coord]
}

func (c *constructionChunks) PinPersistenceBlocking(coord types.ChunkCoord) error {
	if c.pinErr != nil {
		return c.pinErr
	}
	c.pins[coord]++
	return nil
}

func (c *constructionChunks) UnpinPersistence(coord types.ChunkCoord) { c.pins[coord]-- }

func (c *constructionChunks) WaitPreloaded(ctx context.Context, coord types.ChunkCoord) error {
	c.waited = append(c.waited, coord)
	c.waitPins = append(c.waitPins, c.pins[coord])
	deadline, _ := ctx.Deadline()
	c.waitDeadlines = append(c.waitDeadlines, deadline)
	if c.wait != nil {
		return c.wait(coord)
	}
	return nil
}

type constructionReader struct {
	row    repository.GetCharacterHearthRow
	err    error
	found  bool
	reads  int
	onRead func()
}

func (r *constructionReader) GetCharacterHearth(context.Context, repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
	r.reads++
	if r.onRead != nil {
		r.onRead()
	}
	if r.err != nil {
		return repository.GetCharacterHearthRow{}, r.err
	}
	if !r.found {
		return repository.GetCharacterHearthRow{}, sql.ErrNoRows
	}
	return r.row, nil
}

type constructionFixture struct {
	w        *ecs.World
	chunks   *constructionChunks
	reader   *constructionReader
	runtime  hearth.Runtime
	registry *behaviors.Registry
	ctx      contracts.ConstructionContext
	result   *objectdefs.ObjectDef
}

func newConstructionFixture(t *testing.T) *constructionFixture {
	t.Helper()
	dataPath := filepath.Join("..", "..", "..", "data")
	items, err := itemdefs.LoadFromDirectory(filepath.Join(dataPath, "items"), zap.NewNop())
	require.NoError(t, err)
	previousItems := itemdefs.Global()
	itemdefs.SetGlobalForTesting(items)
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousItems) })
	registry := behaviors.MustDefaultRegistry()
	objects, err := objectdefs.LoadFromDirectory(filepath.Join(dataPath, "objects"), registry, zap.NewNop())
	require.NoError(t, err)
	previousObjects := objectdefs.Global()
	objectdefs.SetGlobalForTesting(objects)
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previousObjects) })
	result, found := objects.GetByKey("hearth_fire")
	require.True(t, found)
	w := ecs.NewWorldForTesting()
	actor := w.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityHealth{HHP: 100, SHP: 100})
		ecs.AddComponent(w, h, components.Transform{X: 30, Y: 40})
	})
	site := w.Spawn(1000, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: constt.BuildObjectTypeID, Region: 1})
		ecs.AddComponent(w, h, components.Transform{X: 50, Y: 60})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
		ecs.AddComponent(w, h, components.Appearance{Resource: "build_site"})
	})
	chunks := &constructionChunks{chunks: make(map[types.ChunkCoord]*core.Chunk), pins: make(map[types.ChunkCoord]int)}
	reader := &constructionReader{}
	f := &constructionFixture{w: w, chunks: chunks, reader: reader, registry: registry, result: result,
		ctx: contracts.ConstructionContext{World: w, PlayerID: 1, PlayerHandle: actor, TargetID: 1000, TargetHandle: site, ResultType: uint32(result.DefID)}}
	f.runtime = hearth.NewRuntime(w, chunks, reader, registry, nil, zap.NewNop())
	return f
}

func (f *constructionFixture) seedHome(t *testing.T, home *hearth.Home) {
	t.Helper()
	version, started := f.runtime.Index.BeginLookup(f.ctx.PlayerID)
	require.True(t, started)
	require.NoError(t, f.runtime.Index.CompleteLookup(f.ctx.PlayerID, version, home))
}

func (f *constructionFixture) finishSite() bool {
	return world.TransformObjectToDefInPlace(f.w, f.ctx.TargetID, f.ctx.TargetHandle, f.result, world.TransformObjectInPlaceOptions{BehaviorRegistry: f.registry})
}

func (f *constructionFixture) oldRaw(t *testing.T, state types.ChunkState) (*core.Chunk, *repository.Object) {
	t.Helper()
	raw := &repository.Object{ID: 2000, TypeID: f.result.DefID, Region: 1, X: 5000, Y: 200, Heading: sql.NullInt16{Int16: 90, Valid: true}, Quality: 17, CreateTick: 12}
	coord := types.WorldToChunkCoord(raw.X, raw.Y, constt.ChunkSize, constt.CoordPerTile)
	raw.ChunkX, raw.ChunkY = coord.X, coord.Y
	chunk := core.NewChunk(coord, 1, 0, constt.ChunkSize)
	chunk.SetState(state)
	chunk.SetRawObjects([]*repository.Object{raw})
	f.seedHome(t, &hearth.Home{Object: hearth.ObjectKey{Region: 1, ID: 2000}, X: raw.X, Y: raw.Y})
	return chunk, raw
}

func TestConstructionFirstHomeCompletesInSameCall(t *testing.T) {
	for _, scenario := range []string{"normal", "without_chunks"} {
		t.Run(scenario, func(t *testing.T) {
			f := newConstructionFixture(t)
			if scenario == "without_chunks" {
				f.runtime = hearth.NewRuntime(f.w, nil, f.reader, f.registry, nil, zap.NewNop())
			}
			calls := 0
			result := f.runtime.CompleteConstruction(&f.ctx, func() bool {
				calls++
				require.Equal(t, 1, f.reader.reads)
				require.Equal(t, hearth.NoHome, f.runtime.Index.Snapshot(1).State)
				return f.finishSite()
			})
			require.True(t, result.OK)
			require.Equal(t, 1, calls)
			require.Equal(t, 1, f.reader.reads)
			home := f.runtime.Index.Snapshot(1)
			require.Equal(t, hearth.CurrentHome, home.State)
			require.Equal(t, types.EntityID(1000), home.Home.Object.ID)
			require.True(t, home.Dirty)
			position, _ := ecs.GetComponent[components.Transform](f.w, f.ctx.PlayerHandle)
			require.Equal(t, float64(30), position.X)
			require.Equal(t, float64(40), position.Y)
		})
	}
}

func TestConstructionCachedOwnershipDoesNotRead(t *testing.T) {
	f := newConstructionFixture(t)
	f.seedHome(t, nil)
	require.True(t, f.runtime.CompleteConstruction(&f.ctx, f.finishSite).OK)
	require.Zero(t, f.reader.reads)
}

func TestConstructionSavedHomeIsReplacedInSameCall(t *testing.T) {
	for _, source := range []string{"live", "raw"} {
		t.Run(source, func(t *testing.T) {
			f := newConstructionFixture(t)
			f.reader.found = true
			f.reader.row = repository.GetCharacterHearthRow{CharacterID: 1, ObjectRegion: 1, ObjectID: 2000, X: 5000, Y: 200}
			var chunk *core.Chunk
			var handle types.Handle
			if source == "live" {
				handle = world.SpawnEntityFromDef(f.w, f.result, world.DefSpawnParams{EntityID: 2000, X: 5000, Y: 200, Region: 1})
			} else {
				chunk, _ = f.oldRaw(t, types.ChunkStateInactive)
				f.chunks.chunks[chunk.Coord] = chunk
				f.runtime.Index = hearth.NewIndex()
			}
			result := f.runtime.CompleteConstruction(&f.ctx, f.finishSite)
			require.True(t, result.OK)
			require.Equal(t, 1, f.reader.reads)
			require.Equal(t, types.EntityID(1000), f.runtime.Index.Snapshot(1).Home.Object.ID)
			if source == "live" {
				info, _ := ecs.GetComponent[components.EntityInfo](f.w, handle)
				require.Equal(t, uint32(16), info.TypeID)
			} else {
				require.Equal(t, 16, chunk.GetRawObjectByID(2000).TypeID)
			}
		})
	}
}

func TestConstructionStaleLookupCannotRestoreOwnership(t *testing.T) {
	for _, scenario := range []string{"revoked", "terminal"} {
		t.Run(scenario, func(t *testing.T) {
			f := newConstructionFixture(t)
			f.reader.found = true
			f.reader.row = repository.GetCharacterHearthRow{CharacterID: 1, ObjectRegion: 1, ObjectID: 2000, X: 5000, Y: 200}
			var changed hearth.Snapshot
			f.reader.onRead = func() {
				var err error
				if scenario == "terminal" {
					changed, err = f.runtime.Index.ApplyTerminal(1)
				} else {
					changed, err = f.runtime.Index.RevokeHome(1)
				}
				require.NoError(t, err)
			}
			calls := 0
			result := f.runtime.CompleteConstruction(&f.ctx, func() bool { calls++; return false })
			require.False(t, result.OK)
			require.Equal(t, changed, f.runtime.Index.Snapshot(1))
			_, owned := f.runtime.Index.Owner(hearth.ObjectKey{Region: 1, ID: 2000})
			require.False(t, owned)
			if scenario == "terminal" {
				require.Zero(t, calls)
			}
			info, _ := ecs.GetComponent[components.EntityInfo](f.w, f.ctx.TargetHandle)
			require.Equal(t, uint32(constt.BuildObjectTypeID), info.TypeID)
		})
	}
}

func TestConstructionReadFailureIsFatal(t *testing.T) {
	f := newConstructionFixture(t)
	f.reader.err = errors.New("database connection lost")
	logger := zap.NewNop().WithOptions(zap.WithFatalHook(zapcore.WriteThenPanic))
	f.runtime = hearth.NewRuntime(f.w, f.chunks, f.reader, f.registry, nil, logger)
	calls := 0
	require.Panics(t, func() {
		f.runtime.CompleteConstruction(&f.ctx, func() bool { calls++; return true })
	}, "database errors must synchronously reach the fatal logger hook")
	require.Equal(t, hearth.Unknown, f.runtime.Index.Snapshot(1).State)
	require.Equal(t, 1, f.reader.reads)
	require.Zero(t, calls)
}

func TestConstructionLoadsOldChunkAndCompletesInSameCall(t *testing.T) {
	f := newConstructionFixture(t)
	chunk, raw := f.oldRaw(t, types.ChunkStateInactive)
	f.chunks.wait = func(coord types.ChunkCoord) error {
		require.Equal(t, chunk.Coord, coord)
		f.chunks.chunks[coord] = chunk
		return nil
	}
	calls := 0
	result := f.runtime.CompleteConstruction(&f.ctx, func() bool {
		calls++
		require.Equal(t, 1, f.chunks.pins[chunk.Coord])
		return f.finishSite()
	})
	require.True(t, result.OK)
	require.Equal(t, 1, calls)
	require.Equal(t, []types.ChunkCoord{chunk.Coord}, f.chunks.waited)
	require.Equal(t, []int{1}, f.chunks.waitPins, "loading must retain the chunk before the database read")
	require.Len(t, f.chunks.waitDeadlines, 1)
	require.Positive(t, time.Until(f.chunks.waitDeadlines[0]))
	require.LessOrEqual(t, time.Until(f.chunks.waitDeadlines[0]), 5*time.Second)
	require.Empty(t, chunk.GetHandles(), "loading must not activate the distant chunk")
	require.Equal(t, types.InvalidHandle, f.w.GetHandleByEntityID(types.EntityID(raw.ID)))
	require.Zero(t, f.chunks.pins[chunk.Coord])
	require.Equal(t, 16, chunk.GetRawObjectByID(2000).TypeID)
}

func TestConstructionOldChunkLoadRefusalPreservesState(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		err   error
		fatal bool
	}{
		{name: "shutdown", err: world.ErrChunkNotLoaded},
		{name: "timeout", err: context.DeadlineExceeded, fatal: true},
		{name: "database_error", err: errors.New("chunk query failed"), fatal: true},
		{name: "not_ready_after_wait"},
		{name: "revoked_during_wait"},
		{name: "terminal_pending_during_wait"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newConstructionFixture(t)
			logger := zap.NewNop().WithOptions(zap.WithFatalHook(zapcore.WriteThenPanic))
			f.runtime = hearth.NewRuntime(f.w, f.chunks, f.reader, f.registry, nil, logger)
			chunk, raw := f.oldRaw(t, types.ChunkStateInactive)
			expected := f.runtime.Index.Snapshot(1)
			f.chunks.wait = func(coord types.ChunkCoord) error {
				f.chunks.chunks[coord] = chunk
				switch scenario.name {
				case "not_ready_after_wait":
					chunk.SetState(types.ChunkStateLoading)
				case "revoked_during_wait":
					_, err := f.runtime.Index.RevokeHome(1)
					require.NoError(t, err)
				case "terminal_pending_during_wait":
					require.NoError(t, f.runtime.Index.MarkTerminalPending(1))
				}
				expected = f.runtime.Index.Snapshot(1)
				return scenario.err
			}
			calls := 0
			attempt := func() {
				result := f.runtime.CompleteConstruction(&f.ctx, func() bool { calls++; return f.finishSite() })
				require.False(t, result.OK)
			}
			if scenario.fatal {
				require.Panics(t, attempt)
			} else {
				attempt()
			}
			require.Zero(t, calls)
			require.Zero(t, f.chunks.pins[chunk.Coord])
			require.Equal(t, []int{1}, f.chunks.waitPins)
			require.Equal(t, expected, f.runtime.Index.Snapshot(1))
			require.Same(t, raw, chunk.GetRawObjectByID(2000))
			require.Empty(t, chunk.GetRawDirtyObjectIDs())
			info, _ := ecs.GetComponent[components.EntityInfo](f.w, f.ctx.TargetHandle)
			require.Equal(t, uint32(constt.BuildObjectTypeID), info.TypeID)
		})
	}
}

func TestConstructionRawReplacementRestoresOrdinaryUnlitCampfire(t *testing.T) {
	for _, state := range []types.ChunkState{types.ChunkStatePreloaded, types.ChunkStateInactive, types.ChunkStateActive} {
		t.Run(state.String(), func(t *testing.T) {
			f := newConstructionFixture(t)
			chunk, raw := f.oldRaw(t, state)
			raw.Data.Valid, raw.Data.RawMessage = true, []byte(`{"v":1,"station":{"current_state":"burning"}}`)
			f.chunks.chunks[chunk.Coord] = chunk
			require.True(t, f.runtime.CompleteConstruction(&f.ctx, f.finishSite).OK)
			require.Zero(t, f.chunks.pins[chunk.Coord])
			converted := chunk.GetRawObjectByID(2000)
			require.NotSame(t, raw, converted)
			require.Equal(t, f.result.DefID, raw.TypeID, "borrowed cache records remain unchanged")
			require.True(t, raw.Data.Valid)
			require.False(t, converted.Data.Valid)
			require.Equal(t, raw.ID, converted.ID)
			require.Equal(t, raw.X, converted.X)
			require.Equal(t, raw.Y, converted.Y)
			require.Equal(t, raw.Heading, converted.Heading)
			require.Equal(t, raw.Quality, converted.Quality)
			require.Equal(t, raw.CreateTick, converted.CreateTick)
			require.Contains(t, chunk.GetRawDirtyObjectIDs(), types.EntityID(2000))
			_, owned := f.runtime.Index.Owner(hearth.ObjectKey{Region: 1, ID: 2000})
			require.False(t, owned)
			restoredWorld := ecs.NewWorldForTesting()
			factory := world.NewObjectFactory(nil)
			handle, err := factory.BuildForChunk(restoredWorld, converted, nil)
			require.NoError(t, err)
			info, _ := ecs.GetComponent[components.EntityInfo](restoredWorld, handle)
			require.NoError(t, f.registry.InitObjectBehaviors(&contracts.BehaviorObjectInitContext{
				World: restoredWorld, Handle: handle, EntityID: 2000, EntityType: info.TypeID, Reason: contracts.ObjectBehaviorInitReasonRestore,
			}, info.Behaviors))
			station, exists := ecs.GetComponent[components.StationState](restoredWorld, handle)
			require.True(t, exists)
			require.Equal(t, "unlit", station.CurrentState)
			internal, _ := ecs.GetComponent[components.ObjectInternalState](restoredWorld, handle)
			burner, exists := components.GetBehaviorState[components.BurnerBehaviorState](internal, "burner")
			require.True(t, exists)
			require.Equal(t, uint32(5), burner.Fuel)
			require.Zero(t, burner.NextFuelBurnAtTick)
			appearance, _ := ecs.GetComponent[components.Appearance](restoredWorld, handle)
			require.Equal(t, "campfire/unlit", appearance.Resource)
		})
	}
}

func TestConstructionFailedCompletionPreservesOldHome(t *testing.T) {
	f := newConstructionFixture(t)
	chunk, raw := f.oldRaw(t, types.ChunkStateInactive)
	f.chunks.chunks[chunk.Coord] = chunk
	before := f.runtime.Index.Snapshot(1)
	calls := 0
	result := f.runtime.CompleteConstruction(&f.ctx, func() bool { calls++; return false })
	require.False(t, result.OK)
	require.Equal(t, 1, calls)
	require.Equal(t, before, f.runtime.Index.Snapshot(1))
	require.Same(t, raw, chunk.GetRawObjectByID(2000))
	require.Empty(t, chunk.GetRawDirtyObjectIDs())
	require.Zero(t, f.chunks.pins[chunk.Coord])
}

func TestConstructionRejectsInvalidActorAndUnderground(t *testing.T) {
	for _, scenario := range []string{"stale_handle", "wrong_actor", "dead_actor", "underground"} {
		t.Run(scenario, func(t *testing.T) {
			f := newConstructionFixture(t)
			f.seedHome(t, nil)
			switch scenario {
			case "stale_handle":
				f.w.Despawn(f.ctx.PlayerHandle)
				f.w.Spawn(f.ctx.PlayerID, nil)
			case "wrong_actor":
				f.ctx.PlayerID = 2
			case "dead_actor":
				ecs.WithComponent(f.w, f.ctx.PlayerHandle, func(health *components.EntityHealth) { health.HHP = 0 })
			case "underground":
				f.w.Layer = 1
			}
			calls := 0
			result := f.runtime.CompleteConstruction(&f.ctx, func() bool { calls++; return true })
			require.False(t, result.OK)
			require.Zero(t, calls)
			require.Equal(t, hearth.NoHome, f.runtime.Index.Snapshot(1).State)
		})
	}
}

func TestConstructionUnavailablePreparationRefusesCompletion(t *testing.T) {
	for _, scenario := range []string{"nil_reader", "nil_chunks_home", "outside_world"} {
		t.Run(scenario, func(t *testing.T) {
			f := newConstructionFixture(t)
			var chunks hearth.ConstructionChunks = f.chunks
			var reader hearth.HomeReader = f.reader
			switch scenario {
			case "nil_reader":
				reader = nil
			case "nil_chunks_home":
				chunks = nil
			case "outside_world":
				f.chunks.pinErr = world.ErrChunkOutsideWorld
			}
			f.runtime = hearth.NewRuntime(f.w, chunks, reader, f.registry, nil, zap.NewNop())
			if scenario == "nil_chunks_home" || scenario == "outside_world" {
				chunk, _ := f.oldRaw(t, types.ChunkStateInactive)
				f.chunks.chunks[chunk.Coord] = chunk
			}
			calls := 0
			result := f.runtime.CompleteConstruction(&f.ctx, func() bool { calls++; return true })
			require.False(t, result.OK)
			require.Zero(t, calls)
			require.Empty(t, f.chunks.pins)
		})
	}
}
