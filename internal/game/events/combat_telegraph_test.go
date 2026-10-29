package events

import (
	"math"
	"testing"
	"time"

	"origin/internal/actiondefs"
	"origin/internal/combatpresentation"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func TestFoxSpawnCarriesCurrentOrInactiveCombatWarning(t *testing.T) {
	w := ecs.NewWorldWithCapacity(16, nil, 1)
	combatpresentation.Attach(w, nil)
	*ecs.GetResource[ecs.TimeState](w) = ecs.TimeState{Tick: 1, TickPeriod: 100 * time.Millisecond, UnixMs: 1000}
	h := w.Spawn(100, nil)
	ecs.AddComponent(w, h, components.NPCBrain{})
	ecs.AddComponent(w, h, components.Transform{X: 200, Y: 200})
	ecs.AddComponent(w, h, components.EntityInfo{TypeID: 42})
	ecs.AddComponent(w, h, components.Appearance{Resource: "fox"})
	d := NewNetworkVisibilityDispatcher(nil, zap.NewNop())
	spawn := d.buildObjectSpawn(w, 100, h)
	require.NotNil(t, spawn.CombatTelegraph)
	require.NotEmpty(t, spawn.CombatTelegraph.Generation)
	require.False(t, spawn.CombatTelegraph.Active)
	require.True(t, combatpresentation.Start(w, h, 7, 0, actiondefs.Sector{Range: 12, Angle: math.Pi / 3}, 4))
	ecs.GetResource[ecs.TimeState](w).Tick = 3
	spawn = d.buildObjectSpawn(w, 100, h)
	require.True(t, spawn.CombatTelegraph.Active)
	require.EqualValues(t, 2, spawn.CombatTelegraph.ElapsedTicks)
	encoded, err := proto.Marshal(&netproto.S2C_ObjectSpawnBatch{Spawns: []*netproto.S2C_ObjectSpawn{spawn}})
	require.NoError(t, err)
	var decoded netproto.S2C_ObjectSpawnBatch
	require.NoError(t, proto.Unmarshal(encoded, &decoded))
	require.True(t, proto.Equal(spawn.CombatTelegraph, decoded.Spawns[0].CombatTelegraph))
	combatpresentation.Stop(w, h, 7)
	ended := d.buildObjectSpawn(w, 100, h)
	require.False(t, ended.CombatTelegraph.Active)
	require.Greater(t, ended.CombatTelegraph.Revision, spawn.CombatTelegraph.Revision)
}
