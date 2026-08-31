-- Intentional no-op. The dropped credit-balance/ledger data is not
-- recreatable: it recorded prepaid balances and spend state that the router
-- no longer tracks. Recreating the tables would imply empty balances were
-- meaningful. Usage visibility survives via
-- router.model_router_request_telemetry, which this migration does not touch.
SELECT 1;
