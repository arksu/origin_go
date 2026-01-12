package game

import (
	"origin/internal/ecs"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

type ObjectBuilder interface {
	Build(w *ecs.World, raw *repository.Object) (types.Handle, error)
	Serialize(w *ecs.World, h types.Handle) (*repository.Object, error)
	ObjectType() types.ObjectType
}

type ObjectFactory struct {
	builders map[types.ObjectType]ObjectBuilder
}

func NewObjectFactory() *ObjectFactory {
	return &ObjectFactory{
		builders: make(map[types.ObjectType]ObjectBuilder),
	}
}

func (f *ObjectFactory) RegisterBuilder(builder ObjectBuilder) {
	f.builders[builder.ObjectType()] = builder
}

func (f *ObjectFactory) Build(w *ecs.World, raw *repository.Object) (types.Handle, error) {
	objType := types.ObjectType(raw.ObjectType)
	builder, ok := f.builders[objType]
	if !ok {
		return types.InvalidHandle, ErrBuilderNotFound
	}
	return builder.Build(w, raw)
}

func (f *ObjectFactory) Serialize(w *ecs.World, h types.Handle, objType types.ObjectType) (*repository.Object, error) {
	builder, ok := f.builders[objType]
	if !ok {
		return nil, ErrBuilderNotFound
	}
	return builder.Serialize(w, h)
}

func (f *ObjectFactory) IsStatic(raw *repository.Object) bool {
	return raw.IsStatic.Valid && raw.IsStatic.Bool
}
