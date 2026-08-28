BEGIN;

ALTER TABLE router.model_router_request_telemetry
    DROP COLUMN semantic_cache_hit;

COMMIT;
