package proto

import (
	"math"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestServerConstantsWireContract(t *testing.T) {
	fields := []protoreflect.Name{
		"coord_per_tile", "chunk_size", "tick_rate", "directional_movement_supported",
		"real_seconds_per_game_day", "hours_per_day", "days_per_month", "months_per_year",
	}
	descriptor := (&S2C_ServerConstants{}).ProtoReflect().Descriptor()
	if descriptor.Fields().Len() != len(fields) {
		t.Fatalf("unexpected constants fields: %d", descriptor.Fields().Len())
	}
	for i, name := range fields {
		field := descriptor.Fields().ByName(name)
		wantKind := protoreflect.Uint32Kind
		if name == "directional_movement_supported" {
			wantKind = protoreflect.BoolKind
		}
		if field == nil || field.Number() != protoreflect.FieldNumber(i+1) || field.Kind() != wantKind {
			t.Fatalf("constants field %s changed: %v", name, field)
		}
	}
	field := (&ServerMessage{}).ProtoReflect().Descriptor().Fields().ByName("server_constants")
	if field == nil || field.Number() != 53 || field.ContainingOneof().Name() != "payload" {
		t.Fatalf("constants payload contract changed: %v", field)
	}
	for _, enabled := range []bool{false, true} {
		want := &ServerMessage{Sequence: 42, Payload: &ServerMessage_ServerConstants{ServerConstants: &S2C_ServerConstants{
			CoordPerTile: 12, ChunkSize: 128, TickRate: 10, DirectionalMovementSupported: enabled,
			RealSecondsPerGameDay: 28800, HoursPerDay: 24, DaysPerMonth: 30, MonthsPerYear: 12,
		}}}
		wire, err := proto.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		var got ServerMessage
		if err := proto.Unmarshal(wire, &got); err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(want, &got) {
			t.Fatalf("constants round trip (enabled=%t): %v", enabled, &got)
		}
	}
}

func TestEnterWorldConstantsAreReserved(t *testing.T) {
	descriptor := (&S2C_PlayerEnterWorld{}).ProtoReflect().Descriptor()
	for name, number := range map[protoreflect.Name]protoreflect.FieldNumber{
		"coord_per_tile": 3, "chunk_size": 4, "tick_rate": 5, "directional_movement_supported": 10,
	} {
		if !descriptor.ReservedNames().Has(name) || !descriptor.ReservedRanges().Has(number) ||
			descriptor.Fields().ByName(name) != nil || descriptor.Fields().ByNumber(number) != nil {
			t.Fatalf("removed field %s=%d is not fully reserved", name, number)
		}
	}
	for name, number := range map[protoreflect.Name]protoreflect.FieldNumber{
		"entity_id": 1, "name": 2, "stream_epoch": 9, "audio": 11,
	} {
		field := descriptor.Fields().ByName(name)
		if field == nil || field.Number() != number {
			t.Fatalf("entry field %s changed: %v", name, field)
		}
	}
}

func TestPongOptionalRuntimeContract(t *testing.T) {
	descriptor := (&S2C_Pong{}).ProtoReflect().Descriptor()
	for name, number := range map[protoreflect.Name]protoreflect.FieldNumber{
		"client_time_ms": 1, "server_time_ms": 2, "runtime_seconds_total": 3,
	} {
		field := descriptor.Fields().ByName(name)
		if field == nil || field.Number() != number || field.Kind() != protoreflect.Int64Kind {
			t.Fatalf("Pong field %s changed: %v", name, field)
		}
	}
	if descriptor.Fields().Len() != 3 || !descriptor.Fields().ByName("runtime_seconds_total").HasPresence() {
		t.Fatal("Pong requires only one additional optional field")
	}
	// Original Pong wire: client_time_ms=42, server_time_ms=43, no runtime.
	var legacy S2C_Pong
	if err := proto.Unmarshal([]byte{8, 42, 16, 43}, &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.RuntimeSecondsTotal != nil || legacy.ClientTimeMs != 42 || legacy.ServerTimeMs != 43 {
		t.Fatalf("legacy wall-time Pong changed: %v", &legacy)
	}
	for _, seconds := range []int64{0, 1000, math.MaxInt64} {
		want := &S2C_Pong{ClientTimeMs: 42, ServerTimeMs: 43, RuntimeSecondsTotal: proto.Int64(seconds)}
		wire, err := proto.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		var got S2C_Pong
		if err := proto.Unmarshal(wire, &got); err != nil {
			t.Fatal(err)
		}
		if got.RuntimeSecondsTotal == nil || !proto.Equal(want, &got) {
			t.Fatalf("runtime %d presence/precision lost: %v", seconds, &got)
		}
	}
}
