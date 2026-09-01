-- Canonical router schema: the single source of truth for a FRESH install.
-- Folded mechanically from the retired migration chain (0001..0069 + dated
-- migrations through 20260902000000) by applying every step in order on an
-- empty database and dumping the terminal state.
--
-- Loaded three ways:
--   * docker-compose: mounted at /docker-entrypoint-initdb.d (first boot)
--   * `make initdb` / cmd/initdb: applied when the router schema is empty
--   * sqlc: parsed as the schema source for type inference (db/sqlc.yml)
--
-- Incremental changes after baseline live in db/migrations/ (0002+). Keep
-- this file and the migration pair in sync — see db/CLAUDE.md.

CREATE SCHEMA IF NOT EXISTS router;

--
-- PostgreSQL database dump
--


-- Dumped from database version 18.6 (Ubuntu 18.6-0ubuntu0.26.04.1)
-- Dumped by pg_dump version 18.6 (Ubuntu 18.6-0ubuntu0.26.04.1)




SET default_table_access_method = heap;

--
-- Name: account_sessions; Type: TABLE; Schema: router; Owner: -
--

CREATE TABLE router.account_sessions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    account_id character varying(36) NOT NULL,
    token_hash character varying(64) NOT NULL,
    token_prefix character varying(16) NOT NULL,
    token_suffix character varying(16) NOT NULL,
    issued_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    last_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    ip_at_issue inet,
    installation_id character varying(36)
);


--
-- Name: TABLE account_sessions; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON TABLE router.account_sessions IS 'Revocable dashboard sessions for self-service accounts. token_hash is the SHA-256 of the opaque cookie value; token_prefix/suffix are the safe 8+4 display parts.';


--
-- Name: COLUMN account_sessions.revoked_at; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.account_sessions.revoked_at IS 'Set on logout / account wipe. NULL = still active.';


--
-- Name: COLUMN account_sessions.installation_id; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.account_sessions.installation_id IS 'Installation UUID resolved at login; avoids per-request EnsureAccountInstallation in dashboard middleware.';


--
-- Name: accounts; Type: TABLE; Schema: router; Owner: -
--

CREATE TABLE router.accounts (
    id character varying(36) NOT NULL,
    aiand_user_id character varying(128) NOT NULL,
    aiand_organization_id character varying(128) NOT NULL,
    display_name character varying(255),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_login_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);


--
-- Name: TABLE accounts; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON TABLE router.accounts IS 'Self-service login: aiand identity mapped to the router installation that owns all tenant data. Account id doubles as the installation external_id (no FK; aiand ids are opaque external strings).';


--
-- Name: COLUMN accounts.deleted_at; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.accounts.deleted_at IS 'Soft-delete on key revocation / account wipe. NULL = active.';


--
-- Name: cluster_model_lists; Type: TABLE; Schema: router; Owner: -
--

CREATE TABLE router.cluster_model_lists (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id character varying(255) NOT NULL,
    api_key_id uuid NOT NULL,
    cluster_label character varying(128) NOT NULL,
    models text[] NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT cluster_model_lists_models_not_empty CHECK ((cardinality(models) > 0))
);


--
-- Name: flag_definitions; Type: TABLE; Schema: router; Owner: -
--

CREATE TABLE router.flag_definitions (
    key text NOT NULL,
    kind text NOT NULL,
    env_var text NOT NULL,
    deployment_default text,
    org_overridable boolean NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    registry_version integer NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE flag_definitions; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON TABLE router.flag_definitions IS 'Published mirror of internal/flags.Registry, upserted by the router at boot. Read by the Weave control plane to render the per-org flag override admin UI. Never read on the request path.';


--
-- Name: COLUMN flag_definitions.kind; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.flag_definitions.kind IS 'Value type: bool, int, float, or string. A stored override whose JSON type disagrees is rejected at parse time.';


--
-- Name: COLUMN flag_definitions.deployment_default; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.flag_definitions.deployment_default IS 'Deployment default as resolved from env_var at the last boot, rendered as text. Display only; the routing path reads the live in-process value, not this column.';


--
-- Name: COLUMN flag_definitions.org_overridable; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.flag_definitions.org_overridable IS 'Whether a per-organization override may be written for this flag. A registered-but-not-overridable flag is shown read-only and rejects writes.';


--
-- Name: COLUMN flag_definitions.registry_version; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.flag_definitions.registry_version IS 'Monotonic internal/flags.Registry version. Pruning only considers rows at or below the current version, so an older rolling-deploy revision cannot delete a newer definition.';


--
-- Name: loop_escalation_events; Type: TABLE; Schema: router; Owner: -
--

CREATE TABLE router.loop_escalation_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    installation_id uuid NOT NULL,
    session_key bytea NOT NULL,
    role character varying NOT NULL,
    looping_model character varying NOT NULL,
    action character varying NOT NULL,
    escalation_target character varying NOT NULL,
    loop_tool character varying NOT NULL,
    loop_input_hash character varying NOT NULL,
    repeat_count integer NOT NULL,
    distinct_ratio double precision NOT NULL,
    window_size integer NOT NULL
);


--
-- Name: TABLE loop_escalation_events; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON TABLE router.loop_escalation_events IS 'Cyclic tool-call loop detections: ops signal and (session, looping_model) -> looped training labels';


--
-- Name: COLUMN loop_escalation_events.session_key; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.loop_escalation_events.session_key IS '16-byte digest matching router.session_pins.session_key; join key for post-escalation outcome';


--
-- Name: model_router_api_keys; Type: TABLE; Schema: router; Owner: -
--

CREATE TABLE router.model_router_api_keys (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    installation_id uuid NOT NULL,
    external_id character varying(36) NOT NULL,
    name character varying(255),
    key_prefix character varying(16) NOT NULL,
    key_hash character varying(255) NOT NULL,
    key_suffix character varying(4) NOT NULL,
    last_used_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at timestamp without time zone,
    created_by character varying(36),
    scope character varying DEFAULT 'routing'::character varying NOT NULL,
    CONSTRAINT model_router_api_keys_scope_check CHECK (((scope)::text = ANY ((ARRAY['routing'::character varying, 'analytics_read'::character varying])::text[])))
);


--
-- Name: TABLE model_router_api_keys; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON TABLE router.model_router_api_keys IS 'Rotatable bearer keys (rk_ prefix). An installation may hold multiple active keys; identity is carried by router.model_router_users.';


--
-- Name: COLUMN model_router_api_keys.key_hash; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_api_keys.key_hash IS 'SHA-256 of the full rk_ token';


--
-- Name: COLUMN model_router_api_keys.key_suffix; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_api_keys.key_suffix IS 'Last 4 chars for display';


--
-- Name: COLUMN model_router_api_keys.scope; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_api_keys.scope IS 'routing = rk_ data-plane key (can proxy and spend); analytics_read = ra_ export key (read-only, non-billable)';


--
-- Name: model_router_external_api_keys; Type: TABLE; Schema: router; Owner: -
--

CREATE TABLE router.model_router_external_api_keys (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    installation_id uuid NOT NULL,
    external_id character varying(36) NOT NULL,
    provider character varying(32) NOT NULL,
    key_ciphertext bytea NOT NULL,
    key_prefix character varying(16) NOT NULL,
    key_suffix character varying(4) NOT NULL,
    key_fingerprint character varying(64) NOT NULL,
    name character varying(255),
    last_used_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at timestamp without time zone,
    created_by character varying(36),
    base_url text,
    model_aliases jsonb,
    identity_header_name text,
    identity_header_format text,
    deleted_by character varying(36),
    CONSTRAINT model_router_external_api_keys_identity_header_check CHECK (((identity_header_name IS NULL) = (identity_header_format IS NULL))),
    CONSTRAINT model_router_external_api_keys_identity_header_format_check CHECK ((identity_header_format = ANY (ARRAY['email'::text, 'json'::text]))),
    CONSTRAINT model_router_external_api_keys_provider_check CHECK (((provider)::text = 'aiand'::text))
);


--
-- Name: TABLE model_router_external_api_keys; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON TABLE router.model_router_external_api_keys IS 'Customer-owned provider API keys for BYOK routing';


--
-- Name: COLUMN model_router_external_api_keys.key_ciphertext; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_external_api_keys.key_ciphertext IS 'AES-256-GCM encrypted API key with 12-byte nonce prepended';


--
-- Name: COLUMN model_router_external_api_keys.key_fingerprint; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_external_api_keys.key_fingerprint IS 'SHA-256 of plaintext for deduplication display';


--
-- Name: COLUMN model_router_external_api_keys.base_url; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_external_api_keys.base_url IS 'Customer-supplied upstream base URL; NULL uses the deployment default for the provider';


--
-- Name: COLUMN model_router_external_api_keys.model_aliases; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_external_api_keys.model_aliases IS 'Catalog model id -> upstream model id rewrite for this key''s endpoint; NULL means no rewrite';


--
-- Name: COLUMN model_router_external_api_keys.identity_header_name; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_external_api_keys.identity_header_name IS 'Header carrying the caller identity to this key''s endpoint; NULL forwards nothing';


--
-- Name: COLUMN model_router_external_api_keys.identity_header_format; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_external_api_keys.identity_header_format IS 'email = bare address, json = URL-encoded JSON property bag';


--
-- Name: COLUMN model_router_external_api_keys.deleted_by; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_external_api_keys.deleted_by IS 'Weave account that soft-deleted or replaced this key; NULL when the router deleted it internally or attribution predates the column';


--
-- Name: model_router_installations; Type: TABLE; Schema: router; Owner: -
--

CREATE TABLE router.model_router_installations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    external_id character varying(36) NOT NULL,
    name character varying(255) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at timestamp without time zone,
    created_by character varying(36),
    excluded_models text[] DEFAULT '{}'::text[] NOT NULL,
    routing_quality_weight double precision,
    preferred_models text[] DEFAULT '{}'::text[] NOT NULL,
    routing_strategy character varying(64),
    routing_rollout_id character varying(128),
    policy_shadow_strategy character varying(64),
    policy_debug_enabled boolean DEFAULT false NOT NULL,
    policy_header_overrides_enabled boolean DEFAULT false CONSTRAINT model_router_installations_policy_header_overrides_ena_not_null NOT NULL,
    policy_routing_intent character varying(32),
    ai_training_allowed boolean DEFAULT false NOT NULL,
    content_capture_mode text,
    hide_terminal_surfaces boolean DEFAULT false NOT NULL,
    allowed_models text[] DEFAULT '{}'::text[] NOT NULL,
    first_request_served_at timestamp with time zone,
    flag_overrides jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT model_router_installations_content_capture_mode_check CHECK ((content_capture_mode = ANY (ARRAY['off'::text, 'hashed'::text, 'full'::text])))
);


--
-- Name: TABLE model_router_installations; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON TABLE router.model_router_installations IS 'Customer router installations; owns API keys';


--
-- Name: COLUMN model_router_installations.routing_strategy; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_installations.routing_strategy IS 'Optional serving-strategy override. NULL follows the deployment default.';


--
-- Name: COLUMN model_router_installations.ai_training_allowed; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_installations.ai_training_allowed IS 'Privacy snapshot synced from the organization AI-training setting. False disables policy learning.';


--
-- Name: COLUMN model_router_installations.content_capture_mode; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_installations.content_capture_mode IS 'Per-installation capture ceiling (off|hashed|full); NULL uses the deployment default';


--
-- Name: COLUMN model_router_installations.allowed_models; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_installations.allowed_models IS 'Org-level positive model allowlist. Empty = no restriction. Effective set = allowed_models minus excluded_models. Fail-closed: an allowlist with no eligible overlap refuses the turn.';


--
-- Name: COLUMN model_router_installations.first_request_served_at; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_installations.first_request_served_at IS 'Timestamp of the installation''s first routed request. Monotonic — never moves backwards or clears on key rotation.';


--
-- Name: COLUMN model_router_installations.flag_overrides; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_installations.flag_overrides IS 'Sparse per-org behavioral flag overrides, keyed by internal/flags registry key. Empty object = inherit every deployment default. Precedence: header override > this > env default, unless ROUTER_FLAG_OVERRIDES_DISABLED is set.';


--
-- Name: model_router_request_telemetry; Type: TABLE; Schema: router; Owner: -
--

CREATE TABLE router.model_router_request_telemetry (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    installation_id uuid NOT NULL,
    request_id character varying NOT NULL,
    span_type character varying NOT NULL,
    trace_id character varying NOT NULL,
    "timestamp" timestamp with time zone NOT NULL,
    requested_model character varying,
    decision_model character varying,
    decision_provider character varying,
    decision_reason character varying,
    estimated_input_tokens integer DEFAULT 0,
    sticky_hit boolean DEFAULT false,
    embed_input character varying,
    input_tokens integer DEFAULT 0,
    output_tokens integer DEFAULT 0,
    requested_input_cost_usd bigint DEFAULT 0,
    requested_output_cost_usd bigint DEFAULT 0,
    actual_input_cost_usd bigint DEFAULT 0,
    actual_output_cost_usd bigint DEFAULT 0,
    route_latency_ms bigint,
    upstream_latency_ms bigint,
    total_latency_ms bigint,
    cross_format boolean DEFAULT false,
    upstream_status_code integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    cluster_ids integer[],
    candidate_models text[],
    chosen_score double precision,
    alpha_breakdown jsonb,
    cluster_router_version character varying,
    ttft_ms bigint,
    cache_creation_tokens integer,
    cache_read_tokens integer,
    device_id character varying,
    session_id character varying,
    candidate_scores jsonb,
    propensity double precision,
    router_user_id uuid,
    client_app text,
    turn_type character varying,
    rollout_id character varying,
    upstream_finish_reason text,
    stop_reason text,
    tool_use_blocks integer,
    invalid_tool_args_blocks integer,
    failover_used boolean,
    degenerate_shadow boolean,
    session_key bytea,
    role character varying,
    fresh_decision_model character varying,
    fresh_candidate_scores jsonb,
    pin_age_sec bigint,
    tool_result_bytes integer,
    credential_key_prefix character varying,
    credential_key_suffix character varying,
    credential_source character varying,
    api_key_id uuid,
    strategy character varying,
    route_id character varying,
    policy_route_key character varying,
    policy_artifact_id character varying,
    policy_artifact_sha256 character varying,
    roster_version character varying,
    sidecar_schema_version character varying,
    training_allowed boolean,
    capture_mode character varying,
    debug_ref character varying,
    planner_outcome character varying,
    planner_reason character varying,
    planner_pin_model character varying,
    planner_pin_provider character varying,
    planner_expected_savings_usd_micros bigint,
    planner_eviction_cost_usd_micros bigint,
    planner_pin_cache_cold boolean,
    planner_shadow_outcome character varying,
    planner_shadow_savings_usd_micros bigint,
    authority_shadow_outcome character varying,
    authority_shadow_would_diverge boolean,
    authority_shadow_reason character varying,
    authority_shadow_stay_model character varying,
    authority_shadow_stay_provider character varying,
    authority_shadow_savings_usd_micros bigint,
    authority_shadow_eviction_cost_usd_micros bigint,
    authority_shadow_pin_cache_cold boolean,
    authority_shadow_corrected_outcome character varying,
    authority_shadow_corrected_savings_usd_micros bigint,
    authority_shadow_stay_score double precision,
    authority_shadow_fresh_score double precision,
    semantic_cache_hit boolean,
    cache_input_savings_usd double precision,
    pin_tier character varying
);


--
-- Name: COLUMN model_router_request_telemetry.session_key; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.session_key IS '16-byte session digest (matches session_pins / spiral_shadow_events). NULL on rows written before this column existed. Join key to spiral_shadow_events on (installation_id, session_key, role).';


--
-- Name: COLUMN model_router_request_telemetry.role; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.role IS 'Session-pin role used for the turn (roleForTier of the requested model). Pairs with session_key to identify the turn thread; matches spiral_shadow_events.role.';


--
-- Name: COLUMN model_router_request_telemetry.fresh_decision_model; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.fresh_decision_model IS 'The cluster scorer''s fresh pick for this turn, recorded even when the planner returned STAY (decision_model then names the pinned model served). NULL when the scorer did not run.';


--
-- Name: COLUMN model_router_request_telemetry.fresh_candidate_scores; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.fresh_candidate_scores IS 'The fresh pre-argmax score vector (model -> blended score) from this turn''s scorer run, recorded even on STAY. Sweep tau against served decision_model + catalog tier to measure the hysteresis downgrade opportunity. NULL when the scorer did not run.';


--
-- Name: COLUMN model_router_request_telemetry.pin_age_sec; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.pin_age_sec IS 'Age of the loaded session pin in seconds at decision time; supports min-dwell analysis for the hysteresis policy. NULL when no pin was loaded.';


--
-- Name: COLUMN model_router_request_telemetry.tool_result_bytes; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.tool_result_bytes IS 'Summed raw-JSON byte size of the trailing turn''s tool_result payload(s) -- the incoming tool-output size. Structural triviality proxy for the tier-cap shadow. NULL when the turn carries no trailing tool_result.';


--
-- Name: COLUMN model_router_request_telemetry.credential_key_prefix; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.credential_key_prefix IS 'Safe display prefix (first 8 characters) of the upstream credential that served the turn. NULL on deployment-key turns.';


--
-- Name: COLUMN model_router_request_telemetry.credential_key_suffix; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.credential_key_suffix IS 'Safe display suffix (last 4 characters) of the upstream credential that served the turn. NULL on deployment-key turns or very short credentials.';


--
-- Name: COLUMN model_router_request_telemetry.credential_source; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.credential_source IS 'Which credential precedence branch served the turn: subscription, codex_subscription, byok, or client. NULL on deployment-key turns.';


--
-- Name: COLUMN model_router_request_telemetry.planner_outcome; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.planner_outcome IS 'Planner verdict for this turn: stay or switch. NULL when the planner did not run.';


--
-- Name: COLUMN model_router_request_telemetry.planner_reason; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.planner_reason IS 'Snake-case planner reason (ev_positive, ev_negative, same_model, no_pin, …). NULL when the planner did not run.';


--
-- Name: COLUMN model_router_request_telemetry.planner_pin_model; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.planner_pin_model IS 'Pinned model the planner compared against. On a switch this is the model that was abandoned; decision_model is the one served.';


--
-- Name: COLUMN model_router_request_telemetry.planner_pin_provider; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.planner_pin_provider IS 'Provider binding of the pin the planner priced. Distinct from decision_provider on a switch.';


--
-- Name: COLUMN model_router_request_telemetry.planner_expected_savings_usd_micros; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.planner_expected_savings_usd_micros IS 'Planner expected_savings as USD micros (USD × 1e6), not float USD. NULL when the planner did not run.';


--
-- Name: COLUMN model_router_request_telemetry.planner_eviction_cost_usd_micros; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.planner_eviction_cost_usd_micros IS 'Planner eviction_cost as USD micros (USD × 1e6), not float USD. NULL when the planner did not run.';


--
-- Name: COLUMN model_router_request_telemetry.planner_pin_cache_cold; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.planner_pin_cache_cold IS 'Whether the EV math priced the pin as cache-cold. NULL when the planner did not run.';


--
-- Name: COLUMN model_router_request_telemetry.planner_shadow_outcome; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.planner_shadow_outcome IS 'Shadow (corrected-economics) verdict: stay or switch. NULL when the shadow was not computed.';


--
-- Name: COLUMN model_router_request_telemetry.planner_shadow_savings_usd_micros; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.planner_shadow_savings_usd_micros IS 'Shadow expected_savings as USD micros (USD × 1e6). NULL when the shadow was not computed.';


--
-- Name: COLUMN model_router_request_telemetry.authority_shadow_outcome; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.authority_shadow_outcome IS 'Shadow verdict of the HMM cache gate on an authoritative turn: stay or switch. NEVER what was served -- authoritative turns always serve decision_model. NULL when the shadow did not run.';


--
-- Name: COLUMN model_router_request_telemetry.authority_shadow_would_diverge; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.authority_shadow_would_diverge IS 'The gate''s own verdict that it would have served authority_shadow_stay_model instead of decision_model. Use this, not a string compare: stay_model is a serving identity that may carry '':effort'' while decision_model is a bare catalog ID.';


--
-- Name: COLUMN model_router_request_telemetry.authority_shadow_reason; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.authority_shadow_reason IS 'Snake-case reason from the shadow gate (ev_positive, ev_negative, same_model, no_pin, no_prior_usage, hmm_upgrade_confidence_low, ...). Read no_pin carefully: it also covers a pin that exists but whose serving identity carries '':effort'', because catalog.ByID strips a date suffix and not an effort suffix, so normalizeHMMStayPin rejects it. no_pin is therefore NOT the same as ''this session had no pin''.';


--
-- Name: COLUMN model_router_request_telemetry.authority_shadow_stay_model; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.authority_shadow_stay_model IS 'Pin the shadow gate priced against, as a serving identity -- it carries '':effort'' when the pin used one, unlike the bare decision_model. Compare via authority_shadow_would_diverge rather than against decision_model directly.';


--
-- Name: COLUMN model_router_request_telemetry.authority_shadow_stay_provider; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.authority_shadow_stay_provider IS 'Provider binding of authority_shadow_stay_model.';


--
-- Name: COLUMN model_router_request_telemetry.authority_shadow_savings_usd_micros; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.authority_shadow_savings_usd_micros IS 'Signed expected savings as USD micros (USD x 1e6) under the deployed economics config. Negative on a typical stay; not clamped. NULL on an early exit (no_pin, no_prior_usage, same_model, pricing_missing) where the cost arithmetic never ran -- a stored 0 there would be a fabricated measurement.';


--
-- Name: COLUMN model_router_request_telemetry.authority_shadow_eviction_cost_usd_micros; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.authority_shadow_eviction_cost_usd_micros IS 'Signed eviction cost as USD micros (USD x 1e6) under the deployed economics config. NULL on an early exit, like the savings column.';


--
-- Name: COLUMN model_router_request_telemetry.authority_shadow_pin_cache_cold; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.authority_shadow_pin_cache_cold IS 'Whether the shadow EV math priced the pin as cache-cold. NULL on an early exit, where the flag is meaningless rather than false.';


--
-- Name: COLUMN model_router_request_telemetry.authority_shadow_corrected_outcome; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.authority_shadow_corrected_outcome IS 'Verdict under corrected cache-aware economics, computed by planner.Decide as its own shadow on every EV turn regardless of the deployed config. Pre-gate: the upgrade-confidence and same-tier overrides are NOT applied to it, unlike authority_shadow_outcome. NULL on an early exit -- the enum zero value renders as ''stay'', so an uncomputed verdict must never be stored.';


--
-- Name: COLUMN model_router_request_telemetry.authority_shadow_corrected_savings_usd_micros; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.authority_shadow_corrected_savings_usd_micros IS 'Signed expected savings under corrected economics as USD micros (USD x 1e6).';


--
-- Name: COLUMN model_router_request_telemetry.authority_shadow_stay_score; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.authority_shadow_stay_score IS 'Sidecar candidate score for authority_shadow_stay_model this turn. NULL when the sidecar reported no score for the pin -- that NULL rate is the measurement that decides whether a quality tie-band is implementable at all.';


--
-- Name: COLUMN model_router_request_telemetry.authority_shadow_fresh_score; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.authority_shadow_fresh_score IS 'Sidecar candidate score for the served model this turn, paired with authority_shadow_stay_score.';


--
-- Name: COLUMN model_router_request_telemetry.semantic_cache_hit; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.semantic_cache_hit IS 'True when the turn was served from the router semantic response cache (x-router-cache: hit). Distinct from upstream prompt-cache token counters.';


--
-- Name: COLUMN model_router_request_telemetry.cache_input_savings_usd; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.cache_input_savings_usd IS 'Dollars saved on prompt-cache reads for this turn (catalog-priced). NULL when not computed at insert time.';


--
-- Name: COLUMN model_router_request_telemetry.pin_tier; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_request_telemetry.pin_tier IS 'The actual served-path pin tier for this turn (for example, authoritative_per_turn or hmm_ev_stay_ev_negative). NULL on rows written before this column existed or when no turn-loop tier was available.';


--
-- Name: model_router_user_cluster_model_lists; Type: TABLE; Schema: router; Owner: -
--

CREATE TABLE router.model_router_user_cluster_model_lists (
    router_user_id uuid NOT NULL,
    cluster_label character varying(128) NOT NULL,
    organization_id character varying(36) NOT NULL,
    models text[] NOT NULL,
    created_by character varying(36),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT model_router_user_cluster_model_lists_models_not_empty CHECK ((cardinality(models) > 0))
);


--
-- Name: COLUMN model_router_user_cluster_model_lists.cluster_label; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_user_cluster_model_lists.cluster_label IS 'Free-form label from the deployed HMM roster artifact (see GET /v1/router/hmm-roster). Not an enum — a roster bump can add or rename clusters.';


--
-- Name: COLUMN model_router_user_cluster_model_lists.organization_id; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_user_cluster_model_lists.organization_id IS 'Denormalized for control-plane queries. The router never reads it — lookups are by router_user_id only.';


--
-- Name: model_router_users; Type: TABLE; Schema: router; Owner: -
--

CREATE TABLE router.model_router_users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    installation_id uuid NOT NULL,
    email text,
    claude_account_uuid uuid,
    first_seen_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    last_seen_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at timestamp without time zone,
    display_name text,
    CONSTRAINT model_router_users_identity_present CHECK (((email IS NOT NULL) OR (claude_account_uuid IS NOT NULL)))
);


--
-- Name: TABLE model_router_users; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON TABLE router.model_router_users IS 'End-user identities seen on inbound requests, scoped to an installation. Replaces the per-user API key pattern.';


--
-- Name: COLUMN model_router_users.email; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_users.email IS 'Lowercased, trimmed user email (typically git user.email). Nullable: Claude CLI versions that send only account_uuid in metadata.user_id produce email-NULL rows keyed on (installation_id, claude_account_uuid).';


--
-- Name: COLUMN model_router_users.claude_account_uuid; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_users.claude_account_uuid IS 'Optional Claude Code account_uuid carried in metadata.user_id; informational only.';


--
-- Name: COLUMN model_router_users.display_name; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.model_router_users.display_name IS 'Free-form user display name (typically git user.name) carried on the X-Weave-User-Name request header. Nullable: requests without the header leave the column NULL; existing rows are not back-filled.';


--
-- Name: policy_shadow_decisions; Type: TABLE; Schema: router; Owner: -
--

CREATE TABLE router.policy_shadow_decisions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    installation_id uuid NOT NULL,
    organization_id character varying,
    rollout_id character varying,
    client_app character varying,
    training_allowed boolean NOT NULL,
    serving_strategy character varying NOT NULL,
    serving_model character varying NOT NULL,
    serving_provider character varying NOT NULL,
    serving_route_id character varying,
    serving_policy_route_key character varying,
    serving_policy_artifact_id character varying,
    serving_policy_artifact_sha256 character varying,
    shadow_strategy character varying NOT NULL,
    shadow_model character varying,
    shadow_provider character varying,
    shadow_route_id character varying,
    shadow_policy_route_key character varying,
    shadow_policy_artifact_id character varying,
    shadow_policy_artifact_sha256 character varying,
    shadow_latency_ms bigint NOT NULL,
    shadow_error character varying,
    models_agree boolean NOT NULL
);


--
-- Name: TABLE policy_shadow_decisions; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON TABLE router.policy_shadow_decisions IS 'Content-free serving-vs-shadow policy comparisons; shadow decisions never dispatch or enter online learning';


--
-- Name: production_request_telemetry; Type: VIEW; Schema: router; Owner: -
--

CREATE VIEW router.production_request_telemetry AS
 SELECT id,
    installation_id,
    request_id,
    span_type,
    trace_id,
    "timestamp",
    requested_model,
    decision_model,
    decision_provider,
    decision_reason,
    estimated_input_tokens,
    sticky_hit,
    embed_input,
    input_tokens,
    output_tokens,
    requested_input_cost_usd,
    requested_output_cost_usd,
    actual_input_cost_usd,
    actual_output_cost_usd,
    route_latency_ms,
    upstream_latency_ms,
    total_latency_ms,
    cross_format,
    upstream_status_code,
    created_at,
    cluster_ids,
    candidate_models,
    chosen_score,
    alpha_breakdown,
    cluster_router_version,
    ttft_ms,
    cache_creation_tokens,
    cache_read_tokens,
    device_id,
    session_id,
    candidate_scores,
    propensity,
    router_user_id,
    client_app,
    turn_type,
    rollout_id,
    upstream_finish_reason,
    stop_reason,
    tool_use_blocks,
    invalid_tool_args_blocks,
    failover_used,
    degenerate_shadow,
    session_key,
    role,
    fresh_decision_model,
    fresh_candidate_scores,
    pin_age_sec,
    tool_result_bytes,
    credential_key_prefix,
    credential_key_suffix,
    credential_source
   FROM router.model_router_request_telemetry
  WHERE (((span_type)::text = 'router.upstream'::text) AND ((client_app IS NULL) OR (client_app !~~ 'weave-eval%'::text)));


--
-- Name: request_feedback; Type: TABLE; Schema: router; Owner: -
--

CREATE TABLE router.request_feedback (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    installation_id uuid NOT NULL,
    external_id character varying NOT NULL,
    request_id character varying NOT NULL,
    rating character varying NOT NULL,
    comment text,
    source character varying DEFAULT 'link'::character varying NOT NULL,
    router_user_id uuid
);


--
-- Name: TABLE request_feedback; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON TABLE router.request_feedback IS 'Router-owned per-request human feedback captured via the no-login feedback link; mirrored into Weave via the router.feedback OTLP span';


--
-- Name: router_feedback; Type: TABLE; Schema: router; Owner: -
--

CREATE TABLE router.router_feedback (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    installation_id uuid NOT NULL,
    session_key bytea NOT NULL,
    role character varying NOT NULL,
    router_user_id uuid,
    client_app text,
    session_id character varying,
    requested_model character varying NOT NULL,
    served_model character varying NOT NULL,
    feedback text NOT NULL,
    rating character varying,
    suggested_label character varying,
    source character varying DEFAULT 'user'::character varying NOT NULL,
    request_id character varying,
    route_id character varying
);


--
-- Name: TABLE router_feedback; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON TABLE router.router_feedback IS 'User-submitted /router-feedback about routing decisions or model performance';


--
-- Name: COLUMN router_feedback.session_key; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.router_feedback.session_key IS '16-byte digest matching router.session_pins.session_key';


--
-- Name: COLUMN router_feedback.rating; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.router_feedback.rating IS '"up", "down", or NULL. Null means abstain or note-only (no verdict).';


--
-- Name: COLUMN router_feedback.suggested_label; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.router_feedback.suggested_label IS '"fast", "explore", "balanced", "high", or "maximum" — the complexity label the submitter thinks the turn needed. Set when rating is "down", NULL otherwise.';


--
-- Name: COLUMN router_feedback.source; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.router_feedback.source IS 'How the feedback was submitted: "user" (explicit /rf command), "auto" (automated judge at session stop).';


--
-- Name: COLUMN router_feedback.request_id; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.router_feedback.request_id IS 'Telemetry request_id for the specific turn this feedback targets. NULL when no sequence was specified.';


--
-- Name: COLUMN router_feedback.route_id; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.router_feedback.route_id IS 'Sidecar correlation id from the telemetry row (HMM/RL). NULL when no sequence was specified.';


--
-- Name: session_pins; Type: TABLE; Schema: router; Owner: -
--

CREATE TABLE router.session_pins (
    session_key bytea NOT NULL,
    role character varying(32) DEFAULT 'default'::character varying NOT NULL,
    installation_id uuid NOT NULL,
    pinned_provider character varying(32) NOT NULL,
    pinned_model character varying(128) NOT NULL,
    decision_reason text NOT NULL,
    turn_count integer DEFAULT 1 NOT NULL,
    pinned_until timestamp without time zone NOT NULL,
    first_pinned_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    last_seen_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    last_input_tokens integer DEFAULT 0 NOT NULL,
    last_cached_read_tokens integer DEFAULT 0 NOT NULL,
    last_cached_write_tokens integer DEFAULT 0 NOT NULL,
    last_output_tokens integer DEFAULT 0 NOT NULL,
    last_turn_ended_at timestamp with time zone,
    consecutive_upstream_errors integer DEFAULT 0 NOT NULL,
    last_served_model character varying DEFAULT ''::character varying NOT NULL,
    has_ever_switched boolean DEFAULT false NOT NULL,
    paired_provider character varying(32) DEFAULT ''::character varying NOT NULL,
    paired_model character varying(128) DEFAULT ''::character varying NOT NULL,
    consecutive_overload_errors integer DEFAULT 0 NOT NULL,
    disabled_providers text[] DEFAULT '{}'::text[] NOT NULL,
    policy_group character varying(64) DEFAULT ''::character varying NOT NULL,
    routing_strategy character varying(32) DEFAULT ''::character varying NOT NULL
);


--
-- Name: TABLE session_pins; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON TABLE router.session_pins IS 'Session-sticky routing pins; sliding 1h TTL matching Anthropic prompt cache';


--
-- Name: COLUMN session_pins.session_key; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.session_pins.session_key IS '16-byte digest derived from api_key_id + (metadata.user_id | system+first-user hashes)';


--
-- Name: COLUMN session_pins.role; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.session_pins.role IS 'Stage 1 always emits "default"; turn-type roles land with §3.3';


--
-- Name: session_strategy_preferences; Type: TABLE; Schema: router; Owner: -
--

CREATE TABLE router.session_strategy_preferences (
    installation_id uuid NOT NULL,
    session_key bytea NOT NULL,
    strategy character varying(32) NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    CONSTRAINT session_strategy_preferences_session_key_check CHECK ((octet_length(session_key) = 16)),
    CONSTRAINT session_strategy_preferences_strategy_check CHECK (((strategy)::text = 'hmm_beta'::text))
);


--
-- Name: TABLE session_strategy_preferences; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON TABLE router.session_strategy_preferences IS 'Explicit per-session router strategy preferences';


--
-- Name: spiral_shadow_events; Type: TABLE; Schema: router; Owner: -
--

CREATE TABLE router.spiral_shadow_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    installation_id uuid NOT NULL,
    session_key bytea NOT NULL,
    role character varying NOT NULL,
    routed_model character varying NOT NULL,
    turn_type character varying NOT NULL,
    reason character varying NOT NULL,
    err_streak integer NOT NULL,
    errored_results integer NOT NULL,
    tool_results integer NOT NULL,
    max_same_file_edits integer NOT NULL,
    same_file_path_hash character varying NOT NULL,
    repeat_frac double precision NOT NULL,
    monologue_len integer NOT NULL,
    tool_call_count integer NOT NULL,
    message_count integer NOT NULL,
    ping_pong_len integer DEFAULT 0 NOT NULL,
    steps_since_progress integer DEFAULT 0 NOT NULL
);


--
-- Name: TABLE spiral_shadow_events; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON TABLE router.spiral_shadow_events IS 'Shadow-mode spiral (death-march) detections: log-only fire-rate corpus measured on live traffic before escalation is armed';


--
-- Name: COLUMN spiral_shadow_events.session_key; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.spiral_shadow_events.session_key IS '16-byte digest matching router.session_pins.session_key; join key for session outcome';


--
-- Name: COLUMN spiral_shadow_events.ping_pong_len; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.spiral_shadow_events.ping_pong_len IS 'Length of the trailing A/B/A/B alternation between exactly two tool-call signatures';


--
-- Name: COLUMN spiral_shadow_events.steps_since_progress; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.spiral_shadow_events.steps_since_progress IS 'Tool calls made since the last non-errored edit/write tool_result; 0 when the session has never attempted an edit';


--
-- Name: struggle_escalation_events; Type: TABLE; Schema: router; Owner: -
--

CREATE TABLE router.struggle_escalation_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    installation_id uuid NOT NULL,
    session_key bytea NOT NULL,
    role character varying NOT NULL,
    struggling_model character varying NOT NULL,
    action character varying NOT NULL,
    escalation_target character varying DEFAULT ''::character varying NOT NULL,
    turn_count integer NOT NULL,
    wall_seconds bigint NOT NULL,
    session_ever_switched boolean NOT NULL,
    arming_mode character varying DEFAULT ''::character varying NOT NULL,
    evidence_reasons character varying[] DEFAULT '{}'::character varying[] NOT NULL
);


--
-- Name: COLUMN struggle_escalation_events.arming_mode; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.struggle_escalation_events.arming_mode IS 'What armed this escalation: turn_wall (turn/wall thresholds) or evidence (behavioral signals)';


--
-- Name: COLUMN struggle_escalation_events.evidence_reasons; Type: COMMENT; Schema: router; Owner: -
--

COMMENT ON COLUMN router.struggle_escalation_events.evidence_reasons IS 'Spiral signal classes present at arming time (err_streak, same_file_thrash, repetition, monologue, ping_pong, no_progress)';


--
-- Name: struggle_shadow_events; Type: TABLE; Schema: router; Owner: -
--

CREATE TABLE router.struggle_shadow_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    installation_id uuid NOT NULL,
    session_key bytea NOT NULL,
    role character varying NOT NULL,
    routed_model character varying NOT NULL,
    turn_type character varying NOT NULL,
    reason character varying NOT NULL,
    turn_count integer NOT NULL,
    wall_seconds bigint NOT NULL,
    session_ever_switched boolean NOT NULL,
    est_input_tokens integer NOT NULL
);


--
-- Name: account_sessions account_sessions_pkey; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.account_sessions
    ADD CONSTRAINT account_sessions_pkey PRIMARY KEY (id);


--
-- Name: accounts accounts_pkey; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.accounts
    ADD CONSTRAINT accounts_pkey PRIMARY KEY (id);


--
-- Name: cluster_model_lists cluster_model_lists_api_key_id_cluster_label_key; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.cluster_model_lists
    ADD CONSTRAINT cluster_model_lists_api_key_id_cluster_label_key UNIQUE (api_key_id, cluster_label);


--
-- Name: cluster_model_lists cluster_model_lists_pkey; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.cluster_model_lists
    ADD CONSTRAINT cluster_model_lists_pkey PRIMARY KEY (id);


--
-- Name: flag_definitions flag_definitions_pkey; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.flag_definitions
    ADD CONSTRAINT flag_definitions_pkey PRIMARY KEY (key);


--
-- Name: loop_escalation_events loop_escalation_events_pkey; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.loop_escalation_events
    ADD CONSTRAINT loop_escalation_events_pkey PRIMARY KEY (id);


--
-- Name: model_router_api_keys model_router_api_keys_pkey; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.model_router_api_keys
    ADD CONSTRAINT model_router_api_keys_pkey PRIMARY KEY (id);


--
-- Name: model_router_external_api_keys model_router_external_api_keys_pkey; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.model_router_external_api_keys
    ADD CONSTRAINT model_router_external_api_keys_pkey PRIMARY KEY (id);


--
-- Name: model_router_installations model_router_installations_pkey; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.model_router_installations
    ADD CONSTRAINT model_router_installations_pkey PRIMARY KEY (id);


--
-- Name: model_router_request_telemetry model_router_request_telemetry_pkey; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.model_router_request_telemetry
    ADD CONSTRAINT model_router_request_telemetry_pkey PRIMARY KEY (id);


--
-- Name: model_router_user_cluster_model_lists model_router_user_cluster_model_lists_pkey; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.model_router_user_cluster_model_lists
    ADD CONSTRAINT model_router_user_cluster_model_lists_pkey PRIMARY KEY (router_user_id, cluster_label);


--
-- Name: model_router_users model_router_users_pkey; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.model_router_users
    ADD CONSTRAINT model_router_users_pkey PRIMARY KEY (id);


--
-- Name: policy_shadow_decisions policy_shadow_decisions_pkey; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.policy_shadow_decisions
    ADD CONSTRAINT policy_shadow_decisions_pkey PRIMARY KEY (id);


--
-- Name: request_feedback request_feedback_installation_id_request_id_key; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.request_feedback
    ADD CONSTRAINT request_feedback_installation_id_request_id_key UNIQUE (installation_id, request_id);


--
-- Name: request_feedback request_feedback_pkey; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.request_feedback
    ADD CONSTRAINT request_feedback_pkey PRIMARY KEY (id);


--
-- Name: router_feedback router_feedback_pkey; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.router_feedback
    ADD CONSTRAINT router_feedback_pkey PRIMARY KEY (id);


--
-- Name: session_pins session_pins_pkey; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.session_pins
    ADD CONSTRAINT session_pins_pkey PRIMARY KEY (session_key, role);


--
-- Name: session_strategy_preferences session_strategy_preferences_pkey; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.session_strategy_preferences
    ADD CONSTRAINT session_strategy_preferences_pkey PRIMARY KEY (installation_id, session_key);


--
-- Name: spiral_shadow_events spiral_shadow_events_pkey; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.spiral_shadow_events
    ADD CONSTRAINT spiral_shadow_events_pkey PRIMARY KEY (id);


--
-- Name: struggle_escalation_events struggle_escalation_events_pkey; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.struggle_escalation_events
    ADD CONSTRAINT struggle_escalation_events_pkey PRIMARY KEY (id);


--
-- Name: struggle_shadow_events struggle_shadow_events_pkey; Type: CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.struggle_shadow_events
    ADD CONSTRAINT struggle_shadow_events_pkey PRIMARY KEY (id);


--
-- Name: account_sessions_account_id_issued_at_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX account_sessions_account_id_issued_at_idx ON router.account_sessions USING btree (account_id, issued_at DESC);


--
-- Name: account_sessions_token_hash_unique; Type: INDEX; Schema: router; Owner: -
--

CREATE UNIQUE INDEX account_sessions_token_hash_unique ON router.account_sessions USING btree (token_hash);


--
-- Name: accounts_aiand_org_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX accounts_aiand_org_idx ON router.accounts USING btree (aiand_organization_id) WHERE (deleted_at IS NULL);


--
-- Name: accounts_aiand_user_id_active_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE UNIQUE INDEX accounts_aiand_user_id_active_idx ON router.accounts USING btree (aiand_user_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_router_request_telemetry_api_key_id; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX idx_router_request_telemetry_api_key_id ON router.model_router_request_telemetry USING btree (api_key_id, "timestamp" DESC);


--
-- Name: loop_escalation_events_installation_id_created_at_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX loop_escalation_events_installation_id_created_at_idx ON router.loop_escalation_events USING btree (installation_id, created_at DESC);


--
-- Name: loop_escalation_events_session_key_role_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX loop_escalation_events_session_key_role_idx ON router.loop_escalation_events USING btree (session_key, role);


--
-- Name: model_router_api_keys_external_id_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX model_router_api_keys_external_id_idx ON router.model_router_api_keys USING btree (external_id);


--
-- Name: model_router_api_keys_installation_id_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX model_router_api_keys_installation_id_idx ON router.model_router_api_keys USING btree (installation_id);


--
-- Name: model_router_api_keys_key_hash_unique; Type: INDEX; Schema: router; Owner: -
--

CREATE UNIQUE INDEX model_router_api_keys_key_hash_unique ON router.model_router_api_keys USING btree (key_hash) WHERE (deleted_at IS NULL);


--
-- Name: model_router_external_api_keys_external_id_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX model_router_external_api_keys_external_id_idx ON router.model_router_external_api_keys USING btree (external_id);


--
-- Name: model_router_external_api_keys_installation_active_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX model_router_external_api_keys_installation_active_idx ON router.model_router_external_api_keys USING btree (installation_id, created_at DESC) WHERE (deleted_at IS NULL);


--
-- Name: model_router_external_api_keys_installation_provider_active_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE UNIQUE INDEX model_router_external_api_keys_installation_provider_active_idx ON router.model_router_external_api_keys USING btree (installation_id, provider) WHERE (deleted_at IS NULL);


--
-- Name: model_router_installations_external_id_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX model_router_installations_external_id_idx ON router.model_router_installations USING btree (external_id);


--
-- Name: model_router_installations_name_external_id_unique; Type: INDEX; Schema: router; Owner: -
--

CREATE UNIQUE INDEX model_router_installations_name_external_id_unique ON router.model_router_installations USING btree (external_id, name) WHERE (deleted_at IS NULL);


--
-- Name: model_router_request_telemetr_installation_id_request_id_sp_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE UNIQUE INDEX model_router_request_telemetr_installation_id_request_id_sp_idx ON router.model_router_request_telemetry USING btree (installation_id, request_id, span_type);


--
-- Name: model_router_request_telemetry_export_cursor_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX model_router_request_telemetry_export_cursor_idx ON router.model_router_request_telemetry USING btree (installation_id, created_at, id) WHERE ((span_type)::text = 'router.upstream'::text);


--
-- Name: model_router_request_telemetry_installation_id_timestamp_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX model_router_request_telemetry_installation_id_timestamp_idx ON router.model_router_request_telemetry USING btree (installation_id, "timestamp" DESC);


--
-- Name: model_router_request_telemetry_rollout_id_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX model_router_request_telemetry_rollout_id_idx ON router.model_router_request_telemetry USING btree (rollout_id) WHERE (rollout_id IS NOT NULL);


--
-- Name: model_router_request_telemetry_session_cost_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX model_router_request_telemetry_session_cost_idx ON router.model_router_request_telemetry USING btree (installation_id, session_id) WHERE (((span_type)::text = ANY ((ARRAY['router.upstream'::character varying, 'router.auxiliary_inference'::character varying])::text[])) AND (session_id IS NOT NULL));


--
-- Name: model_router_user_cluster_model_lists_organization_id_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX model_router_user_cluster_model_lists_organization_id_idx ON router.model_router_user_cluster_model_lists USING btree (organization_id);


--
-- Name: model_router_users_installation_account_unique; Type: INDEX; Schema: router; Owner: -
--

CREATE UNIQUE INDEX model_router_users_installation_account_unique ON router.model_router_users USING btree (installation_id, claude_account_uuid) WHERE ((email IS NULL) AND (claude_account_uuid IS NOT NULL) AND (deleted_at IS NULL));


--
-- Name: model_router_users_installation_email_unique; Type: INDEX; Schema: router; Owner: -
--

CREATE UNIQUE INDEX model_router_users_installation_email_unique ON router.model_router_users USING btree (installation_id, email) WHERE (deleted_at IS NULL);


--
-- Name: model_router_users_installation_id_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX model_router_users_installation_id_idx ON router.model_router_users USING btree (installation_id);


--
-- Name: policy_shadow_decisions_installation_created_at_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX policy_shadow_decisions_installation_created_at_idx ON router.policy_shadow_decisions USING btree (installation_id, created_at DESC);


--
-- Name: policy_shadow_decisions_rollout_id_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX policy_shadow_decisions_rollout_id_idx ON router.policy_shadow_decisions USING btree (rollout_id) WHERE (rollout_id IS NOT NULL);


--
-- Name: request_feedback_installation_id_request_id_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX request_feedback_installation_id_request_id_idx ON router.request_feedback USING btree (installation_id, request_id);


--
-- Name: router_feedback_installation_id_created_at_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX router_feedback_installation_id_created_at_idx ON router.router_feedback USING btree (installation_id, created_at DESC);


--
-- Name: router_feedback_session_key_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX router_feedback_session_key_idx ON router.router_feedback USING btree (session_key);


--
-- Name: session_pins_pinned_until_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX session_pins_pinned_until_idx ON router.session_pins USING btree (pinned_until);


--
-- Name: spiral_shadow_events_installation_id_created_at_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX spiral_shadow_events_installation_id_created_at_idx ON router.spiral_shadow_events USING btree (installation_id, created_at DESC);


--
-- Name: spiral_shadow_events_session_key_role_reason_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX spiral_shadow_events_session_key_role_reason_idx ON router.spiral_shadow_events USING btree (session_key, role, reason);


--
-- Name: struggle_escalation_events_installation_created_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX struggle_escalation_events_installation_created_idx ON router.struggle_escalation_events USING btree (installation_id, created_at DESC);


--
-- Name: struggle_escalation_events_session_role_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE UNIQUE INDEX struggle_escalation_events_session_role_idx ON router.struggle_escalation_events USING btree (session_key, role);


--
-- Name: struggle_shadow_events_session_role_idx; Type: INDEX; Schema: router; Owner: -
--

CREATE INDEX struggle_shadow_events_session_role_idx ON router.struggle_shadow_events USING btree (session_key, role);


--
-- Name: account_sessions account_sessions_account_id_fkey; Type: FK CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.account_sessions
    ADD CONSTRAINT account_sessions_account_id_fkey FOREIGN KEY (account_id) REFERENCES router.accounts(id) ON DELETE CASCADE;


--
-- Name: cluster_model_lists cluster_model_lists_api_key_id_fkey; Type: FK CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.cluster_model_lists
    ADD CONSTRAINT cluster_model_lists_api_key_id_fkey FOREIGN KEY (api_key_id) REFERENCES router.model_router_api_keys(id) ON DELETE CASCADE;


--
-- Name: loop_escalation_events loop_escalation_events_installation_id_fkey; Type: FK CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.loop_escalation_events
    ADD CONSTRAINT loop_escalation_events_installation_id_fkey FOREIGN KEY (installation_id) REFERENCES router.model_router_installations(id) ON DELETE CASCADE;


--
-- Name: model_router_api_keys model_router_api_keys_installation_id_fkey; Type: FK CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.model_router_api_keys
    ADD CONSTRAINT model_router_api_keys_installation_id_fkey FOREIGN KEY (installation_id) REFERENCES router.model_router_installations(id) ON DELETE CASCADE;


--
-- Name: model_router_external_api_keys model_router_external_api_keys_installation_id_fkey; Type: FK CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.model_router_external_api_keys
    ADD CONSTRAINT model_router_external_api_keys_installation_id_fkey FOREIGN KEY (installation_id) REFERENCES router.model_router_installations(id) ON DELETE CASCADE;


--
-- Name: model_router_request_telemetry model_router_request_telemetry_installation_id_fkey; Type: FK CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.model_router_request_telemetry
    ADD CONSTRAINT model_router_request_telemetry_installation_id_fkey FOREIGN KEY (installation_id) REFERENCES router.model_router_installations(id) ON DELETE CASCADE;


--
-- Name: model_router_user_cluster_model_lists model_router_user_cluster_model_lists_router_user_id_fkey; Type: FK CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.model_router_user_cluster_model_lists
    ADD CONSTRAINT model_router_user_cluster_model_lists_router_user_id_fkey FOREIGN KEY (router_user_id) REFERENCES router.model_router_users(id) ON DELETE CASCADE;


--
-- Name: model_router_users model_router_users_installation_id_fkey; Type: FK CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.model_router_users
    ADD CONSTRAINT model_router_users_installation_id_fkey FOREIGN KEY (installation_id) REFERENCES router.model_router_installations(id) ON DELETE CASCADE;


--
-- Name: policy_shadow_decisions policy_shadow_decisions_installation_id_fkey; Type: FK CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.policy_shadow_decisions
    ADD CONSTRAINT policy_shadow_decisions_installation_id_fkey FOREIGN KEY (installation_id) REFERENCES router.model_router_installations(id) ON DELETE CASCADE;


--
-- Name: request_feedback request_feedback_installation_id_fkey; Type: FK CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.request_feedback
    ADD CONSTRAINT request_feedback_installation_id_fkey FOREIGN KEY (installation_id) REFERENCES router.model_router_installations(id) ON DELETE CASCADE;


--
-- Name: router_feedback router_feedback_installation_id_fkey; Type: FK CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.router_feedback
    ADD CONSTRAINT router_feedback_installation_id_fkey FOREIGN KEY (installation_id) REFERENCES router.model_router_installations(id) ON DELETE CASCADE;


--
-- Name: session_pins session_pins_installation_id_fkey; Type: FK CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.session_pins
    ADD CONSTRAINT session_pins_installation_id_fkey FOREIGN KEY (installation_id) REFERENCES router.model_router_installations(id) ON DELETE CASCADE;


--
-- Name: session_strategy_preferences session_strategy_preferences_installation_id_fkey; Type: FK CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.session_strategy_preferences
    ADD CONSTRAINT session_strategy_preferences_installation_id_fkey FOREIGN KEY (installation_id) REFERENCES router.model_router_installations(id) ON DELETE CASCADE;


--
-- Name: spiral_shadow_events spiral_shadow_events_installation_id_fkey; Type: FK CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.spiral_shadow_events
    ADD CONSTRAINT spiral_shadow_events_installation_id_fkey FOREIGN KEY (installation_id) REFERENCES router.model_router_installations(id) ON DELETE CASCADE;


--
-- Name: struggle_escalation_events struggle_escalation_events_installation_id_fkey; Type: FK CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.struggle_escalation_events
    ADD CONSTRAINT struggle_escalation_events_installation_id_fkey FOREIGN KEY (installation_id) REFERENCES router.model_router_installations(id) ON DELETE CASCADE;


--
-- Name: struggle_shadow_events struggle_shadow_events_installation_id_fkey; Type: FK CONSTRAINT; Schema: router; Owner: -
--

ALTER TABLE ONLY router.struggle_shadow_events
    ADD CONSTRAINT struggle_shadow_events_installation_id_fkey FOREIGN KEY (installation_id) REFERENCES router.model_router_installations(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--


