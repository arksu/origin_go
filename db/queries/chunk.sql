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

-- name: SaveChunk :exec
insert into chunk (region, x, y, layer, last_tick, data)
values ($1, $2, $3, $4, $5, $6);

-- name: TruncateChunks :exec
TRUNCATE TABLE chunk;
