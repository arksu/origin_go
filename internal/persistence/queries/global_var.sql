-- name: GetGlobalVar :one
SELECT *
FROM global_var
WHERE name = $1;

-- name: UpsertGlobalVarLong :exec
INSERT INTO global_var (name, value_long)
VALUES ($1, $2)
ON CONFLICT (name) DO UPDATE SET value_long = EXCLUDED.value_long;
