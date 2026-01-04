package game

import (
	"database/sql"
	"origin/internal/ecs/components"
	"time"

	"origin/internal/ecs"
	"origin/internal/persistence/repository"
)

type ObjectBuilder interface {
	Build(w *ecs.World, raw *repository.Object) (ecs.Handle, error)
	Serialize(w *ecs.World, h ecs.Handle) (*repository.Object, error)
	ObjectType() components.ObjectType
}

type ObjectFactory struct {
	builders map[components.ObjectType]ObjectBuilder
}

func NewObjectFactory() *ObjectFactory {
	return &ObjectFactory{
		builders: make(map[components.ObjectType]ObjectBuilder),
	}
}

func (f *ObjectFactory) RegisterBuilder(builder ObjectBuilder) {
	f.builders[builder.ObjectType()] = builder
}

func (f *ObjectFactory) Build(w *ecs.World, raw *repository.Object) (ecs.Handle, error) {
	objType := components.ObjectType(raw.ObjectType)
	builder, ok := f.builders[objType]
	if !ok {
		return ecs.InvalidHandle, ErrBuilderNotFound
	}
	return builder.Build(w, raw)
}

func (f *ObjectFactory) Serialize(w *ecs.World, h ecs.Handle, objType components.ObjectType) (*repository.Object, error) {
	builder, ok := f.builders[objType]
	if !ok {
		return nil, ErrBuilderNotFound
	}
	return builder.Serialize(w, h)
}

func (f *ObjectFactory) IsStatic(raw *repository.Object) bool {
	return raw.IsStatic.Valid && raw.IsStatic.Bool
}

type TreeBuilder struct{}

func (b *TreeBuilder) ObjectType() components.ObjectType { return components.ObjectTypeTree }

func (b *TreeBuilder) Build(w *ecs.World, raw *repository.Object) (ecs.Handle, error) {
	h := w.Spawn(ecs.EntityID(raw.ID))
	if h == ecs.InvalidHandle {
		return ecs.InvalidHandle, ErrEntitySpawnFailed
	}

	// Direction = raw.Heading * 45 degrees
	ecs.AddComponent(w, h, components.Transform{X: float32(raw.X), Y: float32(raw.Y), Direction: float32(raw.Heading.Int16) * 45})

	ecs.AddComponent(w, h, components.EntityInfo{
		IsStatic: raw.IsStatic.Valid && raw.IsStatic.Bool, Region: raw.Region, Layer: raw.Layer,
	})

	return h, nil
}

func (b *TreeBuilder) Serialize(w *ecs.World, h ecs.Handle) (*repository.Object, error) {
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
