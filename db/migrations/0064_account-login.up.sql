BEGIN;

-- Self-service login: a user presents an aiand (sk-) API key; we validate it
-- against aiand's GET /api/v1/me, then we create ONE router installation per
-- aiand user (tenant = installation), keyed on aiand_user_id.
--
-- router.accounts is the login binding; the ACTUAL tenant is a single row of
-- the existing router.model_router_installations table, which owns ALL
-- per-installation data (rk_ keys, BYOK secrets, metrics scoping, billing
-- org_id, allowed/excluded models). The account id (a varchar column, an
-- acct_ prefix id generated in Go) is stored AS that installation's external_id, so the binding is 1:1
-- and re-hydration mirrors the proven EnsureAdminInstallation create-or-relist
-- pattern. There is NO FK column to model_router_installations: aiand_user_id
-- / aiand_organization_id are opaque external strings (never FK), and the
-- account-id-as-external_id convention makes a column redundant.
--
-- Data-retention contract: the aiand API reference exposes NO endpoint to
-- retrieve a user's data or re-instantiate an account from a revoked key. When
-- the user revokes the key, their router install + data are wiped (account row
-- soft-deleted). This is intentional and matches the user's stated design.
CREATE TABLE router.accounts (
  id                    VARCHAR(36) PRIMARY KEY,
  aiand_user_id         VARCHAR(128) NOT NULL,
  aiand_organization_id VARCHAR(128) NOT NULL,
  display_name          VARCHAR(255),
  created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_login_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at            TIMESTAMPTZ
);

-- Login lookup: "find my account by aiand identity, only if active."
CREATE UNIQUE INDEX accounts_aiand_user_id_active_idx
  ON router.accounts(aiand_user_id)
  WHERE deleted_at IS NULL;

-- Future "list accounts in one aiand org" admin view. Cheap to add now.
CREATE INDEX accounts_aiand_org_idx
  ON router.accounts(aiand_organization_id)
  WHERE deleted_at IS NULL;

COMMENT ON TABLE router.accounts IS
  'Self-service login: aiand identity mapped to the router installation that owns all tenant data. Account id doubles as the installation external_id (no FK; aiand ids are opaque external strings).';
COMMENT ON COLUMN router.accounts.deleted_at IS
  'Soft-delete on key revocation / account wipe. NULL = active.';

-- Dashboard sessions: opaque random tokens stored SHA-256-hashed, never
-- recoverable from the DB. Revocation is a ROW UPDATE (revoked_at = NOW()),
-- the same shape as admin sessions — deliberately not JWT.
CREATE TABLE router.account_sessions (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id   VARCHAR(36) NOT NULL REFERENCES router.accounts(id) ON DELETE CASCADE,
  token_hash   VARCHAR(64) NOT NULL,
  token_prefix VARCHAR(16) NOT NULL,
  token_suffix VARCHAR(16) NOT NULL,
  issued_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_at   TIMESTAMPTZ NOT NULL,
  revoked_at   TIMESTAMPTZ,
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ip_at_issue  INET
);

-- Lookup by token hash (constant per session; one row per token).
CREATE UNIQUE INDEX account_sessions_token_hash_unique
  ON router.account_sessions(token_hash);

-- "logout everywhere" + per-account session list.
CREATE INDEX account_sessions_account_id_issued_at_idx
  ON router.account_sessions(account_id, issued_at DESC);

COMMENT ON TABLE router.account_sessions IS
  'Revocable dashboard sessions for self-service accounts. token_hash is the SHA-256 of the opaque cookie value; token_prefix/suffix are the safe 8+4 display parts.';
COMMENT ON COLUMN router.account_sessions.revoked_at IS
  'Set on logout / account wipe. NULL = still active.';

COMMIT;
