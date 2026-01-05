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

-- name: UpsertChunk :exec
INSERT INTO chunk (region, x, y, layer, tiles_data, last_tick, entity_count, last_saved_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
ON CONFLICT (region, x, y, layer) DO UPDATE SET
    tiles_data = EXCLUDED.tiles_data,
    last_tick = EXCLUDED.last_tick,
    entity_count = EXCLUDED.entity_count,
    version = chunk.version + 1,
    last_saved_at = NOW();

-- name: TruncateChunks :exec
TRUNCATE TABLE chunk;

-- name: DeleteChunksByRegion :exec
DELETE FROM chunk WHERE region = $1;
