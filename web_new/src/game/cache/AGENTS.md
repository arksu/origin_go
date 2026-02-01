# Chunk Cache System

## Overview

LRU cache with TTL for chunk data to eliminate freezes when moving between chunks. Implements deferred border refresh to avoid rebuild cascades.

## Architecture

```
ChunkManager
    ├─ ChunkCache (LRU + TTL)
    │   └─ CachedChunk[] (tiles, CPU geometry, GPU resources)
    ├─ BuildQueue (priority queue with time budget)
    │   └─ BuildTask[] (sorted by priority, distance)
    └─ CacheMetricsCollector (debug stats)
```

## Key Components

### ChunkCache

LRU cache storing chunk data at multiple levels:
- **Tiles**: Raw `Uint8Array` from server
- **CPU geometry**: Vertex buffers (positions, uvs, indices)
- **GPU resources**: MeshGeometry, Containers (optional)

**Configuration** (`constants.ts`):
| Constant | Default | Description |
|----------|---------|-------------|
| `CACHE_MAX_ENTRIES` | 32 | Maximum cached chunks |
| `CACHE_TTL_MS` | 120000 | Time-to-live (2 minutes) |
| `CACHE_SWEEP_INTERVAL_MS` | 10000 | TTL cleanup interval |

**Eviction Policy**:
1. LRU eviction when at capacity
2. TTL eviction on periodic sweep
3. Version mismatch invalidation

### BuildQueue

Priority queue for chunk build tasks with frame budget.

**Priorities**:
| Priority | Name | Description |
|----------|------|-------------|
| P0 | VISIBLE | Chunks within 1.5 distance - build immediately |
| P1 | NEARBY | Chunks within 2.5 distance - background build |
| P2 | DISTANT | Far chunks - build on idle only |

**Configuration**:
| Constant | Default | Description |
|----------|---------|-------------|
| `BUILD_TIME_BUDGET_MS` | 2 | Max ms per frame for builds |
| `BUILD_QUEUE_MAX_LENGTH` | 64 | Max queued tasks |
| `MAX_IN_FLIGHT_BUILDS` | 2 | Concurrent builds |

**Features**:
- Deduplication by chunk key
- Task cancellation via `buildToken`
- Adaptive budget based on frame time

### Deferred Border Refresh

Instead of rebuilding all 8 neighbors immediately when a chunk loads:

1. Mark neighbors as `needsBorderRefresh = true`
2. Debounce refresh requests (`BORDER_REFRESH_DELAY_MS`)
3. Enqueue as P1 priority tasks
4. Process within frame budget

This prevents cascade rebuilds that cause freezes.

## Data Flow

### Chunk Load (Cache Miss)
```
Server → ChunkLoad(x, y, tiles, version)
    ↓
ChunkManager.loadChunk()
    ↓ cache miss
BuildQueue.enqueue(task, priority)
    ↓ P0: immediate, P1/P2: queued
processBuildTask()
    ↓
Chunk.buildTiles() → GPU upload
    ↓
ChunkCache.set(cachedChunk)
    ↓
notifyNeighborsOfLoad() → scheduleBorderRefresh()
```

### Chunk Load (Cache Hit)
```
Server → ChunkLoad(x, y, tiles, version)
    ↓
ChunkManager.loadChunk()
    ↓ cache hit (version match)
attachFromCache()
    ↓
Reuse GPU resources or rebuild from CPU cache
    ↓
No neighbor rebuild cascade
```

### Chunk Unload
```
Server → ChunkUnload(x, y)
    ↓
ChunkManager.unloadChunk()
    ↓
chunk.visible = false
    ↓
Keep in cache (don't destroy GPU)
    ↓
Eviction only on LRU/TTL/capacity
```

## Metrics

Available via `cacheMetrics.getMetrics()`:

**Cache Stats**:
- `entries`, `hits`, `misses`, `hitRate`
- `bytesTotal`, `bytesCpu`, `bytesGpu`, `bytesTiles`

**Eviction Stats**:
- `evictionsLru`, `evictionsTtl`, `evictionsVersionMismatch`

**Build Stats**:
- `buildQueueLength`, `canceledBuilds`
- `cpuBuildMsAvg`, `gpuUploadMsAvg`

**Border Refresh Stats**:
- `borderRefreshCount`, `borderRefreshMsAvg`

Debug overlay shows cache section when enabled (press \` key).

## Files

| File | Purpose |
|------|---------|
| `constants.ts` | Configuration constants |
| `types.ts` | TypeScript interfaces |
| `ChunkCache.ts` | LRU cache implementation |
| `BuildQueue.ts` | Priority queue with budget |
| `CacheMetricsCollector.ts` | Metrics aggregation |
| `index.ts` | Module exports |

## Version Handling

Server sends `version` with each `ChunkLoad`. Cache validates:
- If `cached.version === incoming.version` → cache hit
- If version mismatch → invalidate and rebuild

This ensures visual consistency when chunk data changes.

## Future Improvements (Stages 2-3)

### Stage 2: GPU Cache (Level C)
- Store `MeshGeometry` references in cache
- Transfer ownership on hide/show
- Separate GPU memory limits

### Stage 3: Web Worker
- Move CPU geometry build to worker
- Main thread only handles GPU upload
- Message passing with `buildToken` validation
