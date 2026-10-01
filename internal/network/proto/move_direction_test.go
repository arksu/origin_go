package proto

import (
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestMoveDirectionRoundTrip(t *testing.T) {
	for _, direction := range []*MoveDirection{
		{X: -0.31622776, Y: -0.9486833, InputRevision: 1, StreamEpoch: 7},
		{X: 1, InputRevision: 2, StreamEpoch: 7},
		{InputRevision: 3, StreamEpoch: 7},
		{Y: -1, InputRevision: ^uint32(0), StreamEpoch: ^uint32(0)},
	} {
		want := &ClientMessage{Sequence: 42, Payload: &ClientMessage_PlayerAction{PlayerAction: &C2S_PlayerAction{
			Action: &C2S_PlayerAction_MoveDirection{MoveDirection: direction},
		}}}
		wire, err := proto.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		var got ClientMessage
		if err := proto.Unmarshal(wire, &got); err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(want, &got) {
			t.Fatalf("round trip: %v", &got)
		}
	}
}

func TestDirectionalMovementCapabilityCompatibility(t *testing.T) {
	var snapshot S2C_PlayerEnterWorld
	// Pre-capability snapshot: entity_id=42, stream_epoch=7.
	if err := proto.Unmarshal([]byte{8, 42, 72, 7}, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.DirectionalMovementSupported || snapshot.EntityId != 42 || snapshot.StreamEpoch != 7 {
		t.Fatalf("legacy snapshot changed: %v", &snapshot)
	}
	snapshot.DirectionalMovementSupported = true
	wire, err := proto.Marshal(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var decoded S2C_PlayerEnterWorld
	if err := proto.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(&snapshot, &decoded) {
		t.Fatalf("capability lost: %v", &decoded)
	}
	if snapshot.ProtoReflect().Descriptor().Fields().ByName("directional_movement_supported").Number() != 10 ||
		(&C2S_PlayerAction{}).ProtoReflect().Descriptor().Fields().ByName("move_direction").Number() != 6 {
		t.Fatal("directional contract tags changed")
	}
}
