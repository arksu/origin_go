package game

import (
	"database/sql"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

type TreeBuilder struct{}

func (b *TreeBuilder) ObjectType() components.ObjectType { return components.ObjectTypeTree }

func (b *TreeBuilder) Build(w *ecs.World, raw *repository.Object) (types.Handle, error) {
	h := w.Spawn(types.EntityID(raw.ID))
	if h == types.InvalidHandle {
		return types.InvalidHandle, ErrEntitySpawnFailed
	}

	// Direction = raw.Heading * 45 degrees
	ecs.AddComponent(w, h, components.Transform{X: int(raw.X), Y: int(raw.Y), Direction: float32(raw.Heading.Int16) * 45})

	ecs.AddComponent(w, h, components.EntityInfo{
		ObjectType: components.ObjectType(raw.ObjectType),
		IsStatic:   raw.IsStatic.Valid && raw.IsStatic.Bool,
		Region:     raw.Region,
		Layer:      raw.Layer,
	})

	return h, nil
}

func (b *TreeBuilder) Serialize(w *ecs.World, h types.Handle) (*repository.Object, error) {
	extID, ok := ecs.GetComponent[ecs.ExternalID](w, h)
	if !ok {
		return nil, ErrEntityNotFound
	}

	info, ok := ecs.GetComponent[components.EntityInfo](w, h)
	if !ok {
		return nil, ErrEntityNotFound
	}

	obj := &repository.Object{
		ID:         int64(extID.ID),
		ObjectType: int32(info.ObjectType),
		Region:     info.Region,
		Layer:      info.Layer,
		IsStatic:   sql.NullBool{Bool: info.IsStatic, Valid: true},
		UpdatedAt:  sql.NullTime{Time: time.Now(), Valid: true},
	}

	if pos, ok := ecs.GetComponent[components.Transform](w, h); ok {
		obj.X = int32(pos.X)
		obj.Y = int32(pos.Y)
		obj.Heading = sql.NullInt16{Int16: int16(pos.Direction / 45), Valid: true}
	}

	return obj, nil
}
