package game

import (
	"math"
	"testing"
	"time"

	"origin/internal/characterattrs"
	"origin/internal/combatpresentation"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	gameworld "origin/internal/game/world"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func spawnProductionFox(t testing.TB, s *Shard, def *objectdefs.ObjectDef, registry *behaviors.Registry, id types.EntityID, x, y float64) types.Handle {
	t.Helper()
	h := gameworld.SpawnEntityFromDef(s.world, def, gameworld.DefSpawnParams{EntityID: id, X: x, Y: y, Region: 1, InitReason: contracts.ObjectBehaviorInitReasonSpawn, BehaviorRegistry: registry})
	require.NotEqual(t, types.InvalidHandle, h)
	coord := types.ChunkCoord{X: int(x) / constt.ChunkWorldSize, Y: int(y) / constt.ChunkWorldSize}
	ecs.AddComponent(s.world, h, components.ChunkRef{CurrentChunkX: coord.X, CurrentChunkY: coord.Y})
	s.chunkManager.GetChunkFast(coord).Spatial().AddDynamic(h, int(x), int(y))
	return h
}

func spawnProductionCombatPlayer(t testing.TB, s *Shard, id types.EntityID, x, y, hp float64) types.Handle {
	t.Helper()
	w := s.world
	h := w.Spawn(id, nil)
	require.NotEqual(t, types.InvalidHandle, h)
	attributes := characterattrs.Default()
	attributes[characterattrs.CON] = int(math.Ceil(math.Pow(hp/25, 2)))
	ecs.AddComponent(w, h, components.CharacterProfile{Attributes: attributes})
	ecs.AddComponent(w, h, components.EntityStats{Stamina: 1000, Energy: 900})
	ecs.AddComponent(w, h, components.Transform{X: x, Y: y, Direction: math.Pi})
	ecs.AddComponent(w, h, components.Movement{Mode: constt.Walk, Speed: 32})
	ecs.AddComponent(w, h, components.Collider{HalfWidth: 5, HalfHeight: 5, Layer: 1, Mask: 1})
	ecs.AddComponent(w, h, components.EntityHealth{SHP: hp, HHP: hp})
	ecs.AddComponent(w, h, components.EntityInfo{TypeID: 1, Region: 1, Layer: w.Layer})
	coord := types.ChunkCoord{X: int(x) / constt.ChunkWorldSize, Y: int(y) / constt.ChunkWorldSize}
	ecs.AddComponent(w, h, components.ChunkRef{CurrentChunkX: coord.X, CurrentChunkY: coord.Y})
	s.chunkManager.GetChunkFast(coord).Spatial().AddDynamic(h, int(x), int(y))
	require.NoError(t, s.creatureDamage.PrepareTarget(h))
	return h
}

func equipProductionCombatItems(t testing.TB, s *Shard, owner types.Handle, armor bool) {
	t.Helper()
	id, ok := s.world.GetExternalID(owner)
	require.True(t, ok)
	axe, ok := itemdefs.Global().GetByKey("stone_axe")
	require.True(t, ok)
	items := []components.InvItem{{ItemID: 9000, TypeID: uint32(axe.DefID), Quality: 10, EquipSlot: netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND}}
	if armor {
		shirt, ok := itemdefs.Global().GetByKey("nettle_shirt")
		require.True(t, ok)
		items = append(items, components.InvItem{ItemID: 9001, TypeID: uint32(shirt.DefID), Quality: 10, EquipSlot: netproto.EquipSlot_EQUIP_SLOT_CHEST})
	}
	container := s.world.Spawn(9999, nil)
	ecs.AddComponent(s.world, container, components.InventoryContainer{Kind: constt.InventoryEquipment, OwnerID: id, Items: items})
	ecs.GetResource[ecs.InventoryRefIndex](s.world).Add(constt.InventoryEquipment, id, 0, container)
}

func TestNPCFoxDefensiveBiteThroughProductionShard(t *testing.T) {
	for _, scenario := range []string{"hit", "armor", "miss", "knockout", "low_hp", "removed_target", "corpse_target"} {
		t.Run(scenario, func(t *testing.T) {
			s, def, registry := newNPCProductionShardSize(t, 64, 1, func() {
				if scenario == "armor" {
					shirt, ok := itemdefs.Global().GetByKey("nettle_shirt")
					require.True(t, ok)
					shirt.Armor = &itemdefs.ArmorDef{BaseArmor: 4}
				}
			})
			ts := ecs.TimeState{Tick: 1, TickRate: 10, TickPeriod: 100 * time.Millisecond, Delta: .1, Now: time.Unix(100, 0), UnixMs: 100000}
			*ecs.GetResource[ecs.TimeState](s.world) = ts
			fox := spawnProductionFox(t, s, def, registry, 100, 200, 200)
			player := spawnProductionCombatPlayer(t, s, 200, 210, 200, 50)
			s.chunkManager.RegisterEntity(200, 210, 200, false)
			equipProductionCombatItems(t, s, player, scenario == "armor")
			step := func() { s.Update(ts); ts.Tick++; ts.Now = ts.Now.Add(ts.TickPeriod); ts.UnixMs = ts.Now.UnixMilli() }
			s.actionService.ActivateRequest(s.world, 200, player, &netproto.C2S_ActivateAction{ActionId: "axe_strike", StreamEpoch: 1})
			require.True(t, ecs.HasComponent[components.ActiveGameAction](s.world, player))
			var started uint64
			for range 20 {
				step()
				if state := combatpresentation.Snapshot(s.world, fox); state != nil && state.Active {
					started = ecs.GetResource[ecs.TimeState](s.world).Tick
					break
				}
			}
			require.NotZero(t, started, "real axe damage must wake fox and start shared bite")
			brain, _ := ecs.GetComponent[components.NPCBrain](s.world, fox)
			require.Equal(t, contracts.EntityRef{Handle: player, EntityID: 200}, brain.DamageSource)
			hp, _ := ecs.GetComponent[components.ObjectInternalState](s.world, fox)
			require.Less(t, hp.HP, 110.0)
			require.False(t, ecs.HasComponent[components.CharacterProfile](s.world, fox))
			require.False(t, ecs.HasComponent[components.EntityHealth](s.world, fox))
			switch scenario {
			case "miss":
				ecs.WithComponent(s.world, player, func(tr *components.Transform) { tr.X = 230 })
				s.chunkManager.GetChunkFast(types.ChunkCoord{}).Spatial().UpdateDynamic(player, 210, 200, 230, 200)
			case "knockout":
				ecs.WithComponent(s.world, player, func(h *components.EntityHealth) { h.SHP = 0; h.IsLying = true; h.KOUntilUnixMs = 110000 })
			case "low_hp":
				require.NoError(t, gameworld.SetObjectHP(s.world, fox, 20))
			case "removed_target":
				s.world.Despawn(player)
			case "corpse_target":
				// Permanent death keeps the entity identity while removing its
				// live-player components. It must not leave a remembered threat.
				ecs.RemoveComponent[components.EntityHealth](s.world, player)
				ecs.RemoveComponent[components.CharacterProfile](s.world, player)
			}
			for ts.Tick <= started+4 {
				step()
			}
			if scenario != "removed_target" && scenario != "corpse_target" {
				health, _ := ecs.GetComponent[components.EntityHealth](s.world, player)
				expected := 50.0
				if scenario == "hit" {
					expected = 44
				}
				if scenario == "armor" {
					expected = 50 - 36.0/10
				}
				if scenario == "knockout" {
					expected = 0
				}
				require.InDelta(t, expected, health.SHP, 1e-10)
			}
			require.False(t, combatpresentation.Snapshot(s.world, fox).Active)
			cooldowns, _ := ecs.GetComponent[components.ActionCooldowns](s.world, fox)
			if scenario == "hit" || scenario == "armor" || scenario == "miss" {
				require.Greater(t, cooldowns.ByAction["fox_bite"].ExpiresAtMs, ts.UnixMs)
			} else {
				require.Empty(t, cooldowns.ByAction)
			}
		})
	}
}
