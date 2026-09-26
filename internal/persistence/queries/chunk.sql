-- name: GetChunk :one
SELECT *
FROM chunk
WHERE region = $1
  AND x = $2
  AND y = $3
  AND layer = $4;

-- name: GetChunksByRegion :many
SELECT *
FROM chunk
WHERE region = $1;

-- name: UpsertChunk :execrows
INSERT INTO chunk (region, x, y, layer, tiles_data, last_tick, entity_count, version, last_saved_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
ON CONFLICT (region, x, y, layer) DO UPDATE SET
    tiles_data = EXCLUDED.tiles_data,
    last_tick = EXCLUDED.last_tick,
    entity_count = EXCLUDED.entity_count,
    version = EXCLUDED.version,
    last_saved_at = NOW()
WHERE EXCLUDED.version >= chunk.version;

-- name: TruncateChunks :exec
TRUNCATE TABLE chunk;

-- name: DeleteChunksByRegion :exec
DELETE FROM chunk WHERE region = $1;
