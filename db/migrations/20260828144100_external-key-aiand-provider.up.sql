BEGIN;

-- Self-serve login stores the user's aiand sk- key as installation BYOK so
-- dashboard inference bills the signed-in user, not the deployment key.
ALTER TABLE router.model_router_external_api_keys
  DROP CONSTRAINT model_router_external_api_keys_provider_check;

ALTER TABLE router.model_router_external_api_keys
  ADD CONSTRAINT model_router_external_api_keys_provider_check
  CHECK (provider IN (
    'anthropic','openai','google','openrouter','fireworks',
    'bedrock','makora','together','xai','anthropic_gateway','openai_gateway',
    'aiand'
  ));

COMMIT;
