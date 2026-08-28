BEGIN;

-- Cache the account's installation on the session row so dashboard middleware
-- can resolve tenant scope in one read instead of EnsureAccountInstallation on
-- every /admin/v1 request. New sessions are populated at login; NULL rows fall
-- back to the repair path until the user re-authenticates.
ALTER TABLE router.account_sessions
  ADD COLUMN installation_id VARCHAR(36);

COMMENT ON COLUMN router.account_sessions.installation_id IS
  'Installation UUID resolved at login; avoids per-request EnsureAccountInstallation in dashboard middleware.';

COMMIT;
