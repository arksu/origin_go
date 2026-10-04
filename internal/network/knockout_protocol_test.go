package network

import (
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	netproto "origin/internal/network/proto"
	"testing"
)

func TestKnockoutProtocolRoundTrip(t *testing.T) {
	request := &netproto.ClientMessage{Sequence: 17, Payload: &netproto.ClientMessage_PlayerAction{PlayerAction: &netproto.C2S_PlayerAction{Action: &netproto.C2S_PlayerAction_StandUp{StandUp: &netproto.StandUp{StreamEpoch: 8}}}}}
	encoded, err := proto.Marshal(request)
	require.NoError(t, err)
	decoded := &netproto.ClientMessage{}
	require.NoError(t, proto.Unmarshal(encoded, decoded))
	require.True(t, proto.Equal(request, decoded))
	require.Equal(t, uint32(8), decoded.GetPlayerAction().GetStandUp().StreamEpoch)
	stats := &netproto.S2C_PlayerStats{Shp: 3, Hhp: 20, Mhp: 25, IsKnockedOut: true, KoUntilMs: 1791000060000, IsLying: true, CanStandUp: false, StreamEpoch: 8}
	response := &netproto.ServerMessage{Payload: &netproto.ServerMessage_PlayerStats{PlayerStats: stats}}
	encoded, err = proto.Marshal(response)
	require.NoError(t, err)
	received := &netproto.ServerMessage{}
	require.NoError(t, proto.Unmarshal(encoded, received))
	require.True(t, proto.Equal(response, received))
	visual := &netproto.CharacterVisualState{Generation: "0:4294967297", Revision: 9007199254740993, IsLying: true}
	spawn := &netproto.S2C_ObjectSpawn{EntityId: 1, CharacterVisual: visual, StreamEpoch: 8}
	encoded, err = proto.Marshal(spawn)
	require.NoError(t, err)
	decodedSpawn := &netproto.S2C_ObjectSpawn{}
	require.NoError(t, proto.Unmarshal(encoded, decodedSpawn))
	require.True(t, proto.Equal(spawn, decodedSpawn))
	require.EqualValues(t, 7, (&netproto.C2S_PlayerAction{}).ProtoReflect().Descriptor().Fields().ByName("stand_up").Number())
	for name, tag := range map[string]int{"ko_until_ms": 9, "is_lying": 10, "can_stand_up": 11, "stream_epoch": 12} {
		field := stats.ProtoReflect().Descriptor().Fields().ByJSONName(map[string]string{"ko_until_ms": "koUntilMs", "is_lying": "isLying", "can_stand_up": "canStandUp", "stream_epoch": "streamEpoch"}[name])
		require.NotNil(t, field)
		require.EqualValues(t, tag, field.Number())
	}
	require.EqualValues(t, 4, visual.ProtoReflect().Descriptor().Fields().ByName("is_lying").Number())
}
