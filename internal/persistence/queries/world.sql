-- Reset every world table together so restrictive world-object references are
-- removed atomically. Account and character rows and their sequences survive.
-- name: TruncateWorld :exec
TRUNCATE TABLE character_hearth, inventory, build_site, skill, chat, spot, object, chunk, global_var;

-- Deleted characters still reserve their IDs in the shared entity namespace.
-- name: GetMaxCharacterID :one
SELECT GREATEST(COALESCE(MAX(id), 0), 0)::bigint FROM character;
