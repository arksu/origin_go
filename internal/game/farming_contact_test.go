package game

import (
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/behaviors/contracts"
	gameworld "origin/internal/game/world"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestPassableCropHarvestUsesProductionContactAndContextCycle(t *testing.T) {
	for _, touching := range []bool{true, false} {
		t.Run(map[bool]string{true: "touching", false: "too_far"}[touching], func(t *testing.T) {
			s, _, registry := newNPCProductionShard(t, 64)
			w := s.world
			chunk := s.chunkManager.GetChunkFast(types.ChunkCoord{})
			def, found := objectdefs.Global().GetByKey("crop_carrot")
			require.True(t, found)
			ts := ecs.TimeState{Tick: 1, TickRate: 10, TickPeriod: 100 * time.Millisecond, Delta: .1, Now: time.Unix(100, 0), UnixMs: 100000}
			*ecs.GetResource[ecs.TimeState](w) = ts
			crop := gameworld.SpawnEntityFromDef(w, def, gameworld.DefSpawnParams{
				EntityID: 200, X: 200, Y: 200, Region: 1,
				InitReason: contracts.ObjectBehaviorInitReasonSpawn, BehaviorRegistry: registry,
			})
			require.True(t, w.Alive(crop))
			ecs.AddComponent(w, crop, components.ChunkRef{})
			ecs.WithComponent(w, crop, func(state *components.ObjectInternalState) {
				components.SetBehaviorState(state, "crop", &components.CropBehaviorState{Stage: 3})
			})
			chunk.Spatial().AddStatic(crop, 200, 200)
			x := 196.0
			if !touching {
				x = 180
			}
			player := w.Spawn(100, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.Transform{X: x, Y: 200})
				ecs.AddComponent(w, h, components.Collider{HalfWidth: 2, HalfHeight: 2, Layer: 1, Mask: 1})
				ecs.AddComponent(w, h, components.CollisionResult{})
				ecs.AddComponent(w, h, components.Movement{Mode: constt.Walk, Speed: .01})
				ecs.AddComponent(w, h, components.EntityStats{Stamina: 100})
				ecs.AddComponent(w, h, components.ChunkRef{})
			})
			chunk.Spatial().AddDynamic(player, int(x), 200)
			completed := 0
			require.Equal(t, touching, s.additionalObjectContact(100, crop))
			s.contextActions.SetCropHarvest(func(_ *ecs.World, playerID types.EntityID, actor types.Handle, targetID types.EntityID, target types.Handle) contracts.BehaviorResult {
				require.Equal(t, types.EntityID(100), playerID)
				require.Equal(t, player, actor)
				require.Equal(t, types.EntityID(200), targetID)
				require.Equal(t, crop, target)
				completed++
				return contracts.BehaviorResult{OK: true}
			})
			commands := systems.NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, nil, 0, nil)
			commands.SetContextActionService(s.contextActions)
			commands.StartPendingContextActionFromServer(w, player, 100, 200, crop, "harvest")
			for i := 0; i < 12; i++ {
				s.Update(ts)
				if i == 0 && touching {
					_, linked := ecs.GetResource[ecs.LinkState](w).GetLink(100)
					require.True(t, linked, "pass-through contact must establish the link without a physical collision")
				}
				ts.Tick++
				ts.Now = ts.Now.Add(ts.TickPeriod)
				ts.UnixMs = ts.Now.UnixMilli()
			}
			collision, _ := ecs.GetComponent[components.CollisionResult](w, player)
			require.Zero(t, collision.CollidedWith, "crop remains physically passable")
			require.Equal(t, map[bool]int{true: 1, false: 0}[touching], completed)
			_, linked := ecs.GetResource[ecs.LinkState](w).GetLink(100)
			require.Equal(t, touching, linked)
			_, cycling := ecs.GetComponent[components.ActiveCyclicAction](w, player)
			require.False(t, cycling)
		})
	}
}
