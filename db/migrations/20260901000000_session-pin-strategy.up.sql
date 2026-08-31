BEGIN;

ALTER TABLE router.session_pins
    ADD COLUMN routing_strategy VARCHAR(32) NOT NULL DEFAULT '';

COMMIT;
