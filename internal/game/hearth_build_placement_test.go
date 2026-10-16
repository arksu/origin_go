package game

import (
	"fmt"
	"slices"
	"testing"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

type hearthBuildPlacementFixture struct {
	shard   *Shard
	service *BuildService
	player  types.Handle
	sender  *lockedBuildSender
}

func newHearthBuildPlacementFixture(t *testing.T) *hearthBuildPlacementFixture {
	t.Helper()
	_, result := loadHearthBuildRecipeForTest(t)
	require.Equal(t, float64(constt.CoordPerTile), result.Components.Collider.W)
	require.Equal(t, float64(constt.CoordPerTile), result.Components.Collider.H)
	shard, _ := newPlayerSpawnTestShard(t, 4)
	shard.chunkManager.GetChunk(types.ChunkCoord{}).SetState(types.ChunkStateActive)
	shard.chunkManager.SetPositionObserver(core.AttachColliderSpatial(shard.world))
	player := shard.world.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{State: constt.StateIdle})
		ecs.AddComponent(w, h, components.Collider{HalfWidth: 5, HalfHeight: 5, Layer: constt.PlayerLayer, Mask: constt.PlayerMask})
	})
	sender := &lockedBuildSender{}
	return &hearthBuildPlacementFixture{
		shard: shard, player: player, sender: sender,
		service: &BuildService{world: shard.world, chunkManager: shard.chunkManager, alerts: sender},
	}
}

func (f *hearthBuildPlacementFixture) start(t *testing.T, x, y int, rejection string) {
	t.Helper()
	w := f.shard.world
	f.service.CancelPendingBuildPlacement(w, 1, f.player)
	f.sender.alerts = nil
	f.service.HandleStartBuild(w, 1, f.player, &netproto.C2S_BuildStart{
		BuildKey: "hearth_fire", Pos: &netproto.Vector2{X: int32(x), Y: int32(y)},
	})
	pending, found := ecs.GetComponent[components.PendingBuildPlacement](w, f.player)
	collider, hasCollider := ecs.GetComponent[components.Collider](w, f.player)
	require.True(t, hasCollider)
	if rejection != "" {
		require.False(t, found, "rejected placement must not create pending state")
		require.Nil(t, collider.Phantom, "rejected placement must not create a phantom")
		require.Len(t, f.sender.alerts, 1)
		require.Equal(t, rejection, f.sender.alerts[0].ReasonCode)
		return
	}
	require.Empty(t, f.sender.alerts)
	require.True(t, found)
	require.Equal(t, "hearth_fire", pending.BuildKey)
	require.Equal(t, x, pending.TargetX)
	require.Equal(t, y, pending.TargetY)
	require.NotNil(t, collider.Phantom)
	require.Equal(t, float64(x), collider.Phantom.WorldX)
	require.Equal(t, float64(y), collider.Phantom.WorldY)
}

func TestHearthBuildPlacementKnownTiles(t *testing.T) {
	f := newHearthBuildPlacementFixture(t)
	blocked := []int{types.TileDeepWater, types.TileShallowWater, types.TileSwamp1, types.TileSwamp2, types.TileSwamp3, types.TileVoid}
	chunk := f.shard.chunkManager.GetChunk(types.ChunkCoord{})
	for tileID := 0; tileID <= 255; tileID++ {
		if !types.IsKnownTileID(tileID) {
			continue
		}
		t.Run(fmt.Sprintf("tile_%d", tileID), func(t *testing.T) {
			chunk.SetTile(16, 16, byte(tileID))
			rejection := ""
			if slices.Contains(blocked, tileID) {
				rejection = "BUILD_TILE_BLOCKED"
			}
			f.start(t, 198, 198, rejection)
		})
	}
}

func TestHearthBuildPlacementFootprint(t *testing.T) {
	for _, tc := range []struct {
		name         string
		x, y         int
		tileX, tileY int
		rejection    string
	}{
		{name: "left_overlap", x: 193, y: 198, tileX: 15, tileY: 16, rejection: "BUILD_TILE_BLOCKED"},
		{name: "right_overlap", x: 203, y: 198, tileX: 17, tileY: 16, rejection: "BUILD_TILE_BLOCKED"},
		{name: "top_overlap", x: 198, y: 193, tileX: 16, tileY: 15, rejection: "BUILD_TILE_BLOCKED"},
		{name: "bottom_overlap", x: 198, y: 203, tileX: 16, tileY: 17, rejection: "BUILD_TILE_BLOCKED"},
		{name: "corner_overlap", x: 203, y: 203, tileX: 17, tileY: 17, rejection: "BUILD_TILE_BLOCKED"},
		{name: "exact_right_boundary", x: 198, y: 198, tileX: 17, tileY: 16},
		{name: "exact_bottom_boundary", x: 198, y: 198, tileX: 16, tileY: 17},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newHearthBuildPlacementFixture(t)
			chunk := f.shard.chunkManager.GetChunk(types.ChunkCoord{})
			require.True(t, chunk.SetTile(tc.tileX, tc.tileY, types.TileDeepWater))
			anchorTile, found := f.shard.chunkManager.GetTileID(tc.x/constt.CoordPerTile, tc.y/constt.CoordPerTile)
			require.True(t, found)
			require.Equal(t, byte(types.TileGrass), anchorTile)
			f.start(t, tc.x, tc.y, tc.rejection)
		})
	}
}

func TestHearthBuildPlacementUnavailableChunk(t *testing.T) {
	t.Run("inactive", func(t *testing.T) {
		f := newHearthBuildPlacementFixture(t)
		f.shard.chunkManager.GetChunk(types.ChunkCoord{}).SetState(types.ChunkStateInactive)
		f.start(t, 198, 198, "BUILD_OUTSIDE_LOADED_CHUNK")
	})
	t.Run("missing_neighbor", func(t *testing.T) {
		f := newHearthBuildPlacementFixture(t)
		f.shard.cfg.Game.WorldWidthChunks = 2
		require.Nil(t, f.shard.chunkManager.GetChunk(types.ChunkCoord{X: 1}))
		x := constt.ChunkWorldSize - constt.CoordPerTile/2
		f.start(t, x, 198, "")
		f.start(t, x+1, 198, "BUILD_OUTSIDE_LOADED_CHUNK")
	})
}
