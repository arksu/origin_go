package world

import (
	"encoding/json"
	"testing"

	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestSpawnEntityFromDefPublishesIndestructibleIndependentlyOfStatic(t *testing.T) {
	for _, isStatic := range []bool{false, true} {
		for _, indestructible := range []bool{false, true} {
			w := ecs.NewWorldForTesting()
			writes := 0
			w.AddComponentObserver(components.EntityInfoComponentID, func(h types.Handle) {
				info, exists := ecs.GetComponent[components.EntityInfo](w, h)
				require.True(t, exists)
				require.Equal(t, indestructible, info.Indestructible, "first entity-info publication must expose the definition policy")
				writes++
			})
			h := SpawnEntityFromDef(w, &objectdefs.ObjectDef{
				DefID: 9149, Key: "policy-fixture", HP: 100, IsStatic: isStatic, Indestructible: indestructible,
			}, DefSpawnParams{EntityID: 1})
			require.True(t, w.Alive(h))
			info, exists := ecs.GetComponent[components.EntityInfo](w, h)
			require.True(t, exists)
			require.Equal(t, isStatic, info.IsStatic)
			require.Equal(t, indestructible, info.Indestructible)
			require.Equal(t, 1, writes)
		}
	}
}

func TestObjectRestorationUsesCurrentDefinitionIndestructibility(t *testing.T) {
	for _, savedPolicy := range []bool{false, true} {
		name := "becomes_indestructible"
		if savedPolicy {
			name = "becomes_destructible"
		}
		t.Run(name, func(t *testing.T) {
			previous := objectdefs.Global()
			t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
			definition := objectdefs.ObjectDef{
				DefID: 9149, Key: "policy-fixture", HP: 100, IsStatic: true, Indestructible: savedPolicy,
			}
			objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{definition}))
			w := ecs.NewWorldForTesting()
			h := SpawnEntityFromDef(w, &definition, DefSpawnParams{EntityID: 1, X: 10, Y: 10})
			ecs.AddComponent(w, h, components.ChunkRef{})
			require.NoError(t, SetObjectHP(w, h, .49))
			factory := NewObjectFactory(nil)
			raw, err := factory.Serialize(w, h)
			require.NoError(t, err)
			require.False(t, raw.Data.Valid, "definition policy is not part of persisted behavior state")
			snapshot, err := factory.CaptureWorldObjectSnapshot(w, h)
			require.NoError(t, err)
			encoded, err := SerializeSnapshotToJSON(snapshot)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "indestructible")
			// Even an obsolete, externally supplied snapshot policy cannot override the live definition.
			var payload map[string]any
			require.NoError(t, json.Unmarshal(encoded, &payload))
			payload["indestructible"] = savedPolicy
			encoded, err = json.Marshal(payload)
			require.NoError(t, err)
			snapshot, err = DeserializeSnapshotFromJSON(encoded)
			require.NoError(t, err)

			definition.Indestructible = !savedPolicy
			objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{definition}))
			assertPolicy := func(restoredWorld *ecs.World, restored types.Handle) {
				t.Helper()
				info, exists := ecs.GetComponent[components.EntityInfo](restoredWorld, restored)
				require.True(t, exists)
				require.Equal(t, !savedPolicy, info.Indestructible)
				state, exists := ecs.GetComponent[components.ObjectInternalState](restoredWorld, restored)
				require.True(t, exists)
				require.True(t, state.HasHP)
				require.Equal(t, .49, state.HP)
			}
			for _, chunkBuild := range []bool{false, true} {
				restoredWorld := ecs.NewWorldForTesting()
				build := factory.Build
				if chunkBuild {
					build = factory.BuildForChunk
				}
				restored, err := build(restoredWorld, raw, nil)
				require.NoError(t, err)
				assertPolicy(restoredWorld, restored)
			}
			cm := newTestChunkManagerWithLoadWorkers(0)
			t.Cleanup(cm.Stop)
			chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
			chunk.SetState(types.ChunkStateActive)
			cm.chunks[chunk.Coord] = chunk
			restored, err := factory.SpawnWorldObjectFromSnapshot(cm.world, snapshot, SnapshotSpawnOptions{
				X: 10, Y: 10, ChunkManager: cm,
			})
			require.NoError(t, err)
			assertPolicy(cm.world, restored)
		})
	}
}

func TestObjectTransformRefreshesIndestructibilityWithoutSameTypeHealthReset(t *testing.T) {
	w := ecs.NewWorldForTesting()
	definition := objectdefs.ObjectDef{DefID: 9149, Key: "policy-fixture", HP: 100}
	h := SpawnEntityFromDef(w, &definition, DefSpawnParams{EntityID: 1})
	for _, indestructible := range []bool{true, false} {
		definition.Indestructible = indestructible
		require.NoError(t, SetObjectHP(w, h, .49))
		require.True(t, TransformObjectToDefInPlace(w, 1, h, &definition, TransformObjectInPlaceOptions{}))
		info, exists := ecs.GetComponent[components.EntityInfo](w, h)
		require.True(t, exists)
		require.Equal(t, indestructible, info.Indestructible)
		state, exists := ecs.GetComponent[components.ObjectInternalState](w, h)
		require.True(t, exists)
		require.Equal(t, .49, state.HP, "same type must refresh policy without healing")
	}
	for _, indestructible := range []bool{true, false} {
		definition.DefID++
		definition.HP += 100
		definition.Indestructible = indestructible
		require.True(t, TransformObjectToDefInPlace(w, 1, h, &definition, TransformObjectInPlaceOptions{}))
		info, _ := ecs.GetComponent[components.EntityInfo](w, h)
		require.Equal(t, indestructible, info.Indestructible)
		require.Equal(t, uint32(definition.DefID), info.TypeID)
		state, _ := ecs.GetComponent[components.ObjectInternalState](w, h)
		require.Equal(t, float64(definition.HP), state.HP)
	}
}
