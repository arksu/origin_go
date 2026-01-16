package game

import (
	"database/sql"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
)

const treeSize = 10

type TreeBuilder struct{}

func (b *TreeBuilder) ObjectType() types.ObjectType { return types.ObjectTypeTree }

func (b *TreeBuilder) Build(w *ecs.World, raw *repository.Object) (types.Handle, error) {
	h := w.Spawn(types.EntityID(raw.ID))
	if h == types.InvalidHandle {
		return types.InvalidHandle, ErrEntitySpawnFailed
	}

	// Direction = raw.Heading * 45 degrees
	ecs.AddComponent(w, h, components.CreateTransform(raw.X, raw.Y, int(raw.Heading.Int16)))

	ecs.AddComponent(w, h, components.EntityInfo{
		ObjectType: types.ObjectType(raw.ObjectType),
		IsStatic:   raw.IsStatic.Valid && raw.IsStatic.Bool,
		Region:     raw.Region,
		Layer:      raw.Layer,
	})

	// TODO object size
	ecs.AddComponent(w, h, components.Collider{
		HalfWidth:  treeSize / 2.0,
		HalfHeight: treeSize / 2.0,
		Layer:      1,
		Mask:       1,
	})

	return h, nil
}

func (b *TreeBuilder) Serialize(w *ecs.World, h types.Handle) (*repository.Object, error) {
	externalID, ok := ecs.GetComponent[ecs.ExternalID](w, h)
	if !ok {
		return nil, ErrEntityNotFound
	}

	info, ok := ecs.GetComponent[components.EntityInfo](w, h)
	if !ok {
		return nil, ErrEntityNotFound
	}

	transform, ok := ecs.GetComponent[components.Transform](w, h)
	if !ok {
		return nil, ErrEntityNotFound
	}

	chunkRef, ok := ecs.GetComponent[components.ChunkRef](w, h)
	if !ok {
		return nil, ErrEntityNotFound
	}

	obj := &repository.Object{
		ID:         int64(externalID.ID),
		ObjectType: int(info.ObjectType),
		Region:     info.Region,
		X:          int(transform.X),
		Y:          int(transform.Y),
		Layer:      info.Layer,
		ChunkX:     chunkRef.CurrentChunkX,
		ChunkY:     chunkRef.CurrentChunkY,
		Heading:    sql.NullInt16{Int16: int16(transform.Direction), Valid: true},
		Quality:    sql.NullInt16{Int16: 10, Valid: true},  // TODO
		HpCurrent:  sql.NullInt32{Int32: 100, Valid: true}, // todo
		HpMax:      sql.NullInt32{Int32: 100, Valid: true}, // todo
		IsStatic:   sql.NullBool{Bool: info.IsStatic, Valid: true},
		OwnerID:    sql.NullInt64{},
		DataJsonb:  pqtype.NullRawMessage{},
		CreatedAt:  sql.NullTime{},
		CreateTick: 0,
		LastTick:   0, // todo
		UpdatedAt:  sql.NullTime{Time: time.Now(), Valid: true},
		DeletedAt:  sql.NullTime{},
	}

	return obj, nil
}

type PlayerBuilder struct{}

func (b *PlayerBuilder) ObjectType() types.ObjectType { return types.ObjectTypePlayer }

func (b *PlayerBuilder) Build(w *ecs.World, raw *repository.Object) (types.Handle, error) {
	return types.InvalidHandle, nil
}

func (b *PlayerBuilder) Serialize(w *ecs.World, h types.Handle) (*repository.Object, error) {
	return nil, nil
}
