-- name: GetAccountByLogin :one
SELECT *
FROM account
WHERE login = $1;

-- name: CreateAccount :one
INSERT INTO account (login, password_hash)
VALUES ($1, $2)
RETURNING *;

-- name: GetAccountByToken :one
SELECT *
FROM account
WHERE token = $1;

-- name: UpdateAccountToken :exec
UPDATE account
SET token = $1, last_logged_at = now(), updated_at = now()
WHERE id = $2;

-- name: CountCharactersByAccountID :one
SELECT COUNT(*)
FROM character
WHERE account_id = $1
  AND deleted_at IS NULL;

-- name: GetAllAccounts :many
SELECT *
FROM account;
