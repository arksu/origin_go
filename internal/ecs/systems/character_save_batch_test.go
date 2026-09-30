package systems

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type characterSaveInventoryFunc func(interface{}, types.EntityID, types.Handle) []InventorySnapshot

func (f characterSaveInventoryFunc) SerializeInventories(w interface{}, entityID types.EntityID, handle types.Handle) []InventorySnapshot {
	return f(w, entityID, handle)
}

func characterSaveTestSnapshot(id int64, value int) CharacterSnapshot {
	return CharacterSnapshot{
		CharacterID: id, X: value, Y: value + 1, Heading: int16(value + 2),
		Stamina: float64(value + 3), Energy: float64(value + 4), SHP: int16(value + 5), HHP: int16(value + 6),
		Attributes: fmt.Sprintf(`{"strength":%d}`, value), Exp: fmt.Sprintf(`{"LP":%d}`, value),
		Skills: fmt.Sprintf(`["skill%d"]`, value), Discovery: fmt.Sprintf(`["discovery%d"]`, value),
	}
}

func characterSaveTestInventory(owner int64, kind, key int16, version int, label string) InventorySnapshot {
	return InventorySnapshot{CharacterID: owner, Kind: kind, InventoryKey: key, Version: version, Data: json.RawMessage(fmt.Sprintf(`{"label":%q}`, label))}
}

func characterSavePending(t *testing.T, s *CharacterSaver, id int64) *CharacterSnapshot {
	t.Helper()
	queue := s.queueForCharacter(id)
	queue.mu.Lock()
	defer queue.mu.Unlock()
	return queue.pending[id]
}

func TestCharacterSaveBatchCoalescesLatestSnapshotAndInventoryKeys(t *testing.T) {
	var characters repository.UpdateCharactersParams
	var inventories repository.UpsertInventoriesParams
	calls := 0
	saver := newCharacterSaver(0, nil, zap.NewNop(), func(_ context.Context, chars repository.UpdateCharactersParams, invs repository.UpsertInventoriesParams) error {
		calls++
		characters, inventories = chars, invs
		return nil
	})
	older := characterSaveTestSnapshot(10, 1)
	older.Inventories = []InventorySnapshot{
		characterSaveTestInventory(10, 0, 0, 100, "obsolete snapshot"),
		characterSaveTestInventory(999, 0, 0, 100, "removed nested inventory"),
	}
	latest := characterSaveTestSnapshot(10, 20)
	latest.Inventories = []InventorySnapshot{
		characterSaveTestInventory(10, 0, 0, 4, "earlier tie"),
		characterSaveTestInventory(10, 0, 0, 2, "stale version"),
		characterSaveTestInventory(10, 0, 0, 4, "latest tie"),
		characterSaveTestInventory(10, 0, 1, 1, "different key"),
		characterSaveTestInventory(10, 1, 0, 1, "different kind"),
		characterSaveTestInventory(11, 0, 0, 1, "different owner"),
	}
	other := characterSaveTestSnapshot(20, 30)
	other.Inventories = []InventorySnapshot{
		characterSaveTestInventory(10, 0, 0, 3, "shared lower version"),
		characterSaveTestInventory(20, 0, 0, 2, "other character"),
	}
	for _, snapshot := range []CharacterSnapshot{older, other, latest} {
		require.True(t, saver.enqueueSnapshot(snapshot))
	}
	require.NoError(t, saver.flushPending(context.Background(), saver.queueForCharacter(10), 0))
	require.Equal(t, 1, calls)
	require.ElementsMatch(t, []int{10, 20}, characters.Ids)
	for index, id := range characters.Ids {
		want := latest
		if id == 20 {
			want = other
		}
		require.Equal(t, float64(want.X), characters.Xs[index])
		require.Equal(t, float64(want.Y), characters.Ys[index])
		require.Equal(t, float64(want.Heading), characters.Headings[index])
		require.Equal(t, want.Stamina, characters.Staminas[index])
		require.Equal(t, want.Energy, characters.Energies[index])
		require.Equal(t, int(want.SHP), characters.Shps[index])
		require.Equal(t, int(want.HHP), characters.Hhps[index])
		require.Equal(t, want.Attributes, characters.Attributes[index])
		require.Equal(t, want.Exp, characters.Exps[index])
		require.Equal(t, want.Skills, characters.Skills[index])
		require.Equal(t, want.Discovery, characters.Discovery[index])
	}
	wantInventories := map[[3]int64]InventorySnapshot{}
	for _, inv := range []InventorySnapshot{latest.Inventories[2], latest.Inventories[3], latest.Inventories[4], latest.Inventories[5], other.Inventories[1]} {
		wantInventories[[3]int64{inv.CharacterID, int64(inv.Kind), int64(inv.InventoryKey)}] = inv
	}
	require.Len(t, inventories.OwnerIds, len(wantInventories))
	seen := make(map[[3]int64]bool)
	for index, owner := range inventories.OwnerIds {
		key := [3]int64{owner, int64(inventories.Kinds[index]), int64(inventories.InventoryKeys[index])}
		require.False(t, seen[key], "duplicate inventory key would cause PostgreSQL SQLSTATE 21000: %v", key)
		seen[key] = true
		want, exists := wantInventories[key]
		require.True(t, exists, "obsolete inventories must not survive a newer complete character snapshot")
		require.Equal(t, want.Version, inventories.Versions[index])
		require.JSONEq(t, string(want.Data), inventories.Datas[index])
	}
	require.Nil(t, characterSavePending(t, saver, 10))
	require.Nil(t, characterSavePending(t, saver, 20))
}

func TestCharacterSaveBatchRetainsFailedSnapshotForRetry(t *testing.T) {
	for _, failedWrite := range []string{"character update", "inventory upsert"} {
		t.Run(failedWrite, func(t *testing.T) {
			failure := errors.New(failedWrite + " failed")
			var attempts []repository.UpdateCharactersParams
			var inventoryAttempts []repository.UpsertInventoriesParams
			saver := newCharacterSaver(0, nil, zap.NewNop(), func(_ context.Context, chars repository.UpdateCharactersParams, invs repository.UpsertInventoriesParams) error {
				attempts = append(attempts, chars)
				inventoryAttempts = append(inventoryAttempts, invs)
				if len(attempts) == 1 {
					return failure
				}
				return nil
			})
			snapshot := characterSaveTestSnapshot(10, 12)
			snapshot.Inventories = []InventorySnapshot{characterSaveTestInventory(10, 0, 0, 5, "must survive failure")}
			require.True(t, saver.enqueueSnapshot(snapshot))
			pending := characterSavePending(t, saver, 10)
			queue := saver.queueForCharacter(10)
			require.ErrorIs(t, saver.flushPending(context.Background(), queue, 0), failure)
			require.Same(t, pending, characterSavePending(t, saver, 10))
			require.NoError(t, saver.flushPending(context.Background(), queue, 0))
			require.Len(t, attempts, 2)
			require.Equal(t, attempts[0], attempts[1])
			require.Equal(t, inventoryAttempts[0], inventoryAttempts[1])
			require.Nil(t, characterSavePending(t, saver, 10))
			require.NoError(t, saver.flushPending(context.Background(), queue, 0))
			require.Len(t, attempts, 2, "a successful retry must clear the saved snapshot")
		})
	}
}

func TestCharacterSaveBatchKeepsNewSnapshotEnqueuedDuringWrite(t *testing.T) {
	for _, firstWriteFails := range []bool{false, true} {
		t.Run(fmt.Sprintf("first_write_fails_%t", firstWriteFails), func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
			var attempts []repository.UpdateCharactersParams
			failure := errors.New("temporary write failure")
			saver := newCharacterSaver(0, nil, zap.NewNop(), func(ctx context.Context, chars repository.UpdateCharactersParams, _ repository.UpsertInventoriesParams) error {
				attempts = append(attempts, chars)
				if len(attempts) == 1 {
					close(started)
					select {
					case <-release:
					case <-ctx.Done():
						return ctx.Err()
					}
					if firstWriteFails {
						return failure
					}
				}
				return nil
			})
			require.True(t, saver.enqueueSnapshot(characterSaveTestSnapshot(10, 1)))
			written := make(chan error, 1)
			go func() { written <- saver.flushPending(context.Background(), saver.queueForCharacter(10), 0) }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("older write did not start")
			}
			require.True(t, saver.enqueueSnapshot(characterSaveTestSnapshot(10, 2)))
			newer := characterSavePending(t, saver, 10)
			releaseOnce.Do(func() { close(release) })
			if firstWriteFails {
				require.ErrorIs(t, <-written, failure)
			} else {
				require.NoError(t, <-written)
			}
			require.Same(t, newer, characterSavePending(t, saver, 10))
			require.NoError(t, saver.flushPending(context.Background(), saver.queueForCharacter(10), 0))
			require.Len(t, attempts, 2)
			require.Equal(t, []float64{1}, attempts[0].Xs)
			require.Equal(t, []float64{2}, attempts[1].Xs)
			require.Nil(t, characterSavePending(t, saver, 10))
		})
	}
}

func newCharacterSaveTestPlayer() (*ecs.World, types.Handle) {
	w := ecs.NewWorldForTesting()
	handle := w.Spawn(10, nil)
	ecs.AddComponent(w, handle, components.Transform{X: 1, Y: 2})
	ecs.AddComponent(w, handle, components.EntityStats{Stamina: 80, Energy: 90})
	return w, handle
}

func TestCharacterSavePeriodicThenExpiryPersistsLatestAfterDespawn(t *testing.T) {
	var writes []repository.UpdateCharactersParams
	var inventoryWrites []repository.UpsertInventoriesParams
	inventorySaver := characterSaveInventoryFunc(func(w interface{}, id types.EntityID, handle types.Handle) []InventorySnapshot {
		transform, _ := ecs.GetComponent[components.Transform](w.(*ecs.World), handle)
		return []InventorySnapshot{characterSaveTestInventory(int64(id), 0, 0, int(transform.X), fmt.Sprintf("position %d", int(transform.X)))}
	})
	saver := newCharacterSaver(0, inventorySaver, zap.NewNop(), func(_ context.Context, chars repository.UpdateCharactersParams, invs repository.UpsertInventoriesParams) error {
		writes = append(writes, chars)
		inventoryWrites = append(inventoryWrites, invs)
		return nil
	})
	w, handle := newCharacterSaveTestPlayer()
	saver.Save(w, 10, handle)
	ecs.WithComponent(w, handle, func(transform *components.Transform) { transform.X = 9 })
	saver.SaveDetached(w, 10, handle)
	require.True(t, w.Despawn(handle))
	require.NoError(t, saver.flushPending(context.Background(), saver.queueForCharacter(10), 0))
	require.Len(t, writes, 1)
	require.Equal(t, []int{10}, writes[0].Ids)
	require.Equal(t, []float64{9}, writes[0].Xs)
	require.Equal(t, []int64{10}, inventoryWrites[0].OwnerIds)
	require.Equal(t, []int{9}, inventoryWrites[0].Versions)
	require.JSONEq(t, `{"label":"position 9"}`, inventoryWrites[0].Datas[0])
}

func TestCharacterSaveSyncOrdersAfterOlderWrite(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	var writes []repository.UpdateCharactersParams
	saver := newCharacterSaver(0, characterSaveInventoryFunc(func(interface{}, types.EntityID, types.Handle) []InventorySnapshot { return nil }), zap.NewNop(), func(ctx context.Context, chars repository.UpdateCharactersParams, _ repository.UpsertInventoriesParams) error {
		writes = append(writes, chars)
		if len(writes) == 1 {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})
	w, handle := newCharacterSaveTestPlayer()
	saver.Save(w, 10, handle)
	olderDone := make(chan error, 1)
	go func() { olderDone <- saver.flushPending(context.Background(), saver.queueForCharacter(10), 0) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("older write did not start")
	}
	ecs.WithComponent(w, handle, func(transform *components.Transform) { transform.X = 20 })
	syncDone := make(chan error, 1)
	go func() { syncDone <- saver.SaveSync(w, 10, handle) }()
	require.Eventually(t, func() bool {
		pending := characterSavePending(t, saver, 10)
		return pending != nil && pending.X == 20
	}, time.Second, time.Millisecond)
	select {
	case err := <-syncDone:
		t.Fatalf("SaveSync bypassed older in-flight write: %v", err)
	default:
	}
	releaseOnce.Do(func() { close(release) })
	require.NoError(t, <-olderDone)
	require.NoError(t, <-syncDone)
	require.Len(t, writes, 2)
	require.Equal(t, []float64{1}, writes[0].Xs)
	require.Equal(t, []float64{20}, writes[1].Xs)
	require.Nil(t, characterSavePending(t, saver, 10))
	require.NoError(t, saver.flushPending(context.Background(), saver.queueForCharacter(10), 0))
	require.Len(t, writes, 2, "older work must not overwrite successful SaveSync")
}

func TestCharacterSaveSyncFlushesOnlyTargetAndRetainsFailure(t *testing.T) {
	failure := errors.New("sync write failed")
	var writes []repository.UpdateCharactersParams
	saver := newCharacterSaver(0, characterSaveInventoryFunc(func(interface{}, types.EntityID, types.Handle) []InventorySnapshot { return nil }), zap.NewNop(), func(_ context.Context, chars repository.UpdateCharactersParams, _ repository.UpsertInventoriesParams) error {
		writes = append(writes, chars)
		if len(writes) == 1 {
			return failure
		}
		return nil
	})
	w, handle := newCharacterSaveTestPlayer()
	require.True(t, saver.enqueueSnapshot(characterSaveTestSnapshot(20, 30)))
	require.ErrorIs(t, saver.SaveSync(w, 10, handle), failure)
	require.Equal(t, []int{10}, writes[0].Ids)
	require.NotNil(t, characterSavePending(t, saver, 10))
	require.NotNil(t, characterSavePending(t, saver, 20))
	require.NoError(t, saver.SaveSync(w, 10, handle))
	require.Equal(t, []int{10}, writes[1].Ids)
	require.Nil(t, characterSavePending(t, saver, 10))
	require.NotNil(t, characterSavePending(t, saver, 20))
}

func TestCharacterSaveSyncRejectsIncompleteCharacter(t *testing.T) {
	for _, missing := range []string{"Transform", "EntityStats"} {
		t.Run(missing, func(t *testing.T) {
			writes := 0
			saver := newCharacterSaver(0, nil, zap.NewNop(), func(context.Context, repository.UpdateCharactersParams, repository.UpsertInventoriesParams) error {
				writes++
				return nil
			})
			world, handle := newCharacterSaveTestPlayer()
			if missing == "Transform" {
				ecs.RemoveComponent[components.Transform](world, handle)
			} else {
				ecs.RemoveComponent[components.EntityStats](world, handle)
			}
			err := saver.SaveSync(world, 10, handle)
			require.ErrorContains(t, err, missing)
			require.Zero(t, writes)
			require.Nil(t, characterSavePending(t, saver, 10))
		})
	}
}

func TestCharacterSaveWorkerRetriesWithoutAnotherEnqueue(t *testing.T) {
	attempts := make(chan repository.UpdateCharactersParams, 4)
	failure := errors.New("temporary database error")
	calls := 0
	saver := newCharacterSaver(1, nil, zap.NewNop(), func(_ context.Context, chars repository.UpdateCharactersParams, _ repository.UpsertInventoriesParams) error {
		calls++
		attempts <- chars
		if calls == 1 {
			return failure
		}
		return nil
	})
	t.Cleanup(saver.Stop)
	require.True(t, saver.enqueueSnapshot(characterSaveTestSnapshot(10, 50)))
	for range 2 {
		select {
		case params := <-attempts:
			require.Equal(t, []int{10}, params.Ids)
			require.Equal(t, []float64{50}, params.Xs)
		case <-time.After(3 * time.Second):
			t.Fatal("worker did not retain and retry failed snapshot")
		}
	}
	require.Eventually(t, func() bool { return characterSavePending(t, saver, 10) == nil }, time.Second, time.Millisecond)
}

func TestCharacterSaveStopRetriesAndDrainsEveryPendingBatch(t *testing.T) {
	attempts := 0
	saved := make(map[int]int)
	saver := newCharacterSaver(0, nil, zap.NewNop(), func(ctx context.Context, chars repository.UpdateCharactersParams, _ repository.UpsertInventoriesParams) error {
		require.NoError(t, ctx.Err(), "shutdown writes require a live context")
		require.LessOrEqual(t, len(chars.Ids), batchSize)
		attempts++
		if attempts == 1 {
			return errors.New("transient shutdown write failure")
		}
		for index, id := range chars.Ids {
			require.NotContains(t, saved, id)
			saved[id] = int(chars.Xs[index])
		}
		return nil
	})
	const finalSnapshots = 1205 // Exceeds the old bounded channel's 1000-snapshot limit.
	for id := 1; id <= finalSnapshots; id++ {
		require.True(t, saver.enqueueSnapshot(characterSaveTestSnapshot(int64(id), id)))
	}
	saver.Stop()
	require.Len(t, saved, finalSnapshots)
	require.GreaterOrEqual(t, attempts, (finalSnapshots+batchSize-1)/batchSize+1)
	for id, value := range saved {
		require.Equal(t, id, value)
		require.Nil(t, characterSavePending(t, saver, int64(id)))
	}
	require.False(t, saver.enqueueSnapshot(characterSaveTestSnapshot(999, 999)), "stopped saver must reject new work")
}

func TestCharacterSaveWorkersSerializeSameCharacterAndAllowOtherQueue(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	type completedWrite struct {
		id    int
		value float64
	}
	completed := make(chan completedWrite, 4)
	concurrentSameCharacter := make(chan int, 1)
	var activeMu sync.Mutex
	active := make(map[int]bool)
	saver := newCharacterSaver(2, nil, zap.NewNop(), func(ctx context.Context, chars repository.UpdateCharactersParams, _ repository.UpsertInventoriesParams) error {
		for index, id := range chars.Ids {
			activeMu.Lock()
			if active[id] {
				select {
				case concurrentSameCharacter <- id:
				default:
				}
			}
			active[id] = true
			activeMu.Unlock()
			if id == 10 && chars.Xs[index] == 1 {
				close(started)
				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			activeMu.Lock()
			delete(active, id)
			activeMu.Unlock()
			completed <- completedWrite{id: id, value: chars.Xs[index]}
		}
		return nil
	})
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		saver.Stop()
	})
	require.True(t, saver.enqueueSnapshot(characterSaveTestSnapshot(10, 1)))
	saver.queueForCharacter(10).notify()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first worker did not start writing")
	}
	require.True(t, saver.enqueueSnapshot(characterSaveTestSnapshot(10, 2)))
	require.True(t, saver.enqueueSnapshot(characterSaveTestSnapshot(11, 3)))
	saver.queueForCharacter(10).notify()
	saver.queueForCharacter(11).notify()
	select {
	case write := <-completed:
		require.Equal(t, completedWrite{id: 11, value: 3}, write, "another character queue must progress while the first is blocked")
	case <-time.After(2 * time.Second):
		t.Fatal("independent character queue did not progress")
	}
	require.Empty(t, concurrentSameCharacter)
	releaseOnce.Do(func() { close(release) })
	saver.Stop()
	for _, value := range []float64{1, 2} {
		select {
		case write := <-completed:
			require.Equal(t, completedWrite{id: 10, value: value}, write)
		case <-time.After(2 * time.Second):
			t.Fatal("same-character writes did not complete in order")
		}
	}
	require.Empty(t, concurrentSameCharacter)
	require.Empty(t, completed)
	require.Nil(t, characterSavePending(t, saver, 10))
	require.Nil(t, characterSavePending(t, saver, 11))
}
