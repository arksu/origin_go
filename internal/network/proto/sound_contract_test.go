package proto

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestSoundGainPresenceSurvivesRoundTrip(t *testing.T) {
	for _, gain := range []*float32{nil, proto.Float32(0), proto.Float32(0.5)} {
		want := &S2C_Sound{SoundKey: "chop", X: -12.5, Y: 40.25, MaxHearDistance: 1000, DistanceGain: gain}
		encoded, err := proto.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		var got S2C_Sound
		if err := proto.Unmarshal(encoded, &got); err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(want, &got) || (got.DistanceGain == nil) != (gain == nil) {
			t.Fatalf("sound or gain presence changed: want %v, got %v", want, &got)
		}
	}
}

func TestSoundEnvelopeRoundTrip(t *testing.T) {
	for name, want := range map[string]*ServerMessage{
		"batch": {Payload: &ServerMessage_SoundBatch{SoundBatch: &S2C_SoundBatch{
			StreamEpoch: 7, ServerTimeMs: 1790970000000,
			Sounds: []*S2C_Sound{
				{SoundKey: "chop", X: -2, Y: 3, MaxHearDistance: 1000, DistanceGain: proto.Float32(0)},
				{SoundKey: "tree_fall", X: -2, Y: 3, MaxHearDistance: 1400, DistanceGain: proto.Float32(0.5)},
			},
		}}},
		"entry": {Payload: &ServerMessage_PlayerEnterWorld{PlayerEnterWorld: &S2C_PlayerEnterWorld{
			EntityId: 91, StreamEpoch: 7, Audio: &S2C_AudioParameters{Hearing: 1, FreshnessMs: 500},
		}}},
	} {
		t.Run(name, func(t *testing.T) {
			encoded, err := proto.Marshal(want)
			if err != nil {
				t.Fatal(err)
			}
			var got ServerMessage
			if err := proto.Unmarshal(encoded, &got); err != nil {
				t.Fatal(err)
			}
			if !proto.Equal(want, &got) {
				t.Fatalf("audio envelope changed: want %v, got %v", want, &got)
			}
		})
	}
}

func TestSoundWireFieldNumbersRemainStable(t *testing.T) {
	soundFields := (&S2C_Sound{}).ProtoReflect().Descriptor().Fields()
	for name, number := range map[protoreflect.Name]protoreflect.FieldNumber{
		"sound_key": 1, "x": 2, "y": 3, "max_hear_distance": 4, "distance_gain": 5,
	} {
		field := soundFields.ByName(name)
		if field == nil || field.Number() != number {
			t.Fatalf("unexpected sound field %s: %v", name, field)
		}
	}
	if !soundFields.ByName("distance_gain").HasPresence() {
		t.Fatal("distance gain must distinguish absent from explicit zero")
	}
	serverFields := (&ServerMessage{}).ProtoReflect().Descriptor().Fields()
	for name, number := range map[protoreflect.Name]protoreflect.FieldNumber{"sound": 29, "sound_batch": 51} {
		field := serverFields.ByName(name)
		if field == nil || field.Number() != number || field.ContainingOneof() == nil {
			t.Fatalf("unexpected server sound field %s: %v", name, field)
		}
	}
	entryFields := (&S2C_PlayerEnterWorld{}).ProtoReflect().Descriptor().Fields()
	if entryFields.ByName("audio").Number() != 11 || entryFields.ByName("stream_epoch").Number() != 9 {
		t.Fatal("world-entry field numbers changed")
	}
}
