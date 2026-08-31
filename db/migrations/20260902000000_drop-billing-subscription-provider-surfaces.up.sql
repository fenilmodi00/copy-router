BEGIN;

-- Drop the prepaid billing, subscription-bypass, and multi-provider surfaces.
-- The router is now a single-provider (ai&) BYOK product: usage cost stays
-- visible through model_router_request_telemetry (catalog-priced at write
-- time), but the router no longer charges or gates on prepaid credit.

DROP TABLE IF EXISTS router.organization_credit_balance;
DROP TABLE IF EXISTS router.organization_credit_ledger;
DROP TABLE IF EXISTS router.organization_billing_overrides;
DROP TABLE IF EXISTS router.organization_autopay_config;
DROP TABLE IF EXISTS router.organization_spend_limits;
DROP TABLE IF EXISTS router.model_router_user_spend_limits;
DROP TABLE IF EXISTS router.model_router_user_monthly_spend;
DROP TABLE IF EXISTS router.organization_monthly_spend;

ALTER TABLE router.model_router_api_keys
  DROP COLUMN IF EXISTS spend_cap_usd_micros,
  DROP COLUMN IF EXISTS spent_usd_micros;

ALTER TABLE router.model_router_installations
  DROP COLUMN IF EXISTS usage_bypass_enabled,
  DROP COLUMN IF EXISTS usage_bypass_threshold,
  DROP COLUMN IF EXISTS subscription_routing_disabled,
  DROP COLUMN IF EXISTS byok_enabled,
  DROP COLUMN IF EXISTS excluded_providers;

ALTER TABLE router.model_router_request_telemetry
  DROP COLUMN IF EXISTS unified_limit_headers;

-- Key-pair and workload-identity auth never applied to ai& keys; aiand is the
-- only provider left, so bearer is the only auth shape. base_url,
-- model_aliases, and identity_header stay: they are live BYOK features.
ALTER TABLE router.model_router_external_api_keys
  DROP CONSTRAINT IF EXISTS model_router_external_api_keys_keypair_auth_check,
  DROP CONSTRAINT IF EXISTS model_router_external_api_keys_wif_auth_check;

ALTER TABLE router.model_router_external_api_keys
  DROP COLUMN IF EXISTS auth_type,
  DROP COLUMN IF EXISTS auth_account,
  DROP COLUMN IF EXISTS auth_user;

ALTER TABLE router.model_router_external_api_keys
  DROP CONSTRAINT IF EXISTS model_router_external_api_keys_provider_check;

ALTER TABLE router.model_router_external_api_keys
  ADD CONSTRAINT model_router_external_api_keys_provider_check
  CHECK (provider = 'aiand');

COMMIT;
