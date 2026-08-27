-- Upsert is a no-op when the aiand user already has an active account, and a
-- fresh INSERT (with the caller-generated id, which doubles as the
-- installation external_id) when they don't. On returning login it bumps
-- last_login_at, refreshes aiand_organization_id, and updates display_name
-- only if a new one was given (COALESCE so a login from a client that omits
-- display_name can't blank an existing value). Concurrency: two concurrent
-- first-logins for the same aiand user collide on the partial unique index;
-- the loser's INSERT is diverted to the DO UPDATE and returns the WINNER's row,
-- so both callers agree on one account id and, therefore, one installation.
-- name: UpsertAccount :one
INSERT INTO router.accounts (
    id,
    aiand_user_id,
    aiand_organization_id,
    display_name
)
VALUES (
    @id::varchar,
    @aiand_user_id::varchar,
    @aiand_organization_id::varchar,
    sqlc.narg('display_name')::varchar
)
ON CONFLICT (aiand_user_id) WHERE deleted_at IS NULL DO UPDATE SET
    last_login_at          = NOW(),
    aiand_organization_id  = EXCLUDED.aiand_organization_id,
    display_name           = COALESCE(EXCLUDED.display_name, router.accounts.display_name)
RETURNING *;

-- Login lookup, active only. Used when a returning user logs back in with a
-- (new) aiand key that resolves to the same aiand user id.
-- name: GetAccountByAiandUserID :one
SELECT *
FROM router.accounts
WHERE aiand_user_id = @aiand_user_id::varchar
  AND deleted_at IS NULL;

-- Session verification loads the account by its router-generated id.
-- name: GetAccountByID :one
SELECT *
FROM router.accounts
WHERE id = @id::varchar
  AND deleted_at IS NULL;

-- Soft-delete an account (wipe on key revocation). Keeps the row so audit
-- trails survive; the user re-presents a fresh aiand key to re-instantiate.
-- name: SoftDeleteAccount :exec
UPDATE router.accounts
SET deleted_at = NOW()
WHERE id = @id::varchar
  AND deleted_at IS NULL;
