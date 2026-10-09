package world

import (
	"testing"

	"github.com/sqlc-dev/pqtype"
	"origin/internal/ecs/components"
	"origin/internal/persistence/repository"
)

func TestObjectFactoryCorpseDecayStateRoundTrip(t *testing.T) {
	factory := NewObjectFactory(nil)
	state := components.ObjectInternalState{HP: 0.49, HasHP: true}
	components.SetBehaviorState(&state, "player_dead", &components.CorpseDecayBehaviorState{DecayAtRuntimeSeconds: 22100})
	payload, valid, err := serializePersistentObjectState(state)
	if err != nil || !valid {
		t.Fatalf("serialize corpse decay: valid %v, error %v", valid, err)
	}
	restored, err := factory.DeserializeObjectState(&repository.Object{TypeID: 15, Data: pqtype.NullRawMessage{RawMessage: payload, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	state.State = restored
	state.IsDirty = false
	decay, ok := components.GetBehaviorState[components.CorpseDecayBehaviorState](state, "player_dead")
	if !ok || decay.DecayAtRuntimeSeconds != 22100 || state.HP != 0.49 || !state.HasHP || state.IsDirty {
		t.Fatalf("roundtrip changed corpse state: %+v, decay %+v", state, decay)
	}
}

func TestObjectFactoryCorpseDecayStateRejectsInvalidDeadlineType(t *testing.T) {
	_, err := NewObjectFactory(nil).DeserializeObjectState(&repository.Object{
		TypeID: 15,
		Data:   pqtype.NullRawMessage{RawMessage: []byte(`{"v":1,"behaviors":{"player_dead":{"decay_at_runtime_seconds":"invalid"}}}`), Valid: true},
	})
	if err == nil {
		t.Fatal("invalid typed corpse deadline accepted")
	}
}
