package proto

import (
	protobuf "google.golang.org/protobuf/proto"
	"math"
	"testing"
)

func TestCombatProtocolRoundTrip(t *testing.T) {
	input := &ServerMessage{Payload: &ServerMessage_CombatResult{CombatResult: &S2C_CombatResult{
		EntityId: math.MaxUint64, Generation: "0:18446744073709551615", StreamEpoch: 42, ExecutionId: math.MaxUint64 - 1, EventSequence: math.MaxUint64 - 2, ServerTimeMs: 600, Hit: true,
		Hits: []*CombatHitResult{{EventSequence: math.MaxUint64 - 3, TargetId: math.MaxUint64 - 4, RawDamage: 0.4, Damage: 0.4, Target: &CombatTargetState{Generation: "0:2", Revision: math.MaxUint64, Hp: 0.6, MaxHp: 1}}},
	}}}
	encoded, err := protobuf.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	output := &ServerMessage{}
	if err := protobuf.Unmarshal(encoded, output); err != nil {
		t.Fatal(err)
	}
	if !protobuf.Equal(input, output) {
		t.Fatal("combat identity or fractional precision lost")
	}
	animation := &CharacterActionAnimationState{Generation: "0:1", Revision: 3, ExecutionId: 4, ElapsedMs: 400, DurationMs: 1000, LockedDirection: &CombatDirection{X: 0.6, Y: 0.8}}
	encoded, err = protobuf.Marshal(animation)
	if err != nil {
		t.Fatal(err)
	}
	decoded := &CharacterActionAnimationState{}
	if err := protobuf.Unmarshal(encoded, decoded); err != nil || !protobuf.Equal(animation, decoded) {
		t.Fatalf("timeline round trip: %v", err)
	}
	legacy := &MapClick{X: 1, Y: 2, TargetEntityId: 3}
	encoded, err = protobuf.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	decodedClick := &MapClick{}
	if err := protobuf.Unmarshal(encoded, decodedClick); err != nil || decodedClick.CombatAttempt != nil || !protobuf.Equal(legacy, decodedClick) {
		t.Fatal("legacy click changed")
	}
	if (&C2S_ActivateAction{ActionId: "dig"}).RequestRevision != 0 || (&S2C_PlayerEnterWorld{}).CombatSupported {
		t.Fatal("legacy defaults changed")
	}
}
