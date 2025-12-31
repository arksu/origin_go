-- name: TruncateObjects :exec
TRUNCATE TABLE object;

-- name: GetObjectsByChunk :many
SELECT *
FROM object
WHERE region = $1
  AND chunk_x = $2
  AND chunk_y = $3
  AND layer = $4;

-- name: GetObjectByID :one
SELECT *
FROM object
WHERE id = $1;

-- name: SaveObject :exec
INSERT INTO object (id, region, x, y, layer, heading, chunk_x, chunk_y, type_id, quality, hp, create_tick, last_tick, data_hex)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (id) DO UPDATE SET
    region = EXCLUDED.region,
    x = EXCLUDED.x,
    y = EXCLUDED.y,
    layer = EXCLUDED.layer,
    heading = EXCLUDED.heading,
    chunk_x = EXCLUDED.chunk_x,
    chunk_y = EXCLUDED.chunk_y,
    type_id = EXCLUDED.type_id,
    quality = EXCLUDED.quality,
    hp = EXCLUDED.hp,
    last_tick = EXCLUDED.last_tick,
    data_hex = EXCLUDED.data_hex;

-- name: DeleteObject :exec
DELETE FROM object WHERE id = $1;
