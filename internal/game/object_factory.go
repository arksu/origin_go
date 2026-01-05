package game

import (
	"origin/internal/ecs/components"

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
