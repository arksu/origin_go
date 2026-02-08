-- name: GetInventoriesByOwner :many
SELECT *
FROM inventory
WHERE owner_id = $1
ORDER BY kind, inventory_key;

-- name: UpsertInventory :one
INSERT INTO inventory (owner_id, kind, inventory_key, data, version)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (owner_id, kind, inventory_key)
DO UPDATE SET
    data = EXCLUDED.data,
    version = EXCLUDED.version,
    updated_at = now()
RETURNING *;

-- name: UpsertInventories :exec
INSERT INTO inventory (owner_id, kind, inventory_key, data, version)
SELECT
    unnest(sqlc.arg(owner_ids)::bigint[]),
    unnest(sqlc.arg(kinds)::int[]),
    unnest(sqlc.arg(inventory_keys)::int[]),
    unnest(sqlc.arg(datas)::text[])::jsonb,
    unnest(sqlc.arg(versions)::int[])
ON CONFLICT (owner_id, kind, inventory_key)
DO UPDATE SET
    data = EXCLUDED.data,
    version = EXCLUDED.version,
    updated_at = now();

-- name: UpdateInventory :exec
UPDATE inventory
SET data = $2, version = $3, updated_at = now()
WHERE owner_id = $1 AND kind = $4 AND inventory_key = $5 AND version = $6;

-- name: DeleteInventory :exec
DELETE FROM inventory
WHERE owner_id = $1 AND kind = $2 AND inventory_key = $3;
