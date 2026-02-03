-- name: GetInventoriesByOwner :many
SELECT *
FROM inventory
WHERE owner_id = $1
ORDER BY kind, inventory_key;

-- name: UpsertInventory :one
INSERT INTO inventory (owner_id, kind, inventory_key, width, height, data, version)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (owner_id, kind, inventory_key)
DO UPDATE SET
    width = EXCLUDED.width,
    height = EXCLUDED.height,
    data = EXCLUDED.data,
    version = EXCLUDED.version,
    updated_at = now()
RETURNING *;

-- name: UpdateInventory :exec
UPDATE inventory
SET data = $2, version = $3, updated_at = now()
WHERE owner_id = $1 AND kind = $4 AND inventory_key = $5 AND version = $6;

-- name: DeleteInventory :exec
DELETE FROM inventory
WHERE owner_id = $1 AND kind = $2 AND inventory_key = $3;
