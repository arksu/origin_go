package proto

import (
	"testing"

	goproto "google.golang.org/protobuf/proto"
)

func TestCraftStartRequestsDoNotExposeStationStateOrConsumption(t *testing.T) {
	tests := []struct {
		name       string
		message    goproto.Message
		fieldNames []string
	}{
		{
			name:       "one cycle",
			message:    &C2S_StartCraftOne{},
			fieldNames: []string{"craft_key"},
		},
		{
			name:       "many cycles",
			message:    &C2S_StartCraftMany{},
			fieldNames: []string{"craft_key", "cycles"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fields := test.message.ProtoReflect().Descriptor().Fields()
			if fields.Len() != len(test.fieldNames) {
				t.Fatalf("field count = %d, want %d", fields.Len(), len(test.fieldNames))
			}
			for index, want := range test.fieldNames {
				if got := string(fields.Get(index).Name()); got != want {
					t.Fatalf("field %d = %q, want %q", index, got, want)
				}
			}
		})
	}
}
