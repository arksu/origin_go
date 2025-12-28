-- name: GetInventoryItem :one
SELECT *
FROM inventory
WHERE id = $1
  AND deleted = false;

-- name: GetInventoryByParentID :many
SELECT *
FROM inventory
WHERE parent_id = $1
  AND deleted = false;
