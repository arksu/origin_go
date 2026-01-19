-- name: GetCharacter :one
SELECT *
FROM character
WHERE id = $1
  AND deleted_at IS NULL;

-- name: GetCharactersByAccountID :many
SELECT *
FROM character
WHERE account_id = $1
  AND deleted_at IS NULL
ORDER BY id;

-- name: GetCharacterByTokenForUpdate :one
SELECT *
from character
where auth_token = $1
FOR UPDATE;

-- name: ClearAuthToken :exec
UPDATE character
SET auth_token = NULL
WHERE id = $1;

-- name: UpdateCharacterPosition :exec
UPDATE character
SET x = $2, y = $3
WHERE id = $1;

-- name: CreateCharacter :one
INSERT INTO character (id, account_id, name, region, x, y, layer, heading, stamina, shp, hhp)
VALUES ($1, $2, $3, 1, $4, $5, 0, 0, 100, 100, 100)
RETURNING *;

-- name: DeleteCharacter :exec
UPDATE character
SET deleted_at = now()
WHERE id = $1
  AND account_id = $2
  AND deleted_at IS NULL;

-- name: SetCharacterAuthToken :exec
UPDATE character
SET auth_token = $2, token_expires_at = $3
WHERE id = $1
  AND account_id = $4
  AND deleted_at IS NULL;

-- name: SetCharacterOnline :exec
UPDATE character
SET is_online = true
WHERE id = $1
  AND is_online = false
  AND deleted_at IS NULL;

-- name: SetCharacterOffline :exec
UPDATE character
SET is_online = false
WHERE id = $1
  AND is_online = true
  AND deleted_at IS NULL;

-- name: ResetOnlinePlayers :exec
UPDATE character
SET is_online = false
WHERE region = $1
  AND is_online = true
  AND deleted_at IS NULL;

-- name: BatchUpdateCharacters :exec
UPDATE character
SET 
	x = $2,
	y = $3,
	heading = $4,
	stamina = $5,
	shp = $6,
	hhp = $7,
	last_save_at = now(),
	updated_at = now()
WHERE id = $1;
