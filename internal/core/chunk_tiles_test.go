package core

import (
	"context"
	"errors"
	"sync"
	"testing"

	"origin/internal/persistence/repository"
	"origin/internal/types"
)

func TestTileMutationAndRestoreClearStaleBits(t *testing.T) {
	chunk := NewChunk(types.ChunkCoord{}, 0, 0, 2)
	if chunk.SnapshotTiles().Version != 0 {
		t.Fatal("new chunk version must start at zero")
	}
	tiles := []byte{types.TileGrass, types.TileShallowWater, types.TileDeepWater, types.TileVoid}
	if err := chunk.RestoreTiles(tiles, 123, 7); err != nil {
		t.Fatal(err)
	}
	tiles[0] = types.TileVoid
	if !chunk.IsTilePassable(0, 0, 2) || chunk.SnapshotTiles().Version != 7 || chunk.TilesDirty() {
		t.Fatal("restore changed version, aliased input or dirtied tiles")
	}
	if !chunk.SetTile(1, 0, types.TileVoid) || chunk.IsTilePassable(1, 0, 2) || chunk.IsTileSwimmable(1, 0, 2) {
		t.Fatal("replacement left stale tile bits")
	}
	snapshot := chunk.SnapshotTiles()
	if snapshot.Version != 8 || snapshot.Tiles[0] != types.TileGrass || snapshot.Tiles[2] != types.TileDeepWater || !chunk.TilesDirty() {
		t.Fatalf("mutation affected neighbors/version: %#v", snapshot)
	}
	for _, coords := range [][2]int{{-1, 0}, {0, -1}, {2, 0}, {0, 2}} {
		if chunk.SetTile(coords[0], coords[1], types.TileGrass) {
			t.Fatal("out-of-bounds mutation accepted")
		}
	}
	if chunk.SetTile(1, 0, types.TileVoid) || chunk.SnapshotTiles().Version != 8 {
		t.Fatal("no-op advanced version")
	}
	if err := chunk.RestoreTiles([]byte{types.TileVoid, types.TileGrass, types.TileVoid, types.TileShallowWater}, 200, 12); err != nil {
		t.Fatal(err)
	}
	if chunk.IsTilePassable(0, 0, 2) || chunk.IsTileSwimmable(0, 1, 2) || !chunk.IsTilePassable(1, 1, 2) || !chunk.IsTileSwimmable(1, 1, 2) {
		t.Fatal("restore left stale bitsets")
	}
	if snapshot.Version != 8 || snapshot.Tiles[3] != types.TileVoid {
		t.Fatal("snapshot was mutated by restore")
	}
	if err := chunk.RestoreTiles([]byte{1}, 0, 0); err == nil {
		t.Fatal("malformed restore accepted")
	}
}

type tileWriterFunc func(context.Context, repository.UpsertChunkParams) (int64, error)

func (write tileWriterFunc) UpsertChunk(ctx context.Context, snapshot repository.UpsertChunkParams) (int64, error) {
	return write(ctx, snapshot)
}

func TestSaveKeepsAnOverlappingEditDirty(t *testing.T) {
	chunk := NewChunk(types.ChunkCoord{X: 2, Y: 3}, 1, 0, 2)
	if err := chunk.RestoreTiles([]byte{35, 35, 35, 35}, 456, 10); err != nil {
		t.Fatal(err)
	}
	chunk.SetTile(0, 0, types.TilePlowed)
	writes := 0
	writer := tileWriterFunc(func(_ context.Context, snapshot repository.UpsertChunkParams) (int64, error) {
		writes++
		if snapshot.LastTick != 456 || snapshot.Version != 10+writes || snapshot.TilesData[0] != types.TilePlowed {
			t.Fatalf("mismatched save snapshot: %#v", snapshot)
		}
		if writes == 1 {
			chunk.SetTile(1, 0, types.TilePlowed)
			if snapshot.TilesData[1] != types.TileGrass {
				t.Fatal("in-flight bytes were modified")
			}
		} else if snapshot.TilesData[1] != types.TilePlowed {
			t.Fatal("retry lost edit")
		}
		return 1, nil
	})
	if err := chunk.saveTiles(context.Background(), writer, 0); err != nil {
		t.Fatal(err)
	}
	if !chunk.TilesDirty() {
		t.Fatal("old save cleared new edit")
	}
	if err := chunk.saveTiles(context.Background(), writer, 0); err != nil {
		t.Fatal(err)
	}
	if chunk.TilesDirty() || writes != 2 {
		t.Fatal("current version was not saved")
	}
	for _, rejected := range []tileWriterFunc{
		func(context.Context, repository.UpsertChunkParams) (int64, error) { return 0, nil },
		func(context.Context, repository.UpsertChunkParams) (int64, error) {
			return 0, errors.New("database offline")
		},
	} {
		chunk.SetTile(0, 0, types.TileVoid)
		if err := chunk.saveTiles(context.Background(), rejected, 0); err == nil || !chunk.TilesDirty() {
			t.Fatal("failed/superseded save cleared dirty tiles")
		}
	}
}

func TestSnapshotTilesIsAtomicUnderConcurrentEdits(t *testing.T) {
	chunk := NewChunk(types.ChunkCoord{}, 0, 0, 1)
	if err := chunk.RestoreTiles([]byte{types.TileGrass}, 0, 0); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		for i := 0; i < 2000; i++ {
			tile := byte(types.TilePlowed)
			if i%2 == 1 {
				tile = types.TileGrass
			}
			chunk.SetTile(0, 0, tile)
		}
	}()
	for i := 0; i < 2000; i++ {
		snapshot := chunk.SnapshotTiles()
		want := byte(types.TileGrass)
		if snapshot.Version%2 == 1 {
			want = types.TilePlowed
		}
		if snapshot.Tiles[0] != want {
			t.Fatalf("version/bytes mismatch: %#v", snapshot)
		}
	}
	workers.Wait()
}
