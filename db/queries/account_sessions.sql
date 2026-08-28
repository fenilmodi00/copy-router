-- Mints a new session row for an account. token_hash is the SHA-256 of the
-- opaque cookie value; token_prefix/token_suffix are the safe 8+4 display parts.
-- installation_id is the tenant row resolved at login.
-- name: InsertAccountSession :one
INSERT INTO router.account_sessions (
    account_id,
    installation_id,
    token_hash,
    token_prefix,
    token_suffix,
    expires_at,
    ip_at_issue
)
VALUES (
    @account_id::varchar,
    sqlc.narg('installation_id')::varchar,
    @token_hash::varchar,
    @token_prefix::varchar,
    @token_suffix::varchar,
    @expires_at::timestamptz,
    sqlc.narg('ip_at_issue')::inet
)
RETURNING *;

-- Active-session lookup by token hash. Returns a row only while the session is
-- live (not revoked, not expired).
-- name: GetActiveAccountSessionByHash :one
SELECT *
FROM router.account_sessions
WHERE token_hash = @token_hash::varchar
  AND revoked_at IS NULL
  AND expires_at > NOW();

-- Bump last_seen_at when the middleware validates a live session (cheap write,
-- throttled to once per request).
-- name: TouchAccountSessionLastSeen :execrows
UPDATE router.account_sessions
SET last_seen_at = NOW()
WHERE account_id = @account_id::varchar
  AND id = @id::uuid;

-- Revoke one session (logout). Scoped to the account to prevent cross-account
-- revocation.
-- name: RevokeAccountSessionByID :execrows
UPDATE router.account_sessions
SET revoked_at = NOW()
WHERE account_id = @account_id::varchar
  AND id = @id::uuid
  AND revoked_at IS NULL;

-- Revoke every session for an account ("logout everywhere"; also called during
-- account wipe).
-- name: RevokeAllAccountSessionsForAccount :execrows
UPDATE router.account_sessions
SET revoked_at = NOW()
WHERE account_id = @account_id::varchar
  AND revoked_at IS NULL;

-- Session index for "my sessions" dashboard UI.
-- name: ListAccountSessionsForAccount :many
SELECT *
FROM router.account_sessions
WHERE account_id = @account_id::varchar
ORDER BY issued_at DESC;
