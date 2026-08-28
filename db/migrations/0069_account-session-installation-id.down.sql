BEGIN;

ALTER TABLE router.account_sessions
  DROP COLUMN installation_id;

COMMIT;
