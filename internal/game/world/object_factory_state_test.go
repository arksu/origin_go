package world

import (
	"encoding/json"
	"testing"

	"github.com/sqlc-dev/pqtype"

	constt "origin/internal/const"
	"origin/internal/ecs/components"
	"origin/internal/persistence/repository"
)

func TestSerializePersistentObjectState_Empty(t *testing.T) {
	payload, hasPayload, err := serializePersistentObjectState(components.ObjectInternalState{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasPayload {
		t.Fatalf("expected no payload for empty state")
	}
	if payload != nil {
		t.Fatalf("expected nil payload for empty state")
	}
}

func TestSerializePersistentObjectState_Tree(t *testing.T) {
	internalState := components.ObjectInternalState{
		State: &components.RuntimeObjectState{
			Behaviors: map[string]any{
				"tree": &components.TreeBehaviorState{ChopPoints: 4},
			},
		},
	}

	payload, hasPayload, err := serializePersistentObjectState(internalState)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasPayload {
		t.Fatalf("expected payload")
	}

	var envelope components.ObjectStateEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("failed to unmarshal envelope: %v", err)
	}
	if envelope.Version != 1 {
		t.Fatalf("unexpected version: %d", envelope.Version)
	}
	if _, ok := envelope.Behaviors["tree"]; !ok {
		t.Fatalf("expected tree behavior payload")
	}
}

func TestSerializePersistentObjectState_Take(t *testing.T) {
	internalState := components.ObjectInternalState{
		State: &components.RuntimeObjectState{
			Behaviors: map[string]any{
				"take": &components.TakeBehaviorState{
					Taken: map[string]int{"chip_stone": 2},
				},
			},
		},
	}

	payload, hasPayload, err := serializePersistentObjectState(internalState)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasPayload {
		t.Fatalf("expected payload")
	}

	var envelope components.ObjectStateEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("failed to unmarshal envelope: %v", err)
	}
	if _, ok := envelope.Behaviors["take"]; !ok {
		t.Fatalf("expected take behavior payload")
	}
}

func TestSerializePersistentObjectState_StationRoundTrip(t *testing.T) {
	station := &components.StationState{
		CurrentState: "burning",
		Values:       map[string]float64{"temperature": 800},
		Resources:    map[string]uint32{"fuel": 3, "thread": 2},
	}
	payload, hasPayload, err := serializePersistentObjectState(components.ObjectInternalState{}, station)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasPayload {
		t.Fatal("expected station payload")
	}

	factory := &ObjectFactory{}
	state, err := factory.DeserializeObjectState(&repository.Object{
		TypeID: 1,
		Data:   pqtype.NullRawMessage{RawMessage: payload, Valid: true},
	})
	if err != nil {
		t.Fatalf("unexpected deserialize error: %v", err)
	}
	runtime, ok := state.(*components.RuntimeObjectState)
	if !ok || runtime.Station == nil {
		t.Fatal("expected restored station state")
	}
	if runtime.Station.CurrentState != "burning" || runtime.Station.Resources["fuel"] != 3 || runtime.Station.Values["temperature"] != 800 {
		t.Fatalf("unexpected restored station: %#v", runtime.Station)
	}
}

func TestDeserializeObjectState_Tree(t *testing.T) {
	factory := &ObjectFactory{}
	raw := &repository.Object{
		TypeID: 1,
		Data: pqtype.NullRawMessage{
			RawMessage: []byte(`{"v":1,"behaviors":{"tree":{"chop_points":5}}}`),
			Valid:      true,
		},
	}

	state, err := factory.DeserializeObjectState(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runtimeState, ok := state.(*components.RuntimeObjectState)
	if !ok || runtimeState == nil {
		t.Fatalf("expected runtime object state")
	}
	treeStateRaw, hasTree := runtimeState.Behaviors["tree"]
	if !hasTree {
		t.Fatalf("expected tree behavior state")
	}
	treeState, ok := treeStateRaw.(*components.TreeBehaviorState)
	if !ok {
		t.Fatalf("expected *TreeBehaviorState, got %T", treeStateRaw)
	}
	if treeState.ChopPoints != 5 {
		t.Fatalf("unexpected chop points: %d", treeState.ChopPoints)
	}
}

func TestDeserializeObjectState_Take(t *testing.T) {
	factory := &ObjectFactory{}
	raw := &repository.Object{
		TypeID: 1,
		Data: pqtype.NullRawMessage{
			RawMessage: []byte(`{"v":1,"behaviors":{"take":{"chip_stone_taken":5}}}`),
			Valid:      true,
		},
	}

	state, err := factory.DeserializeObjectState(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runtimeState, ok := state.(*components.RuntimeObjectState)
	if !ok || runtimeState == nil {
		t.Fatalf("expected runtime object state")
	}
	takeStateRaw, hasTake := runtimeState.Behaviors["take"]
	if !hasTake {
		t.Fatalf("expected take behavior state")
	}
	takeState, ok := takeStateRaw.(*components.TakeBehaviorState)
	if !ok {
		t.Fatalf("expected *TakeBehaviorState, got %T", takeStateRaw)
	}
	if takeState.Taken["chip_stone"] != 5 {
		t.Fatalf("unexpected taken count: %d", takeState.Taken["chip_stone"])
	}
}

func TestDeserializeObjectState_IgnoresDroppedItem(t *testing.T) {
	factory := &ObjectFactory{}
	raw := &repository.Object{
		TypeID: int(constt.DroppedItemTypeID),
		Data: pqtype.NullRawMessage{
			RawMessage: []byte(`{"v":1,"behaviors":{"tree":{"chop_points":5}}}`),
			Valid:      true,
		},
	}

	state, err := factory.DeserializeObjectState(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != nil {
		t.Fatalf("expected nil state for dropped item object")
	}
}
