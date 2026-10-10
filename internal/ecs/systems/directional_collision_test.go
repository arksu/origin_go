package systems

import (
	"context"
	"math"
	"testing"
	"time"

	"go.uber.org/zap"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

func TestDirectionCommandDrainChargesOneStepAndNeverEmitsPointArrival(t *testing.T) {
	scene := newSweepScene(t, 100, 100, nil)
	w := scene.world
	ecs.AddComponent(w, scene.mover, components.Movement{Speed: 32, Mode: constt.Run})
	ecs.AddComponent(w, scene.mover, components.EntityStats{Stamina: 500, Energy: 1000})
	bus := newTestEventBus(t)
	defer shutdownTestEventBus(t, bus)
	arrivals := 0
	bus.SubscribeSync(ecs.TopicGameplayPointMovementStopped, eventbus.PriorityMedium, func(context.Context, eventbus.Event) error {
		arrivals++
		return nil
	})
	inbox := network.NewPlayerCommandInbox(network.CommandQueueConfig{MaxQueueSize: 20, MaxPacketsPerSecond: 40, MaxCommandsPerTickPerClient: 20})
	commands := NewNetworkCommandSystem(inbox, network.NewServerJobInbox(network.CommandQueueConfig{MaxQueueSize: 20}), nil, nil, nil, nil, nil, 0, zap.NewNop())
	commands.SetDirectionalSessionValidator(func(types.EntityID, uint64, uint32) bool { return true })
	cm := &testChunkManager{chunk: scene.chunk}
	mover := NewMovementSystem(w, cm, zap.NewNop())
	apply := NewTransformUpdateSystem(w, cm, bus, zap.NewNop())
	ecs.GetResource[ecs.VisibilityState](w).ObserversByVisibleTarget[scene.mover] = map[types.Handle]struct{}{scene.mover: {}}
	for index, vector := range [][2]float32{{1, 0}, {0, 0}, {0, 1}} {
		if err := inbox.Enqueue(&network.PlayerCommand{ClientID: 1, CharacterID: 1, CommandID: uint64(index + 1), CommandType: network.CmdMoveDirection,
			Payload: &netproto.MoveDirection{X: vector[0], Y: vector[1], InputRevision: uint32(index + 1), StreamEpoch: 1}}); err != nil {
			t.Fatal(err)
		}
	}
	commands.Update(w, .1)
	NewResetSystem(zap.NewNop()).Update(w, .1)
	mover.Update(w, .1)
	scene.system.Update(w, .1)
	apply.Update(w, .1)
	position, _ := ecs.GetComponent[components.Transform](w, scene.mover)
	stats, _ := ecs.GetComponent[components.EntityStats](w, scene.mover)
	if position.X != 100 || math.Abs(position.Y-104.8) > 1e-9 || math.Abs(stats.Stamina-499.98) > 1e-9 || ecs.GetResource[ecs.MovedEntities](w).Count != 1 || len(apply.moveBatch) != 1 || arrivals != 0 {
		t.Fatalf("drain produced extra step, cost or arrival: %+v %+v arrivals=%d", position, stats, arrivals)
	}
	entry := apply.moveBatch[0]
	if entry.TargetX != nil || entry.TargetY != nil || entry.MoveSeq != 0 || !entry.IsMoving {
		t.Fatalf("bad start batch: %+v", entry)
	}
	ecs.WithComponent(w, scene.mover, func(m *components.Movement) { m.ReleaseDirection() })
	NewResetSystem(zap.NewNop()).Update(w, .1)
	mover.Update(w, .1)
	scene.system.Update(w, .1)
	apply.Update(w, .1)
	if len(apply.moveBatch) != 1 || apply.moveBatch[0].MoveSeq != 1 || apply.moveBatch[0].IsMoving || arrivals != 0 {
		t.Fatalf("bad release batch: %+v arrivals=%d", apply.moveBatch, arrivals)
	}
}

func TestDirectionBlockedProbeExpiryAndBroadcast(t *testing.T) {
	for _, expired := range []bool{false, true} {
		name := "resume"
		if expired {
			name = "expire"
		}
		t.Run(name, func(t *testing.T) {
			scene := newSweepScene(t, 100, 100, []components.Transform{{X: 112, Y: 100}})
			w := scene.world
			movement := components.Movement{Speed: 32, Mode: constt.Walk}
			movement.SetDirection(1, 0, 1, time.Unix(101, 0))
			ecs.AddComponent(w, scene.mover, movement)
			ecs.AddComponent(w, scene.mover, components.EntityStats{Stamina: 500, Energy: 1000})
			cm := &testChunkManager{chunk: scene.chunk}
			bus := newTestEventBus(t)
			defer shutdownTestEventBus(t, bus)
			mover := NewMovementSystem(w, cm, zap.NewNop())
			apply := NewTransformUpdateSystem(w, cm, bus, zap.NewNop())
			ecs.GetResource[ecs.VisibilityState](w).ObserversByVisibleTarget[scene.mover] = map[types.Handle]struct{}{scene.mover: {}}
			step := func(now time.Time) {
				ecs.GetResource[ecs.TimeState](w).Now = now
				ecs.GetResource[ecs.MovedEntities](w).Count = 0
				mover.Update(w, .1)
				scene.system.Update(w, .1)
				apply.Update(w, .1)
			}
			step(time.Unix(100, 0))
			step(time.Unix(100, 100000000))
			current, _ := ecs.GetComponent[components.Movement](w, scene.mover)
			position, _ := ecs.GetComponent[components.Transform](w, scene.mover)
			if current.State != constt.StateIdle || current.TargetType != constt.TargetDirection || current.VelocityX != 0 || current.VelocityY != 0 || position.X > 102 || len(apply.moveBatch) != 1 || apply.moveBatch[0].IsMoving || apply.moveBatch[0].TargetX != nil {
				t.Fatalf("bad blocked state: %+v, position %+v, batch %+v", current, position, apply.moveBatch)
			}
			stamina, _ := ecs.GetComponent[components.EntityStats](w, scene.mover)
			step(time.Unix(100, 200000000))
			after, _ := ecs.GetComponent[components.EntityStats](w, scene.mover)
			if len(apply.moveBatch) != 0 || after.Stamina != stamina.Stamina {
				t.Fatal("repeated block broadcast or stamina charge")
			}
			// Mode changes while blocked still have to reach observers.
			ecs.WithComponent(w, scene.mover, func(m *components.Movement) { m.Mode = constt.Crawl })
			step(time.Unix(100, 300000000))
			if len(apply.moveBatch) != 1 || apply.moveBatch[0].MoveMode != constt.Crawl {
				t.Fatal("blocked mode change lost")
			}
			if expired {
				step(time.Unix(101, 0))
				position, _ = ecs.GetComponent[components.Transform](w, scene.mover)
			}
			w.Despawn(w.GetHandleByEntityID(2))
			now := time.Unix(100, 400000000)
			if expired {
				now = time.Unix(101, 100000000)
			}
			step(now)
			next, _ := ecs.GetComponent[components.Transform](w, scene.mover)
			current, _ = ecs.GetComponent[components.Movement](w, scene.mover)
			if expired {
				if next.X != position.X || current.TargetType != constt.TargetNone {
					t.Fatalf("expired hold changed: before=%+v after=%+v movement=%+v", position, next, current)
				}
			} else if next.X <= position.X || current.State != constt.StateMoving || len(apply.moveBatch) != 1 || !apply.moveBatch[0].IsMoving {
				t.Fatal("valid hold did not resume")
			}
		})
	}
}

func TestDirectionCollisionSlideUsesResolvedVelocityAndBudget(t *testing.T) {
	scene := newSweepScene(t, 100, 100, []components.Transform{{X: 112, Y: 100}})
	w := scene.world
	movement := components.Movement{Speed: 32, Mode: constt.Run}
	movement.SetDirection(1/math.Sqrt(2), 1/math.Sqrt(2), 1, time.Unix(101, 0))
	ecs.AddComponent(w, scene.mover, movement)
	cm := &testChunkManager{chunk: scene.chunk}
	bus := newTestEventBus(t)
	defer shutdownTestEventBus(t, bus)
	mover := NewMovementSystem(w, cm, zap.NewNop())
	apply := NewTransformUpdateSystem(w, cm, bus, zap.NewNop())
	ecs.GetResource[ecs.TimeState](w).Now = time.Unix(100, 0)
	mover.Update(w, .1)
	scene.system.Update(w, .1)
	apply.Update(w, .1)
	position, _ := ecs.GetComponent[components.Transform](w, scene.mover)
	current, _ := ecs.GetComponent[components.Movement](w, scene.mover)
	dx, dy := position.X-100, position.Y-100
	if position.X > 102 || dy <= 0 || math.Hypot(dx, dy) > 4.8+1e-9 || math.Abs(current.VelocityX-dx/.1) > 1e-9 || math.Abs(current.VelocityY-dy/.1) > 1e-9 || math.Abs(position.Direction-math.Atan2(dy, dx)) > 1e-9 {
		t.Fatalf("invalid slide: %+v %+v", position, current)
	}
}

func TestDirectionWorldRestrictionsAndTurnAway(t *testing.T) {
	for _, obstacle := range []string{"wall", "corner", "water", "phantom", "world-bound"} {
		t.Run(obstacle, func(t *testing.T) {
			scene := newSweepScene(t, 100, 100, nil)
			if obstacle == "wall" || obstacle == "corner" {
				scene.addStaticCandidate(2, 112, 100, 5, 60)
			}
			if obstacle == "corner" {
				scene.addStaticCandidate(3, 100, 112, 60, 5)
			}
			if obstacle == "water" {
				paintTestTile(scene.chunk, 110, 100, types.TileDeepWater)
				scene.chunk.RestoreTiles(scene.chunk.Tiles, 0, 0)
			}
			if obstacle == "phantom" {
				ecs.WithComponent(scene.world, scene.mover, func(c *components.Collider) {
					c.Phantom = &components.PhantomCollider{WorldX: 112, WorldY: 100, HalfWidth: 5, HalfHeight: 60}
				})
			}
			if obstacle == "world-bound" {
				scene.system.worldMaxX = 102
			}
			w := scene.world
			m := components.Movement{Speed: 32, Mode: constt.Walk}
			m.SetDirection(1, 0, 1, time.Unix(102, 0))
			ecs.AddComponent(w, scene.mover, m)
			cm := &testChunkManager{chunk: scene.chunk}
			bus := newTestEventBus(t)
			defer shutdownTestEventBus(t, bus)
			mover := NewMovementSystem(w, cm, zap.NewNop())
			apply := NewTransformUpdateSystem(w, cm, bus, zap.NewNop())
			step := func() {
				ecs.GetResource[ecs.MovedEntities](w).Count = 0
				ecs.GetResource[ecs.TimeState](w).Now = time.Unix(100, 0)
				mover.Update(w, .1)
				scene.system.Update(w, .1)
				apply.Update(w, .1)
			}
			for tick := 0; tick < 5; tick++ {
				step()
			}
			position, _ := ecs.GetComponent[components.Transform](w, scene.mover)
			current, _ := ecs.GetComponent[components.Movement](w, scene.mover)
			if position.X > 103.01 || current.TargetType != constt.TargetDirection || current.State != constt.StateIdle {
				t.Fatalf("crossed %s: %+v %+v", obstacle, position, current)
			}
			ecs.WithComponent(w, scene.mover, func(m *components.Movement) { m.SetDirection(-1, 0, 2, time.Unix(102, 0)) })
			step()
			next, _ := ecs.GetComponent[components.Transform](w, scene.mover)
			if next.X >= position.X {
				t.Fatalf("cannot move away from %s", obstacle)
			}
		})
	}
}

func TestDirectionCrossesChunkBoundaryWithResolvedPosition(t *testing.T) {
	cm := newMigrationChunkManager(types.ChunkCoord{X: 0, Y: 0}, types.ChunkCoord{X: 1, Y: 0})
	scene := newMigrationScene(t, cm, float64(constt.ChunkWorldSize)-1, 100, types.ChunkCoord{})
	movement := components.Movement{Speed: 32, Mode: constt.Walk}
	movement.SetDirection(1, 0, 1, time.Time{}.Add(time.Second))
	ecs.AddComponent(scene.world, scene.mover, movement)
	mover := NewMovementSystem(scene.world, cm, zap.NewNop())
	collision := NewCollisionSystem(scene.world, cm, zap.NewNop(), 0, float64(2*constt.ChunkWorldSize), 0, constt.ChunkWorldSize, 0)
	mover.Update(scene.world, .1)
	collision.Update(scene.world, .1)
	scene.runTransformAndChunk()
	ref := chunkRefOf(t, scene.world, scene.mover)
	if ref.CurrentChunkX != 1 || cm.chunk(types.ChunkCoord{}).Spatial().DynamicCount() != 0 || cm.chunk(types.ChunkCoord{X: 1, Y: 0}).Spatial().DynamicCount() != 1 {
		t.Fatalf("invalid directional migration: %+v", ref)
	}
}
