BEGIN;

-- Reverse the baseline schema. Drop order: view, then children, then
-- parents. Indexes go with their tables.
--
-- Do not DROP SCHEMA router. golang-migrate's schema_migrations table
-- lives here (search_path=router on the migrate URL) and is updated
-- after this file runs.

DROP VIEW router.production_request_telemetry;

DROP TABLE router.cluster_model_lists;
DROP TABLE router.model_router_user_cluster_model_lists;
DROP TABLE router.account_sessions;
DROP TABLE router.loop_escalation_events;
DROP TABLE router.model_router_request_telemetry;
DROP TABLE router.model_router_external_api_keys;
DROP TABLE router.model_router_api_keys;
DROP TABLE router.model_router_users;
DROP TABLE router.policy_shadow_decisions;
DROP TABLE router.request_feedback;
DROP TABLE router.router_feedback;
DROP TABLE router.session_pins;
DROP TABLE router.session_strategy_preferences;
DROP TABLE router.spiral_shadow_events;
DROP TABLE router.struggle_escalation_events;
DROP TABLE router.struggle_shadow_events;
DROP TABLE router.model_router_installations;
DROP TABLE router.accounts;
DROP TABLE router.flag_definitions;

COMMIT;
