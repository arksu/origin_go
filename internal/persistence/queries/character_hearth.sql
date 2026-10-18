-- name: GetCharacterHearth :one
SELECT h.character_id, h.object_region, h.object_id, o.x, o.y, o.layer, c.name AS owner_name,
	c.last_ping_at AS owner_last_ping_at
FROM character_hearth AS h
JOIN character AS c ON c.id = h.character_id
JOIN object AS o ON o.region = h.object_region AND o.id = h.object_id
WHERE h.character_id = sqlc.arg(character_id)
	AND c.deleted_at IS NULL
	AND o.deleted_at IS NULL
	AND o.type_id = sqlc.arg(hearth_type_id)
	AND o.layer = sqlc.arg(surface_layer);

-- name: GetCharacterHearthOwners :many
SELECT h.character_id, h.object_region, h.object_id, o.x, o.y, o.layer, c.name AS owner_name,
	c.last_ping_at AS owner_last_ping_at
FROM character_hearth AS h
JOIN character AS c ON c.id = h.character_id
JOIN object AS o ON o.region = h.object_region AND o.id = h.object_id
WHERE c.deleted_at IS NULL
	AND o.deleted_at IS NULL
	AND o.type_id = sqlc.arg(hearth_type_id)
	AND o.layer = sqlc.arg(surface_layer)
	AND (h.object_region, h.object_id) IN (
		SELECT
			unnest(sqlc.arg(object_regions)::int[]),
			unnest(sqlc.arg(object_ids)::bigint[])
	);

-- Replacement also needs the old association when its owner is soft-deleted.
-- name: GetSavedCharacterHearth :one
SELECT h.character_id, h.object_region, h.object_id, o.type_id, o.x, o.y, o.layer, o.deleted_at
FROM character_hearth AS h
LEFT JOIN object AS o ON o.region = h.object_region AND o.id = h.object_id
WHERE h.character_id = sqlc.arg(character_id);

-- Lock the character row so a concurrent soft delete cannot acquire a home.
-- name: SetCharacterHearth :execrows
WITH eligible_character AS (
	SELECT c.id
	FROM character AS c
	WHERE c.id = sqlc.arg(character_id) AND c.deleted_at IS NULL
	FOR UPDATE
)
INSERT INTO character_hearth (character_id, object_region, object_id)
SELECT c.id, o.region, o.id
FROM eligible_character AS c
JOIN object AS o ON o.region = sqlc.arg(object_region) AND o.id = sqlc.arg(object_id)
WHERE o.deleted_at IS NULL
	AND o.type_id = sqlc.arg(hearth_type_id)
	AND o.layer = sqlc.arg(surface_layer)
ON CONFLICT (character_id) DO UPDATE
SET object_region = EXCLUDED.object_region,
	object_id = EXCLUDED.object_id;

-- name: RemoveCharacterHearth :execrows
DELETE FROM character_hearth
WHERE character_id = sqlc.arg(character_id)
	AND object_region = sqlc.arg(object_region)
	AND object_id = sqlc.arg(object_id);

-- Repair only the observed association and only while it is still invalid.
-- name: DeleteInvalidCharacterHearth :execrows
DELETE FROM character_hearth AS h
WHERE h.character_id = sqlc.arg(character_id)
	AND h.object_region = sqlc.arg(object_region)
	AND h.object_id = sqlc.arg(object_id)
	AND NOT EXISTS (
		SELECT 1
		FROM character AS c
		JOIN object AS o ON o.region = h.object_region AND o.id = h.object_id
		WHERE c.id = h.character_id
			AND c.deleted_at IS NULL
			AND o.deleted_at IS NULL
			AND o.type_id = sqlc.arg(hearth_type_id)
			AND o.layer = sqlc.arg(surface_layer)
	);
