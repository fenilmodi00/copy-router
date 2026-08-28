BEGIN;

ALTER TABLE router.model_router_request_telemetry
    ADD COLUMN cache_input_savings_usd DOUBLE PRECISION;

COMMENT ON COLUMN router.model_router_request_telemetry.cache_input_savings_usd IS
    'Dollars saved on prompt-cache reads for this turn (catalog-priced). NULL when not computed at insert time.';

COMMIT;
