package game

import (
	"context"
	"log"
	"origin/internal/db"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"sync"
)

// ObjectLoader manages loading and unloading of world objects from the database
type ObjectLoader struct {
	queries *db.Queries

	// loadedObjects tracks which objects are currently loaded in ECS
	// Maps object ID -> ECS Handle
	loadedObjects map[int64]ecs.Handle

	// chunkObjects tracks which objects belong to which chunk
	// Maps chunk key -> set of object IDs
	chunkObjects map[uint64]map[int64]struct{}

	mu sync.RWMutex
}

// NewObjectLoader creates a new object loader
func NewObjectLoader(queries *db.Queries) *ObjectLoader {
	return &ObjectLoader{
		queries:       queries,
		loadedObjects: make(map[int64]ecs.Handle, 1024),
		chunkObjects:  make(map[uint64]map[int64]struct{}, 256),
	}
}

// LoadChunkObjects loads all objects for a chunk from the database and spawns them in ECS
func (ol *ObjectLoader) LoadChunkObjects(ctx context.Context, w *ecs.World, region, chunkX, chunkY, layer int32) error {
	chunkKey := components.ChunkKeyFromCoords(region, layer, chunkX, chunkY)

	ol.mu.RLock()
	_, alreadyLoaded := ol.chunkObjects[chunkKey]
	ol.mu.RUnlock()

	if alreadyLoaded {
		return nil // Already loaded
	}

	// Load objects from database
	objects, err := ol.queries.GetObjectsByChunk(ctx, db.GetObjectsByChunkParams{
		Region: region,
		GridX:  chunkX,
		GridY:  chunkY,
		Layer:  layer,
	})
	if err != nil {
		return err
	}

	ol.mu.Lock()
	defer ol.mu.Unlock()

	// Initialize chunk object set
	if ol.chunkObjects[chunkKey] == nil {
		ol.chunkObjects[chunkKey] = make(map[int64]struct{}, len(objects))
	}

	chunkIndex := ecs.GetChunkIndex(w)

	for _, obj := range objects {
		// Skip if already loaded
		if _, exists := ol.loadedObjects[obj.ID]; exists {
			continue
		}

		// Spawn entity in ECS
		h := w.Spawn(ecs.EntityID(obj.ID))
		if !h.IsValid() {
			log.Printf("ObjectLoader: failed to spawn object %d", obj.ID)
			continue
		}

		// Add Position component
		ecs.AddComponent(w, h, components.Position{
			X: float64(obj.X),
			Y: float64(obj.Y),
		})

		// Add WorldObject component
		data := ""
		if obj.Data.Valid {
			data = obj.Data.String
		}
		ecs.AddComponent(w, h, components.WorldObject{
			ObjectType: obj.Type,
			Quality:    obj.Quality,
			HP:         obj.Hp,
			Heading:    obj.Heading,
			CreateTick: obj.CreateTick,
			LastTick:   obj.LastTick,
			Data:       data,
		})

		// Add EntityMeta for visibility
		ecs.AddComponent(w, h, components.EntityMeta{
			EntityID:   ecs.EntityID(obj.ID),
			EntityType: uint32(obj.Type),
		})

		// Add Static marker (world objects don't move)
		ecs.AddComponent(w, h, components.Static{})

		// Add Collider based on object type (default size, can be customized per type)
		colliderSize := getObjectColliderSize(obj.Type)
		if colliderSize > 0 {
			ecs.AddComponent(w, h, components.Collider{
				Width:  colliderSize,
				Height: colliderSize,
				Layer:  1, // Default collision layer
				Mask:   0xFF,
			})
		}

		// Add ChunkRef and register in chunk index
		chunkRef := systems.ChunkRefFromPosition(region, layer, float64(obj.X), float64(obj.Y))
		ecs.AddComponent(w, h, chunkRef)
		if chunkIndex != nil {
			chunkIndex.Add(h, chunkRef.Key())
		}

		// Track loaded object
		ol.loadedObjects[obj.ID] = h
		ol.chunkObjects[chunkKey][obj.ID] = struct{}{}
	}

	log.Printf("ObjectLoader: loaded %d objects for chunk (%d, %d, %d, %d)", len(objects), region, chunkX, chunkY, layer)
	return nil
}

// UnloadChunkObjects removes all objects for a chunk from ECS
func (ol *ObjectLoader) UnloadChunkObjects(w *ecs.World, region, chunkX, chunkY, layer int32) {
	chunkKey := components.ChunkKeyFromCoords(region, layer, chunkX, chunkY)

	ol.mu.Lock()
	defer ol.mu.Unlock()

	objectIDs, exists := ol.chunkObjects[chunkKey]
	if !exists {
		return
	}

	chunkIndex := ecs.GetChunkIndex(w)

	for objID := range objectIDs {
		h, ok := ol.loadedObjects[objID]
		if !ok {
			continue
		}

		// Remove from chunk index
		if chunkIndex != nil {
			chunkIndex.Remove(h)
		}

		// Despawn entity
		w.Despawn(h)

		delete(ol.loadedObjects, objID)
	}

	delete(ol.chunkObjects, chunkKey)
	log.Printf("ObjectLoader: unloaded objects for chunk (%d, %d, %d, %d)", region, chunkX, chunkY, layer)
}

// IsObjectLoaded checks if an object is currently loaded
func (ol *ObjectLoader) IsObjectLoaded(objectID int64) bool {
	ol.mu.RLock()
	defer ol.mu.RUnlock()
	_, exists := ol.loadedObjects[objectID]
	return exists
}

// GetObjectHandle returns the ECS handle for a loaded object
func (ol *ObjectLoader) GetObjectHandle(objectID int64) (ecs.Handle, bool) {
	ol.mu.RLock()
	defer ol.mu.RUnlock()
	h, exists := ol.loadedObjects[objectID]
	return h, exists
}

// IsChunkLoaded checks if a chunk's objects are loaded
func (ol *ObjectLoader) IsChunkLoaded(region, chunkX, chunkY, layer int32) bool {
	chunkKey := components.ChunkKeyFromCoords(region, layer, chunkX, chunkY)
	ol.mu.RLock()
	defer ol.mu.RUnlock()
	_, exists := ol.chunkObjects[chunkKey]
	return exists
}

// LoadedObjectCount returns the number of currently loaded objects
func (ol *ObjectLoader) LoadedObjectCount() int {
	ol.mu.RLock()
	defer ol.mu.RUnlock()
	return len(ol.loadedObjects)
}

// getObjectColliderSize returns the collider size for an object type
// Returns 0 if the object should not have a collider
func getObjectColliderSize(objectType int32) float64 {
	// TODO: implement object type -> collider size mapping
	// For now, use a default size for all objects
	switch objectType {
	case 0:
		return 0 // No collider
	default:
		return 24.0 // Default 2-tile collider (2 * COORD_PER_TILE)
	}
}

// ObjectLoaderKey is the resource key for ObjectLoader in World
type ObjectLoaderKey struct{}

// GetObjectLoader retrieves ObjectLoader from world resources
func GetObjectLoader(w *ecs.World) *ObjectLoader {
	if res, ok := w.GetResource(ObjectLoaderKey{}); ok {
		return res.(*ObjectLoader)
	}
	return nil
}

// SetObjectLoader stores ObjectLoader in world resources
func SetObjectLoader(w *ecs.World, ol *ObjectLoader) {
	w.SetResource(ObjectLoaderKey{}, ol)
}
