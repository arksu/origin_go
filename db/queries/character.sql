-- name: GetCharacter :one
SELECT *
FROM character
WHERE id = $1
  AND deleted_at IS NULL;

-- name: GetCharactersByAccountID :many
SELECT *
FROM character
WHERE account_id = $1
  AND deleted_at IS NULL;

-- name: GetCharacterByToken :one
SELECT *
from character
where auth_token = $1;

-- name: ClearAuthToken :exec
UPDATE character
SET auth_token = NULL
WHERE id = $1;
