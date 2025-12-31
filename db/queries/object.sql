-- name: GetObjectsByChunk :many
SELECT *
FROM object
WHERE region = $1
  AND grid_x = $2
  AND grid_y = $3
  AND layer = $4;

-- name: GetObjectByID :one
SELECT *
FROM object
WHERE id = $1;

-- name: SaveObject :exec
INSERT INTO object (id, region, x, y, layer, heading, grid_x, grid_y, type, quality, hp, create_tick, last_tick, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (id) DO UPDATE SET
    region = EXCLUDED.region,
    x = EXCLUDED.x,
    y = EXCLUDED.y,
    layer = EXCLUDED.layer,
    heading = EXCLUDED.heading,
    grid_x = EXCLUDED.grid_x,
    grid_y = EXCLUDED.grid_y,
    type = EXCLUDED.type,
    quality = EXCLUDED.quality,
    hp = EXCLUDED.hp,
    last_tick = EXCLUDED.last_tick,
    data = EXCLUDED.data;

-- name: DeleteObject :exec
DELETE FROM object WHERE id = $1;
