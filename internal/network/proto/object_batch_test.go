package proto

import (
	"math"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestObjectBatchRoundTrip(t *testing.T) {
	spawn := &S2C_ObjectSpawn{
		EntityId: math.MaxUint64, TypeId: 7, ResourcePath: "player", StreamEpoch: 23,
		Position:          &EntityPosition{Position: &Position{X: -17, Y: 42, Heading: 1.25}, Size: &Vector2{X: 8, Y: 9}},
		CarriedByEntityId: 91, Name: "Player", NameColor: NicknameColor_NICKNAME_COLOR_DEFAULT,
		CharacterVisual: &CharacterVisualState{Generation: "0:4294967297", Revision: math.MaxUint64,
			Equipment: []*CharacterEquipmentVisual{{Slot: EquipSlot_EQUIP_SLOT_RIGHT_HAND, VisualKey: "stone_axe"}}},
		ActionAnimation: &CharacterActionAnimationState{Generation: "0:4294967297", Revision: math.MaxUint64,
			AnimationKey: "chop", TotalTicks: 20, ElapsedTicks: 4, TickDurationMs: 1000.0 / 60,
			ServerTimeMs: 10000, TargetPosition: &Position{X: -40, Y: 61}},
	}
	move := &S2C_ObjectMove{
		EntityId: math.MaxUint64, ServerTimeMs: 10000, MoveSeq: math.MaxUint32, IsTeleport: true, CarriedByEntityId: 91,
		Movement: &EntityMovement{Position: &Position{X: -17, Y: 42, Heading: 1.25}, Velocity: &Vector2{X: 3, Y: -2},
			MoveMode: MovementMode_MOVE_MODE_RUN, TargetPosition: &Vector2{X: 60, Y: -30}, IsMoving: true},
	}
	for name, want := range map[string]*ServerMessage{
		"spawn-single": {Payload: &ServerMessage_ObjectSpawn{ObjectSpawn: spawn}},
		"spawn-batch": {Payload: &ServerMessage_ObjectSpawnBatch{ObjectSpawnBatch: &S2C_ObjectSpawnBatch{
			Spawns: []*S2C_ObjectSpawn{spawn, {EntityId: 2, StreamEpoch: 24}},
		}}},
		"move-single": {Payload: &ServerMessage_ObjectMove{ObjectMove: move}},
		"move-batch": {Payload: &ServerMessage_ObjectMoveBatch{ObjectMoveBatch: &S2C_ObjectMoveBatch{
			Moves: []*S2C_ObjectMove{move, {EntityId: 2, Movement: &EntityMovement{Position: &Position{X: 1}}}},
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
				t.Fatalf("payload changed after round trip: %v", &got)
			}
		})
	}
}

func TestObjectBatchWireContract(t *testing.T) {
	serverFields := (&ServerMessage{}).ProtoReflect().Descriptor().Fields()
	for name, number := range map[protoreflect.Name]protoreflect.FieldNumber{
		"object_spawn": 16, "object_despawn": 17, "object_move": 18,
		"object_move_batch": 49, "object_spawn_batch": 50,
	} {
		field := serverFields.ByName(name)
		if field == nil || field.Number() != number || field.ContainingOneof() == nil {
			t.Fatalf("unexpected ServerMessage field %s: %v", name, field)
		}
	}
	for _, batch := range []proto.Message{&S2C_ObjectSpawnBatch{}, &S2C_ObjectMoveBatch{}} {
		fields := batch.ProtoReflect().Descriptor().Fields()
		if fields.Len() != 1 || fields.Get(0).Number() != 1 || fields.Get(0).Cardinality() != protoreflect.Repeated {
			t.Fatalf("batch must contain only repeated entries at field 1: %v", fields)
		}
	}
	if (&S2C_ObjectSpawn{}).ProtoReflect().Descriptor().Fields().ByName("stream_epoch").Number() != 7 {
		t.Fatal("spawn entry epoch field changed")
	}
	if (&S2C_ObjectMove{}).ProtoReflect().Descriptor().Fields().ByName("stream_epoch") != nil {
		t.Fatal("movement must retain its existing epoch-free protocol")
	}
}
