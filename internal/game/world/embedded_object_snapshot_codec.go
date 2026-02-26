package world

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sqlc-dev/pqtype"
	"go.uber.org/zap"

	_const "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	ecssystems "origin/internal/ecs/systems"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

const embeddedObjectSnapshotVersion = 1

type SnapshotSpawnOptions struct {
	X int
	Y int

	Layer int

	ChunkManager     *ChunkManager
	BehaviorRegistry contracts.BehaviorRegistry
	EventBus         *eventbus.EventBus
	Logger           *zap.Logger
}

func (f *ObjectFactory) CaptureWorldObjectSnapshot(w *ecs.World, h types.Handle) (EmbeddedObjectSnapshotV1, error) {
	if f == nil || w == nil || h == types.InvalidHandle || !w.Alive(h) {
		return EmbeddedObjectSnapshotV1{}, fmt.Errorf("invalid capture target")
	}

	obj, err := f.Serialize(w, h)
	if err != nil {
		return EmbeddedObjectSnapshotV1{}, err
	}
	if obj == nil {
		return EmbeddedObjectSnapshotV1{}, fmt.Errorf("entity is not snapshot-persistable")
	}

	inventories, err := f.SerializeObjectInventories(w, h)
	if err != nil {
		return EmbeddedObjectSnapshotV1{}, err
	}

	snapshot := EmbeddedObjectSnapshotV1{
		Version:  embeddedObjectSnapshotVersion,
		EntityID: uint64(obj.ID),
		TypeID:   obj.TypeID,
		Region:   obj.Region,
		Layer:    obj.Layer,
		Quality:  obj.Quality,
	}
	if obj.Heading.Valid {
		v := obj.Heading.Int16
		snapshot.Heading = &v
	}
	if obj.Data.Valid && len(obj.Data.RawMessage) > 0 {
		snapshot.ObjectData = append([]byte(nil), obj.Data.RawMessage...)
	}
	if len(inventories) > 0 {
		snapshot.RootInventories = make([]EmbeddedInventorySnapshotV1, 0, len(inventories))
		for _, inv := range inventories {
			row := EmbeddedInventorySnapshotV1{
				Kind:         inv.Kind,
				InventoryKey: inv.InventoryKey,
				Version:      inv.Version,
				Data:         append([]byte(nil), inv.Data...),
			}
			snapshot.RootInventories = append(snapshot.RootInventories, row)
		}
	}
	return snapshot, nil
}

func SerializeSnapshotToJSON(snapshot EmbeddedObjectSnapshotV1) ([]byte, error) {
	if snapshot.Version == 0 {
		snapshot.Version = embeddedObjectSnapshotVersion
	}
	return json.Marshal(snapshot)
}

func DeserializeSnapshotFromJSON(data []byte) (EmbeddedObjectSnapshotV1, error) {
	var snapshot EmbeddedObjectSnapshotV1
	if len(data) == 0 {
		return snapshot, fmt.Errorf("empty snapshot")
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return snapshot, err
	}
	if snapshot.Version != embeddedObjectSnapshotVersion {
		return snapshot, fmt.Errorf("unsupported embedded object snapshot version %d", snapshot.Version)
	}
	return snapshot, nil
}

func (f *ObjectFactory) SpawnWorldObjectFromSnapshot(
	w *ecs.World,
	snapshot EmbeddedObjectSnapshotV1,
	opts SnapshotSpawnOptions,
) (types.Handle, error) {
	if f == nil || w == nil {
		return types.InvalidHandle, fmt.Errorf("nil spawn dependencies")
	}
	if snapshot.Version != embeddedObjectSnapshotVersion {
		return types.InvalidHandle, fmt.Errorf("unsupported snapshot version %d", snapshot.Version)
	}
	if snapshot.EntityID == 0 {
		return types.InvalidHandle, fmt.Errorf("snapshot entity id is zero")
	}
	if opts.ChunkManager == nil {
		return types.InvalidHandle, fmt.Errorf("nil chunk manager")
	}
	if opts.Logger == nil {
		opts.Logger = zap.NewNop()
	}

	coord := types.WorldToChunkCoord(opts.X, opts.Y, _const.ChunkSize, _const.CoordPerTile)
	chunk := opts.ChunkManager.GetChunkFast(coord)
	if chunk == nil || chunk.GetState() != types.ChunkStateActive {
		return types.InvalidHandle, fmt.Errorf("target chunk not active")
	}

	raw := repository.Object{
		ID:      int64(snapshot.EntityID),
		TypeID:  snapshot.TypeID,
		Region:  snapshot.Region,
		X:       opts.X,
		Y:       opts.Y,
		Layer:   opts.Layer,
		ChunkX:  coord.X,
		ChunkY:  coord.Y,
		Quality: snapshot.Quality,
	}
	if snapshot.Heading != nil {
		raw.Heading = sql.NullInt16{Int16: *snapshot.Heading, Valid: true}
	}
	if len(snapshot.ObjectData) > 0 {
		raw.Data = pqtype.NullRawMessage{RawMessage: append([]byte(nil), snapshot.ObjectData...), Valid: true}
	}

	inventories := make([]repository.Inventory, 0, len(snapshot.RootInventories))
	for _, inv := range snapshot.RootInventories {
		inventories = append(inventories, repository.Inventory{
			OwnerID:      int64(snapshot.EntityID),
			Kind:         inv.Kind,
			InventoryKey: inv.InventoryKey,
			Data:         append([]byte(nil), inv.Data...),
			Version:      inv.Version,
		})
	}

	h, err := f.Build(w, &raw, inventories)
	if err != nil {
		return types.InvalidHandle, err
	}
	if h == types.InvalidHandle {
		return types.InvalidHandle, fmt.Errorf("object build from snapshot failed")
	}

	ecs.AddComponent(w, h, components.ChunkRef{
		CurrentChunkX: coord.X,
		CurrentChunkY: coord.Y,
		PrevChunkX:    coord.X,
		PrevChunkY:    coord.Y,
	})

	restoredState, stateErr := f.DeserializeObjectState(&raw)
	if stateErr != nil {
		opts.Logger.Warn("SpawnWorldObjectFromSnapshot: failed to deserialize runtime object state",
			zap.Uint64("entity_id", snapshot.EntityID),
			zap.Error(stateErr),
		)
	}
	ecs.AddComponent(w, h, components.ObjectInternalState{
		State:   restoredState,
		IsDirty: true,
	})
	f.RestoreDerivedComponentsFromState(w, h)

	if info, hasInfo := ecs.GetComponent[components.EntityInfo](w, h); hasInfo && len(info.Behaviors) > 0 && opts.BehaviorRegistry != nil {
		if initErr := opts.BehaviorRegistry.InitObjectBehaviors(&contracts.BehaviorObjectInitContext{
			World:      w,
			Handle:     h,
			EntityID:   types.EntityID(snapshot.EntityID),
			EntityType: info.TypeID,
			Reason:     contracts.ObjectBehaviorInitReasonRestore,
		}, info.Behaviors); initErr != nil {
			return types.InvalidHandle, initErr
		}
		ecssystems.RecomputeObjectBehaviorsNow(w, opts.EventBus, opts.Logger, opts.BehaviorRegistry, []types.Handle{h})
		ecs.WithComponent(w, h, func(state *components.ObjectInternalState) {
			state.IsDirty = true
		})
	}

	if info, hasInfo := ecs.GetComponent[components.EntityInfo](w, h); hasInfo && info.IsStatic {
		chunk.Spatial().AddStatic(h, opts.X, opts.Y)
	} else {
		chunk.Spatial().AddDynamic(h, opts.X, opts.Y)
	}
	chunk.MarkRawDataDirty()

	return h, nil
}

func (f *ObjectFactory) PersistWorldObjectNow(
	db *persistence.Postgres,
	w *ecs.World,
	h types.Handle,
) error {
	if f == nil || db == nil || w == nil || h == types.InvalidHandle || !w.Alive(h) {
		return fmt.Errorf("invalid persist target")
	}
	obj, err := f.Serialize(w, h)
	if err != nil {
		return err
	}
	if obj == nil {
		return fmt.Errorf("entity is not persistable")
	}
	var inventories []repository.Inventory
	if info, hasInfo := ecs.GetComponent[components.EntityInfo](w, h); hasInfo && f.HasPersistentInventories(info.TypeID, info.Behaviors) {
		inventories, err = f.SerializeObjectInventories(w, h)
		if err != nil {
			return err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.Queries().UpsertObject(ctx, repository.UpsertObjectParams{
		ID:         obj.ID,
		TypeID:     obj.TypeID,
		Region:     obj.Region,
		X:          obj.X,
		Y:          obj.Y,
		Layer:      obj.Layer,
		ChunkX:     obj.ChunkX,
		ChunkY:     obj.ChunkY,
		Heading:    obj.Heading,
		Quality:    obj.Quality,
		Hp:         obj.Hp,
		OwnerID:    obj.OwnerID,
		Data:       obj.Data,
		CreateTick: obj.CreateTick,
		LastTick:   obj.LastTick,
	}); err != nil {
		return fmt.Errorf("upsert moved object: %w", err)
	}
	for _, inv := range inventories {
		if _, err := db.Queries().UpsertInventory(ctx, repository.UpsertInventoryParams{
			OwnerID:      inv.OwnerID,
			Kind:         inv.Kind,
			InventoryKey: inv.InventoryKey,
			Data:         inv.Data,
			Version:      inv.Version,
		}); err != nil {
			return fmt.Errorf("upsert moved object inventory: %w", err)
		}
	}
	return nil
}
