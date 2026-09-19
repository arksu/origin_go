package proto

import (
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestMapClickRoundTrip(t *testing.T) {
	want := &C2S_PlayerAction{Action: &C2S_PlayerAction_MapClick{MapClick: &MapClick{X: -42, Y: 99, TargetEntityId: 9007199254740991}}, Modifiers: 5}
	wire, err := proto.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got C2S_PlayerAction
	if err := proto.Unmarshal(wire, &got); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(want, &got) {
		t.Fatalf("round trip: %v", &got)
	}
}

func TestRetiredMovementActionsAreNotMapClicks(t *testing.T) {
	for _, tag := range []byte{0x0a, 0x12} {
		var action C2S_PlayerAction
		if err := proto.Unmarshal([]byte{tag, 2, 8, 42}, &action); err != nil {
			t.Fatal(err)
		}
		if action.Action != nil {
			t.Fatalf("retired action interpreted as %T", action.Action)
		}
	}
}
