package proto

import (
	"encoding/hex"
	"math"
	"testing"

	"google.golang.org/protobuf/proto"
)

func assertCombatWireRoundTrip(t *testing.T, message proto.Message) []byte {
	t.Helper()
	encoded, err := proto.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	decoded := message.ProtoReflect().New().Interface()
	if err := proto.Unmarshal(encoded, decoded); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(message, decoded) {
		t.Fatalf("wire round trip changed message: want %v, got %v", message, decoded)
	}
	return encoded
}

func TestCombatActivationAnglePresence(t *testing.T) {
	for _, angle := range []*float32{nil, proto.Float32(0), proto.Float32(math.Pi / 2)} {
		request := &C2S_ActivateAction{ActionId: "axe_aoe", AimAngle: angle, StreamEpoch: 7}
		encoded := assertCombatWireRoundTrip(t, &ClientMessage{
			Sequence: 42, Payload: &ClientMessage_ActivateAction{ActivateAction: request},
		})
		if angle != nil && *angle == 0 {
			// Shared wire example with the client test locks the envelope and presence of zero.
			const zeroAim = "082ad201100a076178655f616f6515000000001807"
			if hex.EncodeToString(encoded) != zeroAim {
				t.Fatalf("zero-aim wire contract changed: %x", encoded)
			}
		}
	}

	legacy, err := hex.DecodeString("082ad201060a046c696674")
	if err != nil {
		t.Fatal(err)
	}
	var packet ClientMessage
	if err := proto.Unmarshal(legacy, &packet); err != nil {
		t.Fatal(err)
	}
	request := packet.GetActivateAction()
	if request == nil || request.ActionId != "lift" || request.AimAngle != nil || request.StreamEpoch != 0 || packet.Sequence != 42 {
		t.Fatalf("legacy activation changed: %v", &packet)
	}
}

func TestCombatPresentationWireRoundTrip(t *testing.T) {
	assertCombatWireRoundTrip(t, &ServerMessage{Payload: &ServerMessage_ActionList{ActionList: &S2C_ActionList{
		Actions: []*ActionDefinition{
			{Id: "lift", TargetKind: "object"},
			{Id: "axe_aoe", TargetKind: "direction", Stamina: 60, CooldownMs: 2000,
				Sector: &ActionSector{Range: 18, SectorAngle: math.Pi / 2}},
		},
	}}})

	for _, angle := range []*float32{nil, proto.Float32(0), proto.Float32(math.Pi / 2)} {
		animation := &CharacterActionAnimationState{
			Generation: "0:4294967297", Revision: 9007199254740993, AnimationKey: "axe_aoe",
			TotalTicks: 6, ElapsedTicks: 3, TickDurationMs: 100, ServerTimeMs: 1790970000000,
			FacingAngle: angle,
		}
		if angle == nil {
			animation.TargetPosition = &Position{X: 18, Y: -12}
		}
		assertCombatWireRoundTrip(t, &ServerMessage{Payload: &ServerMessage_CharacterActionAnimation{
			CharacterActionAnimation: &S2C_CharacterActionAnimation{EntityId: 42, StreamEpoch: 7, State: animation},
		}})
		assertCombatWireRoundTrip(t, &ServerMessage{Payload: &ServerMessage_ObjectSpawn{
			ObjectSpawn: &S2C_ObjectSpawn{EntityId: 42, StreamEpoch: 7, ActionAnimation: animation},
		}})
	}
}

func TestAttackResultWireRoundTrip(t *testing.T) {
	for name, hits := range map[string][]*AttackHit{
		"miss": nil,
		"hits": {
			{TargetId: 9007199254740995, Damage: 3.6},
			{TargetId: 9007199254740997, Damage: 0},
		},
	} {
		t.Run(name, func(t *testing.T) {
			encoded := assertCombatWireRoundTrip(t, &ServerMessage{Payload: &ServerMessage_AttackResult{
				AttackResult: &S2C_AttackResult{StreamEpoch: 7, EventId: math.MaxUint64, AttackerId: 9007199254740993, Hits: hits},
			}})
			// ServerMessage field 52 is length-delimited; no extra result envelope is introduced.
			if len(encoded) < 2 || encoded[0] != 0xa2 || encoded[1] != 0x03 {
				t.Fatalf("attack result envelope tag changed: %x", encoded)
			}
		})
	}
}

func TestActionExecutionStateWireRoundTrip(t *testing.T) {
	for _, angle := range []*float32{nil, proto.Float32(0), proto.Float32(2*math.Pi - 0.25)} {
		assertCombatWireRoundTrip(t, &ServerMessage{Payload: &ServerMessage_ActionStateChanged{ActionStateChanged: &S2C_ActionStateChanged{
			ActionId: "axe_sweep", Phase: "executing", ActionGeneration: math.MaxUint64,
			FacingAngle: angle, StreamEpoch: 7, ServerTimeMs: 1790970000000,
		}}})
	}
	state := &S2C_ActionStateChanged{}
	fields := state.ProtoReflect().Descriptor().Fields()
	if fields.ByName("action_generation").Number() != 6 || fields.ByName("facing_angle").Number() != 7 || fields.ByName("stream_epoch").Number() != 8 {
		t.Fatal("action execution confirmation field numbers changed")
	}
	if !fields.ByName("facing_angle").HasPresence() {
		t.Fatal("zero facing must remain distinct from an absent angle")
	}
}
