package world

import (
	"database/sql"
	"encoding/json"
	"testing"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/objectdefs"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type npcTransformRetirementProbe func(types.Handle, contracts.NPCRetireReason)

func (probe npcTransformRetirementProbe) Retire(h types.Handle, reason contracts.NPCRetireReason) {
	probe(h, reason)
}

func TestNPCTransformRetiresOldExecutionBeforeDefinitionMutation(t *testing.T) {
	for _, toNPC := range []bool{false, true} {
		t.Run(map[bool]string{false: "object", true: "another_npc"}[toNPC], func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			def := objectdefs.ObjectDef{DefID: 9149, Key: "source", HP: 110, Transient: true, NPCConfig: &objectdefs.NPCBehaviorConfig{}}
			h := SpawnEntityFromDef(w, &def, DefSpawnParams{EntityID: 1})
			ecs.AddComponent(w, h, components.NPCBrain{})
			calls := 0
			ecs.InitResource(w, contracts.NPCLifecycle{Controller: npcTransformRetirementProbe(func(target types.Handle, reason contracts.NPCRetireReason) {
				calls++
				require.Equal(t, h, target)
				require.Equal(t, contracts.NPCRetireTransform, reason)
				info, _ := ecs.GetComponent[components.EntityInfo](w, target)
				require.Equal(t, uint32(def.DefID), info.TypeID, "the old execution must still see its definition")
				require.True(t, ecs.HasComponent[components.NPCBrain](w, target))
			})})
			require.True(t, TransformObjectToDefInPlace(w, 1, h, &def, TransformObjectInPlaceOptions{}))
			require.Zero(t, calls, "same-definition refresh preserves execution")
			next := objectdefs.ObjectDef{DefID: 9150, Key: "next", HP: 100, Transient: true}
			if toNPC {
				next.NPCConfig = &objectdefs.NPCBehaviorConfig{}
			}
			require.True(t, TransformObjectToDefInPlace(w, 1, h, &next, TransformObjectInPlaceOptions{}))
			require.Equal(t, 1, calls)
		})
	}
}

func TestTransientDefinitionDefaultsAndRuntimeCache(t *testing.T) {
	var def objectdefs.ObjectDef
	require.NoError(t, json.Unmarshal([]byte(`{"defId":9149,"key":"runtime","hp":110}`), &def))
	require.False(t, def.Transient)
	w := ecs.NewWorldForTesting()
	h := SpawnEntityFromDef(w, &def, DefSpawnParams{EntityID: 1})
	info, _ := ecs.GetComponent[components.EntityInfo](w, h)
	require.False(t, info.Transient)
	def.Transient = true
	h = SpawnEntityFromDef(w, &def, DefSpawnParams{EntityID: 2})
	for _, transient := range []bool{true, false} {
		def.Transient = transient
		require.NoError(t, SetObjectHP(w, h, 12))
		require.True(t, TransformObjectToDefInPlace(w, 2, h, &def, TransformObjectInPlaceOptions{}))
		info, _ = ecs.GetComponent[components.EntityInfo](w, h)
		health, _ := ecs.GetComponent[components.ObjectInternalState](w, h)
		require.Equal(t, transient, info.Transient)
		require.Equal(t, 12.0, health.HP, "refreshing policy must not heal the object")
	}
	def.Transient = true
	h = SpawnEntityFromDef(w, &def, DefSpawnParams{EntityID: 3})
	info, _ = ecs.GetComponent[components.EntityInfo](w, h)
	health, _ := ecs.GetComponent[components.ObjectInternalState](w, h)
	require.True(t, info.Transient)
	require.False(t, health.IsDirty)
	require.Equal(t, 110.0, health.HP)
}

func TestTransformPersistentToTransientRejectsBeforeMutation(t *testing.T) {
	w := ecs.NewWorldForTesting()
	def := objectdefs.ObjectDef{DefID: 9149, Key: "persistent", HP: 100}
	h := SpawnEntityFromDef(w, &def, DefSpawnParams{EntityID: 1})
	info, _ := ecs.GetComponent[components.EntityInfo](w, h)
	health, _ := ecs.GetComponent[components.ObjectInternalState](w, h)
	def.Transient = true
	for _, sameType := range []bool{true, false} {
		if !sameType {
			def.DefID++
		}
		require.False(t, TransformObjectToDefInPlace(w, 1, h, &def, TransformObjectInPlaceOptions{}))
		afterInfo, _ := ecs.GetComponent[components.EntityInfo](w, h)
		afterHealth, _ := ecs.GetComponent[components.ObjectInternalState](w, h)
		require.Equal(t, info, afterInfo)
		require.Equal(t, health, afterHealth)
	}
}

func TestTransientObjectSkipsPeriodicSaveSnapshotAndChunkReload(t *testing.T) {
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	def := objectdefs.ObjectDef{DefID: 9149, Key: "runtime", HP: 110, Transient: true}
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{def}))
	cm := newTestChunkManagerWithLoadWorkers(0)
	defer cm.Stop()
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
	chunk.SetState(types.ChunkStateActive)
	cm.chunks[chunk.Coord] = chunk
	h := SpawnEntityFromDef(cm.world, &def, DefSpawnParams{EntityID: 1, Region: 1, X: 10, Y: 10})
	ecs.AddComponent(cm.world, h, components.ChunkRef{})
	chunk.Spatial().AddDynamic(h, 10, 10)
	require.NoError(t, SetObjectHP(cm.world, h, 12))
	require.False(t, chunk.IsDirty(cm.world), "runtime health must not request a chunk save")
	// A nonnil, unconnected DB panics on any query: no insert or delete is
	// allowed even when a save is explicitly requested for this chunk.
	require.NoError(t, chunk.SaveToDB(&persistence.Postgres{}, cm.world, cm.objectFactory, zap.NewNop()))
	require.Empty(t, chunk.GetDeletedObjectIDs())
	raw, err := cm.objectFactory.Serialize(cm.world, h)
	require.NoError(t, err)
	require.Nil(t, raw)
	_, err = cm.objectFactory.CaptureWorldObjectSnapshot(cm.world, h)
	require.ErrorContains(t, err, "not snapshot-persistable")
	require.ErrorContains(t, cm.objectFactory.PersistWorldObjectNow(&persistence.Postgres{}, cm.world, h), "not persistable")
	require.NoError(t, cm.deactivateChunkInternal(chunk))
	require.False(t, cm.world.Alive(h))
	require.Empty(t, chunk.GetRawObjects())
	require.Empty(t, chunk.GetRawDirtyObjectIDs())
	require.Empty(t, chunk.GetRawInventoriesByOwner())
	require.Empty(t, chunk.GetDeletedObjectIDs())
	require.False(t, chunk.IsDirty(cm.world))
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	require.Equal(t, types.InvalidHandle, cm.world.GetHandleByEntityID(1))
}

func TestTransientRestoreUsesDefinitionAndPersistentObjectsStillSerialize(t *testing.T) {
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	for _, transient := range []bool{false, true} {
		objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 9149, Key: "runtime", HP: 110, Transient: transient}}))
		w := ecs.NewWorldForTesting()
		factory := NewObjectFactory(nil)
		h, err := factory.Build(w, &repository.Object{ID: 1, TypeID: 9149, Region: 1}, nil)
		require.NoError(t, err)
		info, _ := ecs.GetComponent[components.EntityInfo](w, h)
		require.Equal(t, transient, info.Transient)
		ecs.AddComponent(w, h, components.ChunkRef{})
		raw, err := factory.Serialize(w, h)
		require.NoError(t, err)
		if transient {
			require.Nil(t, raw)
		} else {
			require.NotNil(t, raw)
			require.Equal(t, 110.0, raw.Hp.Float64)
		}
	}
}

func TestTransientObjectPostgresMixedChunkSaveAndReload(t *testing.T) {
	db := newObjectHealthPostgres(t)
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	transient := objectdefs.ObjectDef{DefID: 9149, Key: "runtime", HP: 110, Transient: true}
	persistent := objectdefs.ObjectDef{DefID: 9150, Key: "persistent", HP: 100}
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{transient, persistent}))
	cm := newTestChunkManagerWithLoadWorkers(0)
	defer cm.Stop()
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
	chunk.SetState(types.ChunkStateActive)
	cm.chunks[chunk.Coord] = chunk
	for i, def := range []*objectdefs.ObjectDef{&transient, &persistent} {
		h := SpawnEntityFromDef(cm.world, def, DefSpawnParams{EntityID: types.EntityID(i + 1), Region: 1, X: 10, Y: 10})
		ecs.AddComponent(cm.world, h, components.ChunkRef{})
		chunk.Spatial().AddDynamic(h, 10, 10)
		require.NoError(t, SetObjectHP(cm.world, h, 12))
	}
	require.NoError(t, chunk.SaveToDB(db, cm.world, cm.objectFactory, zap.NewNop()))
	_, err := db.Queries().GetObjectByID(t.Context(), 1)
	require.ErrorIs(t, err, sql.ErrNoRows)
	row, err := db.Queries().GetObjectByID(t.Context(), 2)
	require.NoError(t, err)
	require.Equal(t, 12.0, row.Hp.Float64)
	require.NoError(t, cm.deactivateChunkInternal(chunk))
	require.Len(t, chunk.GetRawObjects(), 1)
	require.NoError(t, chunk.SaveToDB(db, cm.world, cm.objectFactory, zap.NewNop()))
	rows, err := db.Queries().GetObjectsByChunk(t.Context(), repository.GetObjectsByChunkParams{Region: 1})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(2), rows[0].ID)
	chunk.SetRawObjects([]*repository.Object{&rows[0]})
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	require.Equal(t, types.InvalidHandle, cm.world.GetHandleByEntityID(1))
	require.True(t, cm.world.Alive(cm.world.GetHandleByEntityID(2)))
}

func TestTransformNPCToPersistentObjectRetiresMovementAndSchedules(t *testing.T) {
	cm := newTestChunkManagerWithLoadWorkers(0)
	defer cm.Stop()
	w := cm.world
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
	chunk.SetState(types.ChunkStateActive)
	cm.chunks[chunk.Coord] = chunk
	transient := objectdefs.ObjectDef{DefID: 9149, Key: "runtime", HP: 110, Transient: true}
	h := SpawnEntityFromDef(w, &transient, DefSpawnParams{EntityID: 1, X: 10, Y: 20})
	ecs.AddComponent(w, h, components.ChunkRef{})
	chunk.Spatial().AddDynamic(h, 10, 20)
	ecs.AddComponent(w, h, components.NPCBrain{})
	ecs.AddComponent(w, h, components.Movement{State: constt.StateMoving, TargetType: constt.TargetPoint, VelocityX: 10})
	ecs.AddComponent(w, h, components.CollisionResult{})
	require.True(t, ecs.ScheduleBehaviorTick(w, 1, "npc_ai", 100))
	persistent := objectdefs.ObjectDef{DefID: 9150, Key: "persistent", HP: 100, IsStatic: true}
	require.True(t, TransformObjectToDefInPlace(w, 1, h, &persistent, TransformObjectInPlaceOptions{Chunks: cm}))
	require.False(t, ecs.HasComponent[components.NPCBrain](w, h))
	require.False(t, ecs.HasComponent[components.Movement](w, h))
	require.False(t, ecs.HasComponent[components.CollisionResult](w, h))
	require.Zero(t, ecs.GetResource[ecs.BehaviorTickSchedule](w).PendingCount())
	info, _ := ecs.GetComponent[components.EntityInfo](w, h)
	health, _ := ecs.GetComponent[components.ObjectInternalState](w, h)
	require.False(t, info.Transient)
	require.True(t, health.IsDirty, "new persistent destination must be saved")
	require.Equal(t, 1, chunk.Spatial().StaticCount())
	require.Zero(t, chunk.Spatial().DynamicCount())
	require.Equal(t, []types.Handle{h}, chunk.GetHandles())
	// A same-type refresh must not duplicate membership.
	require.True(t, TransformObjectToDefInPlace(w, 1, h, &persistent, TransformObjectInPlaceOptions{Chunks: cm}))
	require.Equal(t, 1, chunk.Spatial().StaticCount())
	// Destruction removes the destination's static membership. No old dynamic
	// entry may survive it and expose a stale handle to later spatial queries.
	chunk.Spatial().RemoveStatic(h, 10, 20)
	w.Despawn(h)
	require.Empty(t, chunk.GetHandles())
}

func TestTransformStaticToPersistentNPCMovesSpatialMembership(t *testing.T) {
	cm := newTestChunkManagerWithLoadWorkers(0)
	defer cm.Stop()
	w := cm.world
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
	chunk.SetState(types.ChunkStateActive)
	cm.chunks[chunk.Coord] = chunk
	static := objectdefs.ObjectDef{DefID: 9149, Key: "static", HP: 100, IsStatic: true}
	h := SpawnEntityFromDef(w, &static, DefSpawnParams{EntityID: 1, X: 10, Y: 20})
	ecs.AddComponent(w, h, components.ChunkRef{})
	chunk.Spatial().AddStatic(h, 10, 20)
	npc := objectdefs.ObjectDef{DefID: 9150, Key: "npc", HP: 110, NPCConfig: &objectdefs.NPCBehaviorConfig{}}
	require.True(t, TransformObjectToDefInPlace(w, 1, h, &npc, TransformObjectInPlaceOptions{Chunks: cm}))
	info, _ := ecs.GetComponent[components.EntityInfo](w, h)
	require.False(t, info.IsStatic)
	require.Zero(t, chunk.Spatial().StaticCount())
	require.Equal(t, 1, chunk.Spatial().DynamicCount())
	require.Equal(t, []types.Handle{h}, chunk.GetHandles())
	require.True(t, TransformObjectToDefInPlace(w, 1, h, &npc, TransformObjectInPlaceOptions{Chunks: cm}))
	require.Equal(t, 1, chunk.Spatial().DynamicCount())
	chunk.Spatial().RemoveDynamic(h, 10, 20)
	w.Despawn(h)
	require.Empty(t, chunk.GetHandles())
}

func TestTransformStaticChangeRejectsMissingIndexedPositionBeforeMutation(t *testing.T) {
	cm := newTestChunkManagerWithLoadWorkers(0)
	defer cm.Stop()
	w := cm.world
	cm.chunks[types.ChunkCoord{}] = core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
	def := objectdefs.ObjectDef{DefID: 9149, Key: "npc", HP: 110, Transient: true}
	h := SpawnEntityFromDef(w, &def, DefSpawnParams{EntityID: 1})
	ecs.AddComponent(w, h, components.ChunkRef{})
	ecs.AddComponent(w, h, components.NPCBrain{})
	ecs.AddComponent(w, h, components.Movement{State: constt.StateMoving})
	ecs.RemoveComponent[components.Transform](w, h)
	destination := objectdefs.ObjectDef{DefID: 9150, Key: "static", HP: 100, IsStatic: true}
	require.False(t, TransformObjectToDefInPlace(w, 1, h, &destination, TransformObjectInPlaceOptions{Chunks: cm}))
	info, _ := ecs.GetComponent[components.EntityInfo](w, h)
	require.EqualValues(t, def.DefID, info.TypeID)
	require.True(t, info.Transient)
	require.False(t, info.IsStatic)
	require.True(t, ecs.HasComponent[components.NPCBrain](w, h))
	movement, _ := ecs.GetComponent[components.Movement](w, h)
	require.Equal(t, constt.StateMoving, movement.State)
}

func TestTransformPersistentStaticChangeWithoutLoadedChunk(t *testing.T) {
	cm := newTestChunkManagerWithLoadWorkers(0)
	defer cm.Stop()
	w := cm.world
	def := objectdefs.ObjectDef{DefID: 9149, Key: "construction", HP: 100}
	h := SpawnEntityFromDef(w, &def, DefSpawnParams{EntityID: 1})
	ecs.AddComponent(w, h, components.ChunkRef{})
	destination := objectdefs.ObjectDef{DefID: 9150, Key: "static", HP: 110, IsStatic: true}
	require.True(t, TransformObjectToDefInPlace(w, 1, h, &destination, TransformObjectInPlaceOptions{Chunks: cm}))
	info, _ := ecs.GetComponent[components.EntityInfo](w, h)
	health, _ := ecs.GetComponent[components.ObjectInternalState](w, h)
	require.EqualValues(t, destination.DefID, info.TypeID)
	require.True(t, info.IsStatic)
	require.True(t, health.IsDirty)
	require.Equal(t, 110.0, health.HP)
}
