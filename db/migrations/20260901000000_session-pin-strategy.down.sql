BEGIN;

ALTER TABLE router.session_pins
    DROP COLUMN routing_strategy;

COMMIT;
