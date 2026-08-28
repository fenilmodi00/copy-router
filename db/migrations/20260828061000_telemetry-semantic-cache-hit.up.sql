BEGIN;

ALTER TABLE router.model_router_request_telemetry
    ADD COLUMN semantic_cache_hit BOOLEAN;

COMMENT ON COLUMN router.model_router_request_telemetry.semantic_cache_hit IS
    'True when the turn was served from the router semantic response cache (x-router-cache: hit). Distinct from upstream prompt-cache token counters.';

COMMIT;
