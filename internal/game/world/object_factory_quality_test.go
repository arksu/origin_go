package world

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

func TestObjectFactoryBuild_LoadsQualityIntoEntityInfo(t *testing.T) {
	previousRegistry := objectdefs.Global()
	t.Cleanup(func() {
		objectdefs.SetGlobalForTesting(previousRegistry)
	})
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{
			DefID:    9101,
			Key:      "quality_obj",
			Name:     "Quality Object",
			Resource: "quality_obj",
			IsStatic: true,
		},
	}))

	world := ecs.NewWorldForTesting()
	factory := &ObjectFactory{}
	raw := &repository.Object{
		ID:      1001,
		TypeID:  9101,
		Region:  1,
		X:       10,
		Y:       20,
		Layer:   0,
		Quality: 42,
	}

	handle, err := factory.Build(world, raw, nil)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	info, hasInfo := ecs.GetComponent[components.EntityInfo](world, handle)
	if !hasInfo {
		t.Fatalf("expected EntityInfo")
	}
	if info.Quality != 42 {
		t.Fatalf("expected quality 42, got %d", info.Quality)
	}
}

func TestObjectFactorySerialize_SavesQualityFromEntityInfo(t *testing.T) {
	world := ecs.NewWorldForTesting()
	factory := &ObjectFactory{}

	entityID := types.EntityID(2001)
	handle := world.Spawn(entityID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, ecs.ExternalID{ID: entityID})
		ecs.AddComponent(w, h, components.EntityInfo{
			TypeID:   9909,
			IsStatic: true,
			Quality:  77,
			Region:   1,
			Layer:    0,
		})
		ecs.AddComponent(w, h, components.Transform{X: 5, Y: 6})
		ecs.AddComponent(w, h, components.ChunkRef{
			CurrentChunkX: 0,
			CurrentChunkY: 0,
			PrevChunkX:    0,
			PrevChunkY:    0,
		})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
	})

	raw, err := factory.Serialize(world, handle)
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}
	if raw == nil {
		t.Fatalf("expected serialized object")
	}
	if raw.Quality != 77 {
		t.Fatalf("expected serialized quality 77, got %d", raw.Quality)
	}
}

func TestObjectFactorySerialize_SkipsEmptyBuildObject(t *testing.T) {
	previousRegistry := objectdefs.Global()
	t.Cleanup(func() {
		objectdefs.SetGlobalForTesting(previousRegistry)
	})
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 1, Key: "player", Name: "Player", IsStatic: false},
	}))

	world := ecs.NewWorldForTesting()
	factory := &ObjectFactory{}

	entityID := types.EntityID(2002)
	handle := world.Spawn(entityID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, ecs.ExternalID{ID: entityID})
		ecs.AddComponent(w, h, components.EntityInfo{
			TypeID:   constt.BuildObjectTypeID,
			IsStatic: true,
			Region:   1,
			Layer:    0,
		})
		ecs.AddComponent(w, h, components.Transform{X: 7, Y: 8})
		ecs.AddComponent(w, h, components.ChunkRef{
			CurrentChunkX: 0,
			CurrentChunkY: 0,
			PrevChunkX:    0,
			PrevChunkY:    0,
		})
		state := components.ObjectInternalState{}
		components.SetBehaviorState(&state, "build", &components.BuildBehaviorState{
			BuildKey: "campfire",
			Items: []components.BuildRequiredItemState{
				{
					Slot:          0,
					ItemKey:       "stone",
					RequiredCount: 4,
					BuildCount:    0,
					PutItems:      nil,
				},
			},
		})
		ecs.AddComponent(w, h, state)
	})

	raw, err := factory.Serialize(world, handle)
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}
	if raw != nil {
		t.Fatalf("expected empty build object to be skipped from persistence")
	}
}
