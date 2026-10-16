package hearth_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/hearth"
	"origin/internal/game/world"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func (*constructionSaver) SaveTerminal(context.Context, types.EntityID, hearth.ObjectKey, *repository.Object) error {
	panic("unexpected terminal save")
}

func (restorationSaver) SaveTerminal(context.Context, types.EntityID, hearth.ObjectKey, *repository.Object) error {
	panic("unexpected terminal save")
}

type terminalSave struct {
	character types.EntityID
	previous  hearth.ObjectKey
	converted *repository.Object
}

type terminalSaver struct {
	*constructionSaver
	saves  []terminalSave
	onSave func()
	err    error
}

func (s *terminalSaver) SaveTerminal(_ context.Context, character types.EntityID, previous hearth.ObjectKey, converted *repository.Object) error {
	s.saves = append(s.saves, terminalSave{character, previous, converted})
	if s.onSave != nil {
		s.onSave()
	}
	return s.err
}

func newTerminalFixture(t *testing.T) (*constructionFixture, *terminalSaver) {
	t.Helper()
	f := newConstructionFixture(t)
	saver := &terminalSaver{constructionSaver: f.saver}
	f.runtime = hearth.NewRuntime(f.w, f.chunks, f.reader, f.factory, saver, f.registry, nil,
		zap.NewNop().WithOptions(zap.WithFatalHook(zapcore.WriteThenPanic)))
	return f, saver
}

func requireTerminalComplete(t *testing.T, runtime *hearth.Runtime, character types.EntityID) types.Handle {
	t.Helper()
	handle, err := runtime.CompleteTerminal(character)
	require.NoError(t, err)
	return handle
}

func terminalLiveHome(t *testing.T, f *constructionFixture) (*core.Chunk, types.Handle) {
	t.Helper()
	chunk, raw := f.oldRaw(t, types.ChunkStateActive)
	f.chunks.chunks[chunk.Coord] = chunk
	handle := world.SpawnEntityFromDef(f.w, f.result, world.DefSpawnParams{EntityID: types.EntityID(raw.ID), X: float64(raw.X), Y: float64(raw.Y), Region: raw.Region, Quality: uint32(raw.Quality)})
	ecs.AddComponent(f.w, handle, components.ChunkRef{CurrentChunkX: chunk.Coord.X, CurrentChunkY: chunk.Coord.Y})
	chunk.Spatial().AddStatic(handle, raw.X, raw.Y)
	return chunk, handle
}

func TestTerminalNoHomeNeedsNoWorldOrIOAndIsIdempotent(t *testing.T) {
	runtime := hearth.NewRuntime(nil, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, runtime.Index.LoadIfUnknown(1, nil))
	require.NoError(t, runtime.NotifyTerminal(1))
	require.True(t, runtime.Index.Snapshot(1).PendingTerminal)
	require.Equal(t, types.InvalidHandle, requireTerminalComplete(t, &runtime, 1))
	require.Equal(t, hearth.Snapshot{State: hearth.NoHome, Terminal: true}, runtime.Index.Snapshot(1))
	require.NoError(t, runtime.NotifyTerminal(1))
	require.Equal(t, types.InvalidHandle, requireTerminalComplete(t, &runtime, 1))
	_, err := runtime.Index.ReplaceHome(1, hearth.Home{Object: hearth.ObjectKey{Region: 1, ID: 2}})
	require.ErrorIs(t, err, hearth.ErrTerminal)
}

func TestTerminalNotificationOnlyChangesIndexEligibility(t *testing.T) {
	f, saver := newTerminalFixture(t)
	chunk, raw := f.oldRaw(t, types.ChunkStateInactive)
	f.reader.err = errors.New("notification must not read")
	f.chunks.pinErr = errors.New("notification must not pin")
	require.NoError(t, f.runtime.NotifyTerminal(1))
	require.NoError(t, f.runtime.NotifyTerminal(1))
	current := f.runtime.Index.Snapshot(1)
	require.True(t, current.PendingTerminal)
	require.Equal(t, hearth.CurrentHome, current.State)
	require.Equal(t, types.EntityID(2000), current.Home.Object.ID)
	require.False(t, current.CanReturn())
	require.False(t, current.CanAcquire())
	owner, snapshot, exists := f.runtime.Index.SnapshotByObject(current.Home.Object)
	require.True(t, exists, "pending metadata protects the source from orphan normalization")
	require.Equal(t, types.EntityID(1), owner)
	require.Equal(t, current, snapshot)
	require.Zero(t, f.reader.reads)
	require.Empty(t, f.chunks.pins)
	require.Empty(t, saver.saves)
	require.Same(t, raw, chunk.GetRawObjectByID(2000))
}

func TestTerminalUnknownAndInvalidCharacterAreFatalWithoutIO(t *testing.T) {
	for _, operation := range []string{"notify", "complete"} {
		for _, character := range []types.EntityID{0, 1, ^types.EntityID(0)} {
			t.Run(fmt.Sprintf("%s/%d", operation, character), func(t *testing.T) {
				f, saver := newTerminalFixture(t)
				savedReads := 0
				deletionFixtureReader(f, saver, func(context.Context, int64) (repository.GetSavedCharacterHearthRow, error) {
					savedReads++
					return repository.GetSavedCharacterHearthRow{}, sql.ErrNoRows
				})
				require.Panics(t, func() {
					if operation == "notify" {
						_ = f.runtime.NotifyTerminal(character)
					} else {
						_, _ = f.runtime.CompleteTerminal(character)
					}
				})
				require.Zero(t, f.reader.reads)
				require.Zero(t, savedReads, "actual death must never resolve Unknown through the deletion lookup")
				require.Empty(t, f.chunks.pins)
				require.Empty(t, saver.saves)
				require.False(t, f.runtime.Index.Snapshot(character).Terminal)
			})
		}
	}
}

func TestTerminalLiveHomeConvertsInPlaceAndCommitsInSameCall(t *testing.T) {
	f, saver := newTerminalFixture(t)
	chunk, handle := terminalLiveHome(t, f)
	oldInfo, _ := ecs.GetComponent[components.EntityInfo](f.w, handle)
	oldPosition, _ := ecs.GetComponent[components.Transform](f.w, handle)
	links := ecs.GetResource[ecs.LinkState](f.w)
	links.SetLink(ecs.PlayerLink{PlayerID: 1, PlayerHandle: f.ctx.PlayerHandle, TargetID: 2000, TargetHandle: handle})
	f.w.AddComponentObserver(components.EntityInfoComponentID, func(h types.Handle) {
		if h == handle {
			require.Empty(t, links.PlayersByTarget[2000], "links must break before in-place conversion")
		}
	})
	saver.onSave = func() {
		require.Equal(t, 1, f.chunks.pins[chunk.Coord])
		require.Equal(t, hearth.Snapshot{State: hearth.NoHome, Terminal: true}, f.runtime.Index.Snapshot(1))
	}
	require.NoError(t, f.runtime.NotifyTerminal(1))
	require.Equal(t, handle, requireTerminalComplete(t, &f.runtime, 1))
	require.True(t, f.w.Alive(handle))
	require.Equal(t, handle, f.w.GetHandleByEntityID(2000))
	require.Equal(t, []types.Handle{handle}, chunk.GetHandles())
	info, _ := ecs.GetComponent[components.EntityInfo](f.w, handle)
	campfire, found := objectdefs.Global().GetByKey("campfire")
	require.True(t, found)
	require.Equal(t, uint32(campfire.DefID), info.TypeID)
	require.Equal(t, oldInfo.Region, info.Region)
	require.Equal(t, oldInfo.Quality, info.Quality)
	position, _ := ecs.GetComponent[components.Transform](f.w, handle)
	require.Equal(t, oldPosition, position)
	station, exists := ecs.GetComponent[components.StationState](f.w, handle)
	require.True(t, exists)
	require.Equal(t, "unlit", station.CurrentState)
	state, _ := ecs.GetComponent[components.ObjectInternalState](f.w, handle)
	require.True(t, state.IsDirty)
	require.True(t, state.HasHP)
	require.Equal(t, float64(campfire.HP), state.HP)
	appearance, _ := ecs.GetComponent[components.Appearance](f.w, handle)
	require.Equal(t, "campfire/unlit", appearance.Resource)
	require.Len(t, saver.saves, 1)
	saved := saver.saves[0]
	require.Equal(t, types.EntityID(1), saved.character)
	require.Equal(t, hearth.ObjectKey{Region: 1, ID: 2000}, saved.previous)
	require.Equal(t, int64(2000), saved.converted.ID)
	require.Equal(t, campfire.DefID, saved.converted.TypeID)
	require.Equal(t, int64(123), saved.converted.LastTick)
	require.Equal(t, sql.NullFloat64{Float64: float64(campfire.HP), Valid: true}, saved.converted.Hp)
	require.Zero(t, f.reader.reads)
	require.Zero(t, f.chunks.pins[chunk.Coord])
	require.NoError(t, f.runtime.NotifyTerminal(1))
	require.Equal(t, types.InvalidHandle, requireTerminalComplete(t, &f.runtime, 1))
	require.Len(t, saver.saves, 1, "duplicates perform no second save or conversion")
	require.Len(t, f.chunks.waited, 1)
}

func TestTerminalRawAndUnloadedHomeConvertWithoutActivation(t *testing.T) {
	for _, source := range []string{"active", "preloaded", "inactive", "unloaded"} {
		t.Run(source, func(t *testing.T) {
			f, saver := newTerminalFixture(t)
			state := types.ChunkStateInactive
			if source == "active" {
				state = types.ChunkStateActive
			} else if source == "preloaded" {
				state = types.ChunkStatePreloaded
			}
			chunk, raw := f.oldRaw(t, state)
			raw.Data.Valid, raw.Data.RawMessage = true, []byte(`{"old":"hearth"}`)
			if source == "unloaded" {
				f.chunks.wait = func(coord types.ChunkCoord) error {
					f.chunks.chunks[coord] = chunk
					return nil
				}
			} else {
				f.chunks.chunks[chunk.Coord] = chunk
			}
			saver.onSave = func() {
				require.Equal(t, 1, f.chunks.pins[chunk.Coord])
				require.NotSame(t, raw, chunk.GetRawObjectByID(2000))
				require.True(t, f.runtime.Index.Snapshot(1).Terminal)
			}
			require.NoError(t, f.runtime.NotifyTerminal(1))
			require.Equal(t, types.InvalidHandle, requireTerminalComplete(t, &f.runtime, 1))
			require.Len(t, saver.saves, 1)
			converted := saver.saves[0].converted
			require.Same(t, converted, chunk.GetRawObjectByID(2000))
			require.Equal(t, raw.ID, converted.ID)
			require.Equal(t, raw.Region, converted.Region)
			require.Equal(t, raw.X, converted.X)
			require.Equal(t, raw.Y, converted.Y)
			require.Equal(t, raw.Heading, converted.Heading)
			require.Equal(t, raw.Quality, converted.Quality)
			require.Equal(t, raw.CreateTick, converted.CreateTick)
			require.Equal(t, 16, converted.TypeID)
			require.False(t, converted.Data.Valid)
			require.Equal(t, int64(123), converted.LastTick)
			require.Equal(t, f.result.DefID, raw.TypeID, "the borrowed source was not mutated")
			require.True(t, raw.Data.Valid)
			require.Contains(t, chunk.GetRawDirtyObjectIDs(), types.EntityID(2000))
			require.Empty(t, chunk.GetHandles())
			require.Equal(t, types.InvalidHandle, f.w.GetHandleByEntityID(2000))
			require.Equal(t, []int{1}, f.chunks.waitPins)
			require.InDelta(t, 5, time.Until(f.chunks.waitDeadlines[0]).Seconds(), .5)
			require.Zero(t, f.chunks.pins[chunk.Coord])
			require.Zero(t, f.reader.reads)
		})
	}
}

func TestTerminalUnavailableRawSourceOnlyRemovesAssociation(t *testing.T) {
	for _, scenario := range []string{"missing", "deleted", "tombstone", "changed_type", "wrong_region", "wrong_layer"} {
		t.Run(scenario, func(t *testing.T) {
			f, saver := newTerminalFixture(t)
			chunk, raw := f.oldRaw(t, types.ChunkStateInactive)
			f.chunks.chunks[chunk.Coord] = chunk
			switch scenario {
			case "missing":
				chunk.RemoveRawObjectByID(2000)
			case "deleted":
				raw.DeletedAt = sql.NullTime{Time: time.Now(), Valid: true}
			case "tombstone":
				chunk.MarkDeletedObjectID(2000)
			case "changed_type":
				raw.TypeID = 16
			case "wrong_region":
				raw.Region++
			case "wrong_layer":
				raw.Layer++
			}
			before := *raw
			dirty := chunk.GetRawDirtyObjectIDs()
			require.NoError(t, f.runtime.NotifyTerminal(1))
			require.Equal(t, types.InvalidHandle, requireTerminalComplete(t, &f.runtime, 1))
			require.Len(t, saver.saves, 1)
			require.Nil(t, saver.saves[0].converted)
			require.Equal(t, hearth.ObjectKey{Region: 1, ID: 2000}, saver.saves[0].previous)
			require.Equal(t, before, *raw)
			require.Equal(t, dirty, chunk.GetRawDirtyObjectIDs())
			require.True(t, f.runtime.Index.Snapshot(1).Terminal)
			require.Zero(t, f.chunks.pins[chunk.Coord])
			if scenario == "tombstone" {
				require.Contains(t, chunk.GetDeletedObjectIDs(), types.EntityID(2000))
			}
		})
	}
}

func TestTerminalUnavailableLiveSourceNeverFallsBackToStaleRaw(t *testing.T) {
	for _, scenario := range []string{"pending_destruction", "reserved", "changed_type", "wrong_region", "wrong_layer", "tombstone"} {
		t.Run(scenario, func(t *testing.T) {
			f, saver := newTerminalFixture(t)
			chunk, handle := terminalLiveHome(t, f)
			raw := chunk.GetRawObjectByID(2000)
			switch scenario {
			case "pending_destruction":
				ecs.InitResource(f.w, ecs.ObjectDestructionState{Pending: map[types.Handle]bool{handle: true}})
			case "reserved":
				require.True(t, ecs.ReserveInventoryOwner(f.w, 2000, handle))
			case "changed_type":
				ecs.WithComponent(f.w, handle, func(info *components.EntityInfo) { info.TypeID = 16 })
			case "wrong_region":
				ecs.WithComponent(f.w, handle, func(info *components.EntityInfo) { info.Region++ })
			case "wrong_layer":
				ecs.WithComponent(f.w, handle, func(info *components.EntityInfo) { info.Layer++ })
			case "tombstone":
				chunk.MarkDeletedObjectID(2000)
			}
			before, _ := ecs.GetComponent[components.EntityInfo](f.w, handle)
			require.NoError(t, f.runtime.NotifyTerminal(1))
			require.Equal(t, types.InvalidHandle, requireTerminalComplete(t, &f.runtime, 1))
			require.Len(t, saver.saves, 1)
			require.Nil(t, saver.saves[0].converted)
			after, _ := ecs.GetComponent[components.EntityInfo](f.w, handle)
			require.Equal(t, before, after)
			require.Same(t, raw, chunk.GetRawObjectByID(2000))
			require.Empty(t, chunk.GetRawDirtyObjectIDs())
		})
	}
}

func TestTerminalUsesHomeFromAlreadyAdmittedConstruction(t *testing.T) {
	f, saver := newTerminalFixture(t)
	oldChunk, _ := f.oldRaw(t, types.ChunkStateInactive)
	f.chunks.chunks[oldChunk.Coord] = oldChunk
	newChunk := core.NewChunk(types.ChunkCoord{}, 1, hearth.SurfaceLayer, constt.ChunkSize)
	newChunk.SetState(types.ChunkStateActive)
	f.chunks.chunks[newChunk.Coord] = newChunk
	result := f.runtime.CompleteConstruction(&f.ctx, func() bool {
		require.NoError(t, f.runtime.NotifyTerminal(1))
		return f.finishSite()
	})
	require.True(t, result.OK)
	require.True(t, f.runtime.Index.Snapshot(1).PendingTerminal)
	require.Equal(t, types.EntityID(1000), f.runtime.Index.Snapshot(1).Home.Object.ID)
	require.Equal(t, f.w.GetHandleByEntityID(1000), requireTerminalComplete(t, &f.runtime, 1))
	require.Len(t, saver.saves, 1)
	require.Equal(t, hearth.ObjectKey{Region: 1, ID: 1000}, saver.saves[0].previous)
	require.Equal(t, int64(1000), saver.saves[0].converted.ID)
	require.Equal(t, 16, saver.saves[0].converted.TypeID)
	require.True(t, f.runtime.Index.Snapshot(1).Terminal)
}

func TestTerminalWaitsForSaveAndRetainsPinWhileNotificationRemainsNonblocking(t *testing.T) {
	f, saver := newTerminalFixture(t)
	chunk, _ := f.oldRaw(t, types.ChunkStateInactive)
	f.chunks.chunks[chunk.Coord] = chunk
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	saver.onSave = func() { close(entered); <-release }
	require.NoError(t, f.runtime.NotifyTerminal(1))
	go func() { _, err := f.runtime.CompleteTerminal(1); done <- err }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("terminal completion did not reach save")
	}
	require.Equal(t, 1, f.chunks.pins[chunk.Coord])
	require.True(t, f.runtime.Index.Snapshot(1).Terminal)
	require.NoError(t, f.runtime.NotifyTerminal(1), "notification cannot wait on the owner or SQL")
	select {
	case err := <-done:
		t.Fatalf("completion returned before save was released: %v", err)
	default:
	}
	close(release)
	require.NoError(t, <-done)
	require.Zero(t, f.chunks.pins[chunk.Coord])
	require.Len(t, saver.saves, 1)
}

func TestTerminalPreparationErrorsAreFatalWithoutMutation(t *testing.T) {
	for _, scenario := range []string{"pin", "load", "load_timeout", "not_loaded", "missing_chunk", "loading_chunk", "wrong_chunk_region"} {
		t.Run(scenario, func(t *testing.T) {
			f, saver := newTerminalFixture(t)
			chunk, raw := f.oldRaw(t, types.ChunkStateInactive)
			f.chunks.chunks[chunk.Coord] = chunk
			switch scenario {
			case "pin":
				f.chunks.pinErr = world.ErrChunkOutsideWorld
			case "load":
				f.chunks.wait = func(types.ChunkCoord) error { return errors.New("load failed") }
			case "load_timeout":
				f.chunks.wait = func(types.ChunkCoord) error { return context.DeadlineExceeded }
			case "not_loaded":
				f.chunks.wait = func(types.ChunkCoord) error { return world.ErrChunkNotLoaded }
			case "missing_chunk":
				delete(f.chunks.chunks, chunk.Coord)
			case "loading_chunk":
				chunk.SetState(types.ChunkStateLoading)
			case "wrong_chunk_region":
				chunk.Region++
			}
			require.NoError(t, f.runtime.NotifyTerminal(1))
			before := f.runtime.Index.Snapshot(1)
			require.Panics(t, func() { _, _ = f.runtime.CompleteTerminal(1) })
			require.Equal(t, before, f.runtime.Index.Snapshot(1))
			require.Same(t, raw, chunk.GetRawObjectByID(2000))
			require.Empty(t, chunk.GetRawDirtyObjectIDs())
			require.Empty(t, saver.saves)
			require.Zero(t, f.chunks.pins[chunk.Coord])
		})
	}
}

func TestTerminalSaveErrorsAreFatalAfterConversionAndRevocation(t *testing.T) {
	for _, failure := range []error{errors.New("write failed"), errors.New("commit failed"), context.DeadlineExceeded} {
		t.Run(failure.Error(), func(t *testing.T) {
			f, saver := newTerminalFixture(t)
			chunk, _ := f.oldRaw(t, types.ChunkStateInactive)
			f.chunks.chunks[chunk.Coord] = chunk
			saver.err = failure
			saver.onSave = func() { require.Equal(t, 1, f.chunks.pins[chunk.Coord]) }
			require.NoError(t, f.runtime.NotifyTerminal(1))
			require.Panics(t, func() { _, _ = f.runtime.CompleteTerminal(1) })
			require.Len(t, saver.saves, 1)
			require.True(t, f.runtime.Index.Snapshot(1).Terminal)
			require.Equal(t, 16, chunk.GetRawObjectByID(2000).TypeID)
			require.Contains(t, chunk.GetRawDirtyObjectIDs(), types.EntityID(2000))
			require.Zero(t, f.chunks.pins[chunk.Coord])
		})
	}
}

func TestTerminalSerializationFailureIsFatalAfterConversionAndRevocation(t *testing.T) {
	f, saver := newTerminalFixture(t)
	chunk, handle := terminalLiveHome(t, f)
	ecs.RemoveComponent[components.ChunkRef](f.w, handle)
	require.NoError(t, f.runtime.NotifyTerminal(1))
	require.Panics(t, func() { _, _ = f.runtime.CompleteTerminal(1) })
	require.Empty(t, saver.saves)
	require.True(t, f.runtime.Index.Snapshot(1).Terminal)
	info, _ := ecs.GetComponent[components.EntityInfo](f.w, handle)
	require.Equal(t, uint32(16), info.TypeID)
	require.Zero(t, f.chunks.pins[chunk.Coord])
}
