-- name: TruncateObjects :exec
TRUNCATE TABLE object;

-- name: GetObjectsByChunk :many
SELECT *
FROM object
WHERE region = $1
  AND chunk_x = $2
  AND chunk_y = $3
  AND layer = $4
  AND deleted_at IS NULL;

-- name: GetObjectByID :one
SELECT *
FROM object
WHERE id = $1
  AND deleted_at IS NULL;

-- name: DeleteObject :exec
DELETE
FROM object
WHERE id = $1;

-- name: UpsertObject :exec
INSERT INTO object (
    id, object_type, region, x, y, layer, chunk_x, chunk_y,
    heading, quality, hp_current, hp_max, is_static, owner_id,
    data_jsonb, create_tick, last_tick, created_at, updated_at
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    $9, $10, $11, $12, $13, $14,
    $15, $16, $17, NOW(), NOW()
)
ON CONFLICT (region, id) DO UPDATE SET
    object_type = EXCLUDED.object_type,
    x = EXCLUDED.x,
    y = EXCLUDED.y,
    layer = EXCLUDED.layer,
    chunk_x = EXCLUDED.chunk_x,
    chunk_y = EXCLUDED.chunk_y,
    heading = EXCLUDED.heading,
    quality = EXCLUDED.quality,
    hp_current = EXCLUDED.hp_current,
    hp_max = EXCLUDED.hp_max,
    is_static = EXCLUDED.is_static,
    owner_id = EXCLUDED.owner_id,
    data_jsonb = EXCLUDED.data_jsonb,
    last_tick = EXCLUDED.last_tick,
    updated_at = NOW();

-- name: SoftDeleteObject :exec
UPDATE object
SET deleted_at = NOW()
WHERE region = $1 AND id = $2;

-- name: DeleteObjectsByChunk :exec
UPDATE object
SET deleted_at = NOW()
WHERE region = $1
  AND chunk_x = $2
  AND chunk_y = $3
  AND layer = $4
  AND deleted_at IS NULL;
