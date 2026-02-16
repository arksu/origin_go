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
INSERT INTO character (id, account_id, name, region, x, y, layer, heading, stamina, energy, shp, hhp, attributes)
VALUES ($1, $2, $3, 1, $4, $5, 0, 0, sqlc.arg(stamina), sqlc.arg(energy), 100, 100, sqlc.arg(attributes)::jsonb)
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

-- name: UpdateCharacterAttributes :exec
UPDATE character
SET attributes = sqlc.arg(attributes)::jsonb,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- name: ResetOnlinePlayers :exec
UPDATE character
SET is_online = false
WHERE region = $1
  AND is_online = true
  AND deleted_at IS NULL;

-- name: UpdateCharacters :exec
UPDATE character
SET
    x = v.x,
    y = v.y,
    heading = v.heading,
    stamina = v.stamina,
    energy = v.energy,
    shp = v.shp,
    hhp = v.hhp,
    attributes = v.attributes,
    last_save_at = now(),
    updated_at = now()
FROM (
         SELECT
             unnest(sqlc.arg(ids)::int[]) as id,
             unnest(sqlc.arg(xs)::float8[]) as x,
             unnest(sqlc.arg(ys)::float8[]) as y,
             unnest(sqlc.arg(headings)::float8[]) as heading,
             unnest(sqlc.arg(staminas)::float8[]) as stamina,
             unnest(sqlc.arg(energies)::float8[]) as energy,
             unnest(sqlc.arg(shps)::int[]) as shp,
             unnest(sqlc.arg(hhps)::int[]) as hhp,
             unnest(sqlc.arg(attributes)::text[])::jsonb as attributes
     ) AS v
WHERE character.id = v.id;
