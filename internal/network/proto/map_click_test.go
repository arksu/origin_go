package proto

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestMapClickRoundTrip(t *testing.T) {
	for _, button := range []MapClickButton{MapClickButton_MAP_CLICK_BUTTON_PRIMARY, MapClickButton_MAP_CLICK_BUTTON_SECONDARY} {
		t.Run(button.String(), func(t *testing.T) {
			want := &C2S_PlayerAction{Action: &C2S_PlayerAction_MapClick{MapClick: &MapClick{X: -42, Y: 99, TargetEntityId: 9007199254740991, Button: button}}, Modifiers: 5}
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
		})
	}
}

func TestOmittedMapClickButtonIsPrimary(t *testing.T) {
	var click MapClick
	if err := proto.Unmarshal([]byte{8, 42}, &click); err != nil {
		t.Fatal(err)
	}
	if click.GetButton() != MapClickButton_MAP_CLICK_BUTTON_PRIMARY || click.X != 42 {
		t.Fatalf("legacy MapClick changed: %v", &click)
	}
}

func TestRetiredActionsAreNotMapClicks(t *testing.T) {
	for _, tag := range []byte{0x0a, 0x12, 0x1a} {
		var action C2S_PlayerAction
		if err := proto.Unmarshal([]byte{tag, 2, 8, 42}, &action); err != nil {
			t.Fatal(err)
		}
		if action.Action != nil {
			t.Fatalf("retired action interpreted as %T", action.Action)
		}
	}
}

func TestMapClickProtocolReservations(t *testing.T) {
	descriptor := (&C2S_PlayerAction{}).ProtoReflect().Descriptor()
	for index, name := range []protoreflect.Name{"move_to", "move_to_entity", "interact"} {
		if !descriptor.ReservedNames().Has(name) || !descriptor.ReservedRanges().Has(protoreflect.FieldNumber(index+1)) {
			t.Fatalf("retired action %s is not reserved", name)
		}
	}
	if descriptor.Fields().ByName("map_click").Number() != 5 || descriptor.Fields().ByName("select_context_action").Number() != 4 {
		t.Fatal("supported action tags changed")
	}
	if (&MapClick{}).ProtoReflect().Descriptor().Fields().ByName("button").Number() != 4 {
		t.Fatal("MapClick button tag changed")
	}
}
