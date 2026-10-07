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
  AND deleted_at IS NULL
FOR UPDATE;

-- name: ClearAuthToken :exec
UPDATE character
SET auth_token = NULL
WHERE id = $1;

-- name: UpdateCharacterPosition :exec
UPDATE character
SET x = $2, y = $3
WHERE id = $1;

-- name: UpdateCharacterPositionAndLayer :exec
UPDATE character
SET x = $2,
    y = $3,
    layer = $4
WHERE id = $1
  AND deleted_at IS NULL;

-- name: CreateCharacter :one
INSERT INTO character (id, account_id, name, region, x, y, layer, heading, stamina, energy, shp, hhp, attributes, exp, skills,
                       discovery)
VALUES ($1, $2, $3, 1, $4, $5, 0, 0, sqlc.arg(stamina), sqlc.arg(energy), sqlc.arg(shp), sqlc.arg(hhp), sqlc.arg(attributes)::jsonb,
        sqlc.arg(exp)::jsonb, sqlc.arg(skills)::jsonb, sqlc.arg(discovery)::jsonb)
RETURNING *;

-- name: DeleteCharacter :exec
UPDATE character
SET deleted_at = now()
WHERE id = $1
  AND account_id = $2
  AND deleted_at IS NULL;

-- name: DeleteCharacterByID :exec
UPDATE character
SET deleted_at = now()
WHERE id = $1
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
    is_lying = v.is_lying,
    attributes = v.attributes,
    exp = v.exp,
    skills = v.skills,
    discovery = v.discovery,
    action_cooldowns = v.action_cooldowns,
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
             unnest(sqlc.arg(shps)::float8[]) as shp,
             unnest(sqlc.arg(hhps)::float8[]) as hhp,
             unnest(sqlc.arg(is_lyings)::boolean[]) as is_lying,
             unnest(sqlc.arg(attributes)::text[])::jsonb as attributes,
             unnest(sqlc.arg(exps)::text[])::jsonb as exp,
             unnest(sqlc.arg(skills)::text[])::jsonb as skills,
             unnest(sqlc.arg(discovery)::text[])::jsonb as discovery,
             unnest(sqlc.arg(action_cooldowns)::text[])::jsonb as action_cooldowns
     ) AS v
WHERE character.id = v.id
  AND character.deleted_at IS NULL;
