-- name: CommitCropHarvestSource :one
UPDATE object
SET data = $3,
    deleted_at = NOW(),
    updated_at = NOW()
WHERE region = $1
  AND id = $2
  AND deleted_at IS NULL
RETURNING id;
