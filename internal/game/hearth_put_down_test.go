package game

import (
	"testing"

	"origin/internal/actiondefs"
	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/hearth"
	gameworld "origin/internal/game/world"
	"origin/internal/network"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type hearthPutDownFixture struct {
	*rmbLiftTest
	shard   *Shard
	runtime *hearth.Runtime
}

func newHearthPutDownFixture(t *testing.T, objectKey string) *hearthPutDownFixture {
	t.Helper()
	loadHearthBuildRecipeForTest(t)
	shard, _ := newPlayerSpawnTestShard(t, 16)
	w := shard.world
	shard.chunkManager.GetChunk(types.ChunkCoord{}).SetState(types.ChunkStateActive)
	shard.chunkManager.SetPositionObserver(core.AttachColliderSpatial(w))
	player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 100, Y: 100})
		ecs.AddComponent(w, h, components.Movement{})
		ecs.AddComponent(w, h, components.Collider{HalfWidth: 5, HalfHeight: 5, Layer: constt.PlayerLayer, Mask: constt.PlayerMask})
		ecs.AddComponent(w, h, components.EntityStats{Stamina: 1000})
	})
	def, found := objectdefs.Global().GetByKey(objectKey)
	require.True(t, found)
	object := gameworld.SpawnEntityFromDef(w, def, gameworld.DefSpawnParams{EntityID: 3, X: 100, Y: 100, Region: 1})
	ecs.AddComponent(w, object, components.ChunkRef{})
	shard.chunkManager.GetChunk(types.ChunkCoord{}).Spatial().AddStatic(object, 100, 100)
	sender := &testActionSender{}
	lift := NewLiftService(w, shard.chunkManager, nil, sender, nil)
	require.True(t, lift.startCarryingObject(w, 1, player, 3, object))
	actions, err := NewActionService(w, actiondefs.NewRegistry([]actiondefs.Definition{
		{ID: "lift_down", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetTile, Cursor: "lift_down"}},
	}), map[string]ActionHandler{"lift_down": &liftDownActionHandler{lift: lift}}, sender)
	require.NoError(t, err)
	lift.SetActionCompletion(actions.Complete)
	lift.SetActionCanCommit(actions.CanCommit)
	inbox := network.NewPlayerCommandInbox(network.CommandQueueConfig{MaxQueueSize: 20, MaxPacketsPerSecond: 20, MaxCommandsPerTickPerClient: 20})
	commands := systems.NewNetworkCommandSystem(inbox, network.NewServerJobInbox(network.CommandQueueConfig{MaxQueueSize: 20}), nil, nil, nil, nil, nil, 0, zap.NewNop())
	commands.SetActionService(actions)
	commands.SetLiftCommandService(lift)
	return &hearthPutDownFixture{shard: shard, rmbLiftTest: &rmbLiftTest{
		world: w, player: player, object: object, lift: lift, actions: actions, sender: sender, inbox: inbox, commands: commands,
	}}
}

func (f *hearthPutDownFixture) addHearth(t *testing.T, owner types.EntityID) types.Handle {
	t.Helper()
	def, found := objectdefs.Global().GetByKey("hearth_fire")
	require.True(t, found)
	handle := gameworld.SpawnEntityFromDef(f.world, def, gameworld.DefSpawnParams{EntityID: 200, X: 200, Y: 200, Region: 1})
	if owner != 0 {
		f.runtime = ecs.InitResource(f.world, hearth.NewRuntime(f.world, f.shard.chunkManager, nil, nil, nil, nil, nil, nil))
		seedHearthCollisionTestHome(t, f.runtime, owner, &hearth.Home{Object: hearth.ObjectKey{Region: 1, ID: 200}, X: 200, Y: 200})
	}
	return handle
}

func (f *hearthPutDownFixture) start(t *testing.T, route string, x, y int32) components.PendingLiftTransition {
	t.Helper()
	if route == "rmb" {
		return f.click(t, 200, x, y)
	}
	f.actions.Activate(f.world, 1, f.player, "lift_down")
	f.actions.HandleArmedClick(f.world, 1, f.player, 0, types.InvalidHandle, float64(x), float64(y))
	pending, _ := ecs.GetComponent[components.PendingLiftTransition](f.world, f.player)
	return pending
}

func (f *hearthPutDownFixture) assertCarriedRuntime(t *testing.T) {
	t.Helper()
	carry, found := ecs.GetComponent[components.LiftCarryState](f.world, f.player)
	require.True(t, found)
	require.Equal(t, f.object, carry.ObjectHandle)
	lifted, found := ecs.GetComponent[components.LiftedObjectState](f.world, f.object)
	require.True(t, found)
	require.Equal(t, f.player, lifted.CarrierHandle)
	info, _ := ecs.GetComponent[components.EntityInfo](f.world, f.object)
	require.False(t, info.IsStatic)
	position, _ := ecs.GetComponent[components.Transform](f.world, f.object)
	require.Equal(t, float64(100), position.X)
	require.Equal(t, float64(100), position.Y)
	collider, hasCollider := ecs.GetComponent[components.Collider](f.world, f.object)
	require.Equal(t, lifted.HadCollider, hasCollider)
	if hasCollider {
		require.Zero(t, collider.Layer)
		require.Zero(t, collider.Mask)
	}
}

func (f *hearthPutDownFixture) assertRejected(t *testing.T, route string) {
	t.Helper()
	f.assertCarriedRuntime(t)
	require.False(t, ecs.HasComponent[components.PendingLiftTransition](f.world, f.player))
	collider, _ := ecs.GetComponent[components.Collider](f.world, f.player)
	require.Nil(t, collider.Phantom)
	require.Len(t, f.sender.alerts, 1)
	require.Equal(t, "LIFT_PUTDOWN_INVALID", f.sender.alerts[0].ReasonCode)
	phase := "selecting"
	if route == "rmb" {
		phase = "idle"
	}
	require.Equal(t, phase, f.actions.State(f.world, f.player).Phase)
}

func TestHearthPutDownRejectsOwnerAndForeignOverlap(t *testing.T) {
	for _, route := range []string{"lift_down", "rmb"} {
		for _, key := range []string{"box", "log_x", "log_y"} {
			for _, owner := range []types.EntityID{1, 2} {
				name := "foreign"
				if owner == 1 {
					name = "owner"
				}
				t.Run(route+"/"+key+"/"+name, func(t *testing.T) {
					f := newHearthPutDownFixture(t, key)
					f.addHearth(t, owner)
					f.start(t, route, 200, 200)
					f.assertRejected(t, route)
				})
			}
		}
	}
}

func TestHearthPutDownFullFootprintAndExactContact(t *testing.T) {
	for _, object := range []struct {
		key          string
		halfW, halfH int32
	}{
		{key: "box", halfW: 4, halfH: 4},
		{key: "log_x", halfW: 12, halfH: 3},
		{key: "log_y", halfW: 3, halfH: 12},
	} {
		for _, contact := range []string{"overlap_x", "overlap_y", "edge", "corner"} {
			t.Run(object.key+"/"+contact, func(t *testing.T) {
				f := newHearthPutDownFixture(t, object.key)
				f.addHearth(t, 1)
				x, y := int32(206)+object.halfW, int32(200)
				switch contact {
				case "overlap_x":
					x--
				case "overlap_y":
					x, y = 200, 205+object.halfH
				case "corner":
					y = 206 + object.halfH
				}
				pending := f.start(t, "rmb", x, y)
				if contact == "overlap_x" || contact == "overlap_y" {
					f.assertRejected(t, "rmb")
					return
				}
				require.NotZero(t, pending.ActionGeneration)
				if object.key != "box" {
					require.False(t, pending.UsesObjectCollider)
					require.Equal(t, float64(1), pending.PhantomHalfW, "log footprint must not enlarge the approach phantom")
					require.Equal(t, float64(1), pending.PhantomHalfH)
				}
				f.lift.FinalizePendingLiftTransition(f.world, 1, f.player, pending)
				f.assertIdleCarry(t, false)
				position, _ := ecs.GetComponent[components.Transform](f.world, f.object)
				require.Equal(t, float64(x), position.X)
				require.Equal(t, float64(y), position.Y)
				require.Equal(t, object.key == "box", ecs.HasComponent[components.Collider](f.world, f.object))
			})
		}
	}
}

func TestHearthPutDownRechecksBeforeRestoringRuntime(t *testing.T) {
	for _, route := range []string{"lift_down", "rmb"} {
		for _, key := range []string{"box", "log_x", "log_y"} {
			t.Run(route+"/"+key, func(t *testing.T) {
				f := newHearthPutDownFixture(t, key)
				pending := f.start(t, route, 200, 200)
				require.NotZero(t, pending.ActionGeneration)
				f.addHearth(t, 1)
				f.lift.FinalizePendingLiftTransition(f.world, 1, f.player, pending)
				f.assertRejected(t, route)
			})
		}
	}
}

func TestHearthPutDownUsesCurrentDefinitionAtCompletion(t *testing.T) {
	f := newHearthPutDownFixture(t, "log_y")
	f.addHearth(t, 1)
	pending := f.start(t, "rmb", 216, 200)
	require.NotZero(t, pending.ActionGeneration)
	def, found := objectdefs.Global().GetByKey("log_x")
	require.True(t, found)
	ecs.WithComponent(f.world, f.object, func(info *components.EntityInfo) { info.TypeID = uint32(def.DefID) })
	f.lift.FinalizePendingLiftTransition(f.world, 1, f.player, pending)
	f.assertRejected(t, "rmb")
}

func TestHearthPutDownBlocksColliderRegardlessOfHearthState(t *testing.T) {
	for _, route := range []string{"lift_down", "rmb"} {
		for _, state := range []string{"unknown", "pending_terminal", "campfire"} {
			t.Run(route+"/"+state, func(t *testing.T) {
				f := newHearthPutDownFixture(t, "box")
				owner := types.EntityID(0)
				if state != "unknown" {
					owner = 1
				}
				handle := f.addHearth(t, owner)
				if state == "pending_terminal" {
					require.NoError(t, f.runtime.Index.MarkTerminalPending(owner))
				}
				if state == "campfire" {
					def, found := objectdefs.Global().GetByKey("campfire")
					require.True(t, found)
					require.True(t, gameworld.TransformObjectToDefInPlace(f.world, 200, handle, def, gameworld.TransformObjectInPlaceOptions{}))
					collider, found := ecs.GetComponent[components.Collider](f.world, handle)
					require.True(t, found)
					require.Equal(t, float64(5), collider.HalfWidth)
					require.Equal(t, float64(5), collider.HalfHeight)
				}
				f.start(t, route, 200, 200)
				f.assertRejected(t, route)
			})
		}
	}
}

func TestManualPutDownRejectsOrdinaryObstacleAtRequestAndCompletion(t *testing.T) {
	for _, route := range []string{"lift_down", "rmb"} {
		for _, key := range []string{"box", "log_x", "log_y"} {
			for _, stage := range []string{"request", "completion"} {
				t.Run(route+"/"+key+"/"+stage, func(t *testing.T) {
					f := newHearthPutDownFixture(t, key)
					var pending components.PendingLiftTransition
					if stage == "completion" {
						pending = f.start(t, route, 200, 200)
						require.NotZero(t, pending.ActionGeneration)
					}
					f.world.Spawn(200, func(w *ecs.World, h types.Handle) {
						ecs.AddComponent(w, h, components.Transform{X: 200, Y: 200})
						ecs.AddComponent(w, h, components.Collider{HalfWidth: 5, HalfHeight: 5, Layer: 1, Mask: 1})
					})
					if stage == "completion" {
						f.lift.FinalizePendingLiftTransition(f.world, 1, f.player, pending)
					} else {
						f.start(t, route, 200, 200)
					}
					f.assertRejected(t, route)
				})
			}
		}
	}
}

func TestManualPutDownLogFootprintDoesNotBlockOnCarrier(t *testing.T) {
	for _, route := range []string{"lift_down", "rmb"} {
		for _, object := range []struct {
			key  string
			x, y int32
		}{
			{key: "log_x", x: 106, y: 100},
			{key: "log_y", x: 100, y: 106},
		} {
			t.Run(route+"/"+object.key, func(t *testing.T) {
				f := newHearthPutDownFixture(t, object.key)
				pending := f.start(t, route, object.x, object.y)
				require.NotZero(t, pending.ActionGeneration)
				require.False(t, pending.UsesObjectCollider)
				require.Equal(t, float64(1), pending.PhantomHalfW)
				require.Equal(t, float64(1), pending.PhantomHalfH)
				ecs.AddComponent(f.world, f.player, components.CollisionResult{IsPhantom: true})
				systems.NewLiftPlacementSystem(f.world, f.lift, nil).Update(f.world, 0)
				f.assertIdleCarry(t, false)
				require.Empty(t, f.sender.alerts)
				position, found := ecs.GetComponent[components.Transform](f.world, f.object)
				require.True(t, found)
				require.Equal(t, float64(object.x), position.X)
				require.Equal(t, float64(object.y), position.Y)
				require.False(t, ecs.HasComponent[components.Collider](f.world, f.object))
			})
		}
	}
}

func TestHearthPutDownStaleCompletionPreservesNewAttempt(t *testing.T) {
	f := newHearthPutDownFixture(t, "log_x")
	first := f.start(t, "rmb", 200, 200)
	second := f.start(t, "rmb", 250, 200)
	require.Greater(t, second.ActionGeneration, first.ActionGeneration)
	f.addHearth(t, 1)
	f.lift.FinalizePendingLiftTransition(f.world, 1, f.player, first)
	current, found := ecs.GetComponent[components.PendingLiftTransition](f.world, f.player)
	require.True(t, found)
	require.Equal(t, second, current)
	collider, _ := ecs.GetComponent[components.Collider](f.world, f.player)
	require.NotNil(t, collider.Phantom)
	require.Equal(t, float64(250), collider.Phantom.WorldX)
	require.Empty(t, f.sender.alerts)
	f.assertCarriedRuntime(t)
	f.lift.FinalizePendingLiftTransition(f.world, 1, f.player, second)
	f.assertIdleCarry(t, false)
}
