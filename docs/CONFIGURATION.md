# Configuration reference

All router configuration is via environment variables ([12-factor](https://12factor.net/config)).
This page is the exhaustive reference; the [README](../README.md) has the
60-second quickstart.

## Table of contents

- [Provider API keys](#provider-api-keys)
  - [Client Claude aliases → catalog IDs](#client-claude-aliases--catalog-ids)
  - [Routing intent via the `model` field](#routing-intent-via-the-model-field)
  - [Peripheral gateway / BYOK env](#peripheral-gateway--byok-env)
  - [Key-pair auth](#key-pair-auth)
  - [Workload identity federation](#workload-identity-federation)
- [Postgres](#postgres)
- [Server](#server)
  - [Playground (`/ui/playground`)](#playground-uiplayground)
  - [Self-service mode (`ROUTER_DEPLOYMENT_MODE=selfserve`)](#self-service-mode-router_deployment_modeselfserve)
- [Routing](#routing)
- [Provider and model exclusions](#provider-and-model-exclusions)
- [Policy sidecars](#policy-sidecars)
- [BYOK encryption](#byok-encryption)
- [Telemetry (OpenTelemetry)](#telemetry-opentelemetry)
- [Cluster-routing artifacts](#cluster-routing-artifacts)

## Provider API keys

This deploy registers **ai& (aiand) only** at boot. The product lead is
OpenAI-compatible `POST /v1/chat/completions` against the open-weight catalog
(`moonshotai/…`, `zai-org/…`, `deepseek-ai/…`). Set `AIAND_API_KEY` — that is the
deploy baseline. A secondary `POST /v1/messages` ingress accepts Anthropic wire
and translates to OpenAI-compat before dispatch; do not treat it as a second
upstream.

| Variable        | Default                    | Effect |
| --------------- | -------------------------- | ------ |
| `AIAND_API_KEY` | *(none)*                   | **Deploy baseline.** Enables ai& (aiand.com) OpenAI-compatible inference for the open-weight catalog. |
| `AIAND_API_URL` | `https://api.aiand.com/v1` | Base URL for ai&; `/chat/completions` is appended to it. |
| `AIAND_IDENTITY_URL` | `https://api.aiand.com` | API root for the dashboard identity probe (`GET <root>/v1/models`). Separate from `AIAND_API_URL`, which is the OpenAI-compatible inference base and already includes `/v1`. |

**BYOK (per-installation keys).** Instead of (or in addition to) the env vars
above, each installation can supply its own provider keys via the dashboard.
Those are stored in Postgres and used only for that installation's traffic.
See [BYOK encryption](#byok-encryption).

### Client Claude aliases → catalog IDs

Clients may still send Claude-era model names on **force-model** paths
(`claude-sonnet-5`, `/force-model opus`, `x-weave-force-model`, `model="opus"`,
…). Those strings are **remap inputs only** — they are not catalog rows.
Force-model resolution maps them onto existing aiand catalog IDs:

| Client alias | Catalog ID |
| --- | --- |
| `claude-fable-5` (+ fable shorts) | `moonshotai/kimi-k3` |
| `claude-opus-4-8` (+ `opus-4-8` / `claude-4-8`) | `zai-org/glm-5.2` |
| `claude-opus-5` / `opus` / `claude-5` | `zai-org/glm-5.2` |
| `claude-sonnet-5` / `sonnet` | `moonshotai/kimi-k2.7` |
| `claude-sonnet-4-6` | `deepseek-ai/deepseek-v4-pro` |
| `claude-haiku-4-5` / `haiku` | `deepseek-ai/deepseek-v4-flash` |

### Routing intent via the `model` field

The plain inbound `model` field is now routing intent, not a passthrough. Two
values plus everything in between:

- `model="auto"` — the default. Route normally: the scorer picks the model per
  action.
- `model="<catalog-id-or-alias>"` — force exactly that model: a canonical
  catalog ID (`moonshotai/kimi-k2.7`, `deepseek-ai/deepseek-v4-flash`), a bare
  tail (`kimi-k2.7`), an alias (`opus`, `kimi-k3`, `claude-sonnet-5`,
  `claude-haiku-4-5`), an `openai/…` provider prefix, optionally with a
  `:level` effort suffix (`opus:high`). This is **exactly equivalent** to the
  `x-weave-force-model` header / `/force-model` command: it writes the same
  user-forced session pin (`ReasonUserForceModel`, never expires), serves that
  model on the same turn, and 400s on values that name no catalog model. A
  Claude Code `[1m]` context-window variant tag on the field
  (`kimi-k2.7[1m]`) is stripped before resolution.

Precedence, when more than one carrier names a model:
`/force-model` chat command > `model` field > `x-weave-force-model` header. All
three still work; the header is now the fallback when the field is empty or
`auto`.

Two rules that make `auto` safe:

- `model="auto"` **never clears** an existing pin. A user-forced pin from an
  earlier `/force-model`, header, or model field survives `auto` turns —
  only `/unforce-model` clears it.
- A `model` value that resolves to **no catalog model** (typo, misleading
  entry) is now HTTP 400 instead of being silently ignored. This is the
  headline behavior change: previously such fields were skipped and the request
  routed on; now they fail loud with the unresolvable value quoted back.

The router reuses `translate.CanonicalModel` to strip Claude Code's `[1m]`
variant tag from the field before deciding, and ranges every resolvable value
through the same forced-model resolver as the header and command. A `:level`
suffix on the winning value is honored exactly as it is on `x-weave-force-model`
(effort lands in `router.Overrides.ForceEffort`).

### Peripheral gateway / BYOK env

OpenRouter, Anthropic, OpenAI, Google, and gateway vars are **not** the deploy
baseline. Composition root does not register those providers for the aiand-only
deploy. Keep them only when wiring enterprise gateways or BYOK that still speak
those surfaces. Host WSL / Build.io paths stay aiand-only — do not add Anthropic
or OpenAI provider keys there.

| Variable              | Default                                                   | Effect |
| --------------------- | --------------------------------------------------------- | ------ |
| `OPENROUTER_API_KEY`  | *(none)*                                                  | Peripheral. Enables OpenRouter / any OpenAI-compatible pool when registered (not aiand-only boot). |
| `OPENROUTER_BASE_URL` | `https://openrouter.ai/api/v1`                            | Override for OpenRouter or any OpenAI-compatible endpoint (vLLM, Together, Fireworks, self-hosted). |
| `ANTHROPIC_API_KEY`   | *(none — passthrough)*                                    | Peripheral. Router's own Anthropic key for BYOK / gateway paths. When unset, client `Authorization` headers may pass through on Anthropic-family bindings. |
| `OPENAI_API_KEY`      | *(none)*                                                  | Peripheral. Enables an OpenAI provider binding when registered. |
| `OPENAI_BASE_URL`     | `https://api.openai.com`                                  | Override for OpenAI (e.g. Azure OpenAI). |
| `GOOGLE_API_KEY`      | *(none)*                                                  | Peripheral. Used by optional HMM sidecar embeddings; not an aiand deploy upstream. |
| `GOOGLE_BASE_URL`     | `https://generativelanguage.googleapis.com/v1beta/openai` | Override for Gemini-shaped OpenAI-compat endpoints. |
| `ANTHROPIC_GATEWAY_BASE_URL` | *(none)*                                           | Base URL of an Anthropic-compatible gateway; `/v1/messages` is appended to it. |
| `ANTHROPIC_GATEWAY_TOKEN`    | *(none)*                                           | Token for that gateway, sent as `Authorization: Bearer`. Only used when `ANTHROPIC_GATEWAY_BASE_URL` is also set. |
| `OPENAI_GATEWAY_BASE_URL`    | *(none)*                                           | Base URL of an OpenAI-compatible gateway; `/chat/completions` is appended to it. |
| `OPENAI_GATEWAY_TOKEN`       | *(none)*                                           | Token for that gateway, sent as `Authorization: Bearer`. Only used when `OPENAI_GATEWAY_BASE_URL` is also set. |

**Anthropic-compatible gateway.** Some enterprises front an Anthropic Messages
gateway that authenticates with a bearer token instead of `x-api-key`. There is
no default endpoint: an unconfigured gateway does *not* fall back to
`api.anthropic.com`. The provider name stays available so BYOK installations can
point at their own gateway without deployment-level credentials.

**OpenAI-compatible gateway.** `openai_gateway` is the same arrangement one wire
family over: a customer endpoint speaking OpenAI Chat Completions, bearer auth,
no default endpoint.

An endpoint that publishes both surfaces is configured as two keys pointing at
the same base URL. Snowflake Cortex, for example, serves an Anthropic surface at
`/api/v2/cortex/v1/messages` and Chat Completions at
`/api/v2/cortex/v1/chat/completions`:

```bash
# Anthropic Messages surface (BYOK / gateway).
curl -sS -b jar -X POST https://<router>/admin/v1/provider-keys \
  -H 'content-type: application/json' \
  -d '{"provider":"anthropic_gateway","key":"<snowflake PAT>",
       "base_url":"https://<account>.snowflakecomputing.com/api/v2/cortex/v1"}'

# Chat Completions surface, under Cortex's own IDs.
curl -sS -b jar -X POST https://<router>/admin/v1/provider-keys \
  -H 'content-type: application/json' \
  -d '{"provider":"openai_gateway","key":"<snowflake PAT>",
       "base_url":"https://<account>.snowflakecomputing.com/api/v2/cortex/v1",
       "model_aliases":{"gpt-5":"openai-gpt-5"}}'
```

Both keys carry the same PAT; per model, the catalog's binding order decides
which surface serves it. A tenant that can't issue a long-lived PAT configures
each key with an RSA private key instead — see [Key-pair auth](#key-pair-auth)
— or with no secret at all, see
[Workload identity federation](#workload-identity-federation).

Each key may also carry its own endpoint, which overrides the deployment's base
URL for that provider on that installation's requests. Set it in **Settings →
Provider API keys → Endpoint URL**, or through the admin API:

```bash
# /admin/v1 mutations take the dashboard cookie, not an rk_ bearer.
curl -sS -c jar -X POST https://<router>/admin/v1/auth/login \
  -H 'content-type: application/json' -d '{"password":"<admin password>"}'

curl -sS -b jar -X POST https://<router>/admin/v1/provider-keys \
  -H 'content-type: application/json' \
  -d '{"provider":"openai_gateway","key":"<token>","base_url":"https://gateway.example.com/api"}'
```

The value must be an absolute `http(s)` URL; anything else is rejected with
`400`. A trailing slash is stripped, and the provider appends its own API path
(`/v1/messages` for the Anthropic family, `/chat/completions` for the OpenAI
one), so give the base only. Omit the field to keep the deployment endpoint —
except for `anthropic_gateway` and `openai_gateway`, which have no default to
fall back to and reject a key without one.

A key may also carry a model alias map for endpoints that publish the catalog's
models under their own names:

```bash
curl -sS -b jar -X POST https://<router>/admin/v1/provider-keys \
  -H 'content-type: application/json' \
  -d '{"provider":"openai_gateway","key":"<token>","base_url":"https://gateway.example.com/api",
       "model_aliases":{"moonshotai/kimi-k3":"internal.kimi-k3"}}'
```

Keys are catalog model IDs (an ID outside the deployed catalog is rejected with
`400`) and values are what goes on the wire to that endpoint. Only the outbound
model name changes: routing, pricing, and analytics stay keyed on the catalog
ID. Omit the field to send catalog IDs unchanged.

The map is editable in **Settings → Provider API keys → Edit aliases**, or on
its own endpoint, which replaces the whole map and leaves the stored secret
alone — so retargeting model names doesn't need the credential re-entered:

```bash
curl -sS -b jar -X PUT https://<router>/admin/v1/provider-keys/<key id>/model-aliases \
  -H 'content-type: application/json' \
  -d '{"model_aliases":{"moonshotai/kimi-k3":"internal.kimi-k3"}}'
```

### Key-pair auth

A gateway whose tenant forbids long-lived tokens can be given an RSA private
key instead: the router signs a short-lived RS256 JWT for the configured
principal and sends it as the bearer, re-signing well before the one-hour
ceiling upstreams like Snowflake impose on such tokens. The key is stored in
the same encrypted column as a PAT (see [BYOK encryption](#byok-encryption))
and is never returned by the API or rendered back in the dashboard.

```bash
curl -sS -b jar -X POST https://<router>/admin/v1/provider-keys \
  -H 'content-type: application/json' \
  -d '{"provider":"openai_gateway","auth_type":"keypair_jwt",
       "auth_account":"MYORG-MYACCOUNT","auth_user":"SERVICE_USER",
       "key":"-----BEGIN PRIVATE KEY-----\n...",
       "base_url":"https://<account>.snowflakecomputing.com/api/v2/cortex/v1"}'
```

The key must be an unencrypted PKCS#1 or PKCS#8 RSA key of at least 2048 bits —
passphrase-protected keys are rejected, since there is nobody to prompt. Its
public half must already be assigned to the upstream user (Snowflake:
`ALTER USER ... SET RSA_PUBLIC_KEY`), whose default role needs
`SNOWFLAKE.CORTEX_USER` (or `SNOWFLAKE.CORTEX_REST_API_USER`). Account locators
drop their region and cloud suffixes (`xy12345.us-east-1.aws` → `XY12345`);
org-qualified identifiers (`myorg-myaccount`) are used as they are. The same
fields are available in **Settings → Provider API keys → Authentication**;
`auth_type` defaults to `bearer`, which sends the stored secret verbatim as
today.

The minted token claims `iss = ACCOUNT.USER.SHA256:<public key fingerprint>`
and `sub = ACCOUNT.USER`, uppercased, valid 55 minutes and re-signed after 45,
so rotating the stored key takes effect on the next request rather than at the
old token's expiry. Only the auth type and principal are readable back:

```bash
curl -sS -b jar https://<router>/admin/v1/provider-keys
# {"keys":[{"provider":"openai_gateway","auth_type":"keypair_jwt",
#           "auth_account":"MYORG-MYACCOUNT","auth_user":"SERVICE_USER", ...}]}
```

A key whose token can't be signed (wrong secret pasted, unreadable key) is
dropped from that request's credentials rather than sent upstream as-is, so
routing falls back to another binding instead of leaking the key. Misconfigured
input is rejected at write time with a `400`: a non-RSA or under-2048-bit key,
a passphrase-protected key, a missing account or user, or key-pair auth on a
vendor provider (only `anthropic_gateway` and `openai_gateway` accept it).

### Workload identity federation

A tenant that wants no credential in the router's database at all can trust the
router's *own* cloud identity instead. The router attests itself per request —
a Google-signed ID token for the service account it runs as, or a projected
OIDC token mounted into its pod — and sends the attestation as the bearer:

```text
Authorization: Bearer WIF.GCP.<attestation>
X-Snowflake-Authorization-Token-Type: WORKLOAD_IDENTITY_FEDERATION
```

The attestation source is deployment-wide, not per key, because it identifies
the router process rather than a tenant:

| Variable                     | Default                  | Effect |
| ---------------------------- | ------------------------ | ------ |
| `ROUTER_WIF_PROVIDER`        | *(none)*                 | `GCP` or `OIDC`. Unset disables workload identity: a `wif` key is dropped from that request's credentials. Any other value aborts boot. |
| `ROUTER_WIF_AUDIENCE`        | `snowflakecomputing.com` | Audience of the minted GCP ID token. Snowflake requires the default; override only for a non-Snowflake upstream. |
| `ROUTER_WIF_OIDC_TOKEN_FILE` | *(none)*                 | Path to the projected token, re-read per request so a rotated token is picked up. Required when `ROUTER_WIF_PROVIDER=OIDC`; boot aborts without it. |

A key then carries no secret at all:

```bash
curl -sS -b jar -X POST https://<router>/admin/v1/provider-keys \
  -H 'content-type: application/json' \
  -d '{"provider":"openai_gateway","auth_type":"wif",
       "base_url":"https://<account>.snowflakecomputing.com/api/v2/cortex/v1"}'
```

Upstream, the service user is created without a password or key and is bound to
the workload's identity (Snowflake:
`CREATE SECURITY INTEGRATION ... TYPE = WORKLOAD_IDENTITY` plus a
`WORKLOAD_IDENTITY` on the user; see Snowflake's
[workload identity federation](https://docs.snowflake.com/en/user-guide/workload-identity-federation)
guide), with the same `SNOWFLAKE.CORTEX_USER` grant key-pair auth needs. Note
that every installation using `wif` authenticates as the *same* workload — the
router's — so spend attribution upstream is per deployment, not per tenant; use
`identity_header` below if the endpoint needs the calling user.

As with key-pair auth, a key whose attestation can't be obtained (no source
wired, metadata server unreachable, token file missing) is dropped from that
request's credentials rather than dispatched with an empty bearer. Passing
`key`, `auth_account`, or `auth_user` alongside `auth_type: "wif"` is rejected
with a `400` — the principal lives in the attestation, and a stored secret would
never be used. The mode is also available in **Settings → Provider API keys →
Authentication**.

An endpoint that authenticates the org rather than the person can be given the
calling user in a header of its choosing:

```bash
curl -sS -b jar -X POST https://<router>/admin/v1/provider-keys \
  -H 'content-type: application/json' \
  -d '{"provider":"anthropic_gateway","key":"<token>",
       "identity_header":"X-Caller-Identity","identity_header_format":"json"}'
```

`email` sends the bare address; `json` sends a percent-encoded JSON property bag
(`user_email`, `user_name`, `session_id`, `client_app`, empty fields omitted).
The header is set on the upstream request after the client's own headers, so a
caller can't attribute their turns to someone else by sending it themselves, and
nothing is sent when the request carries no identity. Naming a header the
request depends on (`Authorization`, `x-api-key`, `Host`, `Content-Type`,
`Content-Length`, `Accept`) is rejected with `400`. Omit both fields to forward
nothing — identity only ever reaches the endpoint configured to receive it.

In `selfhosted` mode BYOK is always active (it's the only credentialing path).
In `managed` mode it is opt-in per installation: the control plane sets
`byok_enabled` on the installation row, and until it does, the auth middleware
strips BYOK keys so a stored key can't spend against a deployment that bills
prepaid credits. Once enabled, a BYOK turn debits no inference cost (the
customer paid their own provider). A platform fee can be charged on top,
recorded as a separate `byok_fee` ledger row: set `BYOK_FEE_RATE` to a
fraction of upstream cost (e.g. `0.05` for 5%). The default is `0` — no
fee, and no `byok_fee` row is written.

## Postgres

Set `DATABASE_URL` directly, or compose it from the individual vars:

| Variable                   | Default                           | Purpose |
| -------------------------- | --------------------------------- | ------- |
| `DATABASE_URL`             | *(none)*                          | Full connection string (takes precedence). |
| `POSTGRES_USER`            | *(required if no `DATABASE_URL`)* | Username. |
| `POSTGRES_PASSWORD`        | *(required if no `DATABASE_URL`)* | Password. |
| `POSTGRES_DB`              | *(required if no `DATABASE_URL`)* | Database name. |
| `POSTGRES_HOST`            | *(required if no `DATABASE_URL`)* | Hostname. |
| `POSTGRES_PORT`            | `5432`                            | Port. |
| `POSTGRES_SSLMODE`         | `require`                         | TLS mode. Use `disable` for local Docker. |
| `POSTGRES_CONNECTION_NAME` | *(none)*                          | Cloud SQL Auth Proxy instance connection name. |

### Host mode (Supabase session pooler)

For local WSL or Build.io-parity runs without Compose Postgres:

- Point `DATABASE_URL` at the Supabase **session** pooler on port **5432** (`sslmode=require`).
- Do **not** use the **transaction** pooler on port **6543** for migrate or the Go `pgx` pool.
- Set `PUBSUB_DISABLED=true` and leave `PUBSUB_PROJECT_ID` unset. Setting `PUBSUB_PROJECT_ID` alone panics at boot.
- Run `make setup` then `make dev`. Skip `make db` and `make full-setup`.

Step-by-step: [HOST_WSL_SUPABASE.md](HOST_WSL_SUPABASE.md).

## Server

| Variable                 | Default      | Purpose |
| ------------------------ | ------------ | ------- |
| `PORT`                   | `8080`       | HTTP listen port. |
| `ROUTER_DEPLOYMENT_MODE` | `selfhosted` | `selfhosted` mounts `/ui/*` and `/admin/v1/*`. `managed` skips both (for SaaS deployments with a separate admin UI). `selfserve` mounts a self-service dashboard plane instead. |
| `ROUTER_RESTRICT_UPSTREAM_EGRESS` | follows `ROUTER_DEPLOYMENT_MODE` | When true, provider adapters refuse to dial an upstream that resolves outside the public internet (loopback, private, link-local, CGNAT). Defaults to true in `managed` mode and false in `selfhosted`/`selfserve`, where pointing a provider at an in-cluster or loopback gateway is normal. While on, provider adapters also ignore `HTTP_PROXY`/`HTTPS_PROXY` — a proxied connection makes the destination unverifiable, so a proxy set alongside `managed` mode is silently bypassed (traffic dials DIRECT). |
| `ROUTER_ADMIN_PASSWORD`  | `admin`      | Dashboard password. Defaults to `admin` with a startup warning when unset — **set this for any internet-facing deployment**. Not used in `selfserve` mode (see below). |

### Playground (`/ui/playground`)

The dashboard **Playground** (`/ui/playground`) is an interactive routing lab
available in `selfhosted` and `selfserve` modes only — it is **not** mounted in
`managed` deployments. Use it to preview routing decisions and send test chat
turns without leaving the dashboard.

**Auth.** Both playground endpoints live on the `/admin/v1/*` data plane and
require the same credentials as other dashboard mutations: an admin session
cookie (`selfhosted`) or account session cookie (`selfserve`). They do **not**
accept an `rk_` bearer alone.

| Endpoint | Purpose |
| --- | --- |
| `POST /admin/v1/playground/route` | Decision-only preview — no upstream call. |
| `POST /admin/v1/playground/chat` | Full chat dispatch through `ProxyOpenAIChatCompletion`. |

**Request shape (both endpoints).** OpenAI Chat Completions JSON:
`{"model":"auto","messages":[{"role":"user","content":"…"}]}`. Optional
`stream:true` on chat. Force-model precedence matches the product surface:
`model` field > `x-weave-force-model` header; `model:"auto"` or `model:null`
routes normally. Send `X-Playground-Session: <id>` on chat to pin a stable
per-browser session (`client_app="playground"`).

**Route preview response (200).** Six keys plus additive cache savings:

```json
{
  "model": "moonshotai/kimi-k2.7",
  "provider": "aiand",
  "reason": "cluster",
  "requested_cost_usd": 0.0012,
  "actual_cost_usd": 0.0008,
  "cache_savings_usd": 0.0001,
  "id": "<request_id>"
}
```

No `embeddings`, `metadata`, `prompt`, or other scorer internals are returned.

**Chat response.** Streaming (`stream:true`): `Content-Type: text/event-stream`,
upstream `data:` frames forwarded verbatim, terminated with `data: [DONE]`.
Non-streaming: pass-through JSON from the upstream. Mid-stream failures append
one final `data: {"type":"api_error",…,"code":"upstream_interrupted"}` event.

**Error envelope.** Failures use OpenAI-shaped JSON:

```json
{"error":{"message":"…","type":"invalid_request_error|api_error","param":null,"code":"rate_limit|insufficient_credits|provider_error|unavailable|routing_failed|…"}}
```

HTTP status mirrors the classification. `429` responses include `Retry-After`
(default `"1"` when the upstream omits it). `402` maps to
`code:"insufficient_credits"`; `503` maps to `code:"unavailable"`.

**Metrics.** Dashboard cost summaries include additive
`cache_input_savings_usd` on `GET /admin/v1/metrics/summary` (prompt-cache read
savings aggregated from telemetry).

**Smoke script.** `scripts/playground_smoke.sh` exercises login, route preview,
and a classified chat error against a local `selfhosted` router.

### Self-service mode (`ROUTER_DEPLOYMENT_MODE=selfserve`)

`selfserve` lets any user log into their own dashboard with their aiand `sk-`
API key instead of an operator password. Each aiand user authenticates against
aiand's public `GET /api/v1/me` identity endpoint and is mapped to their own
router installation (the account id doubles as the installation
`external_id`), so every row they see — metrics, API keys, BYOK provider keys,
config, model exclusions — is scoped to that installation.

What mounts, versus `selfhosted`:

| Surface | `selfhosted` | `selfserve` |
| --- | --- | --- |
| Static dashboard at `/ui/*` | Yes | Yes |
| `POST /admin/v1/auth/login` + `GET /admin/v1/auth/me` (operator password) | Yes | **Not mounted** |
| `POST /account/v1/login` + `/logout`, `GET /account/v1/me` (aiand-key) | No | Yes |
| `/admin/v1/*` dashboard data plane | Admin cookie or `rk_` bearer | Account cookie only |

The `/admin/v1/*` dashboard data plane has full parity between the two
modes: metrics, API keys, BYOK provider keys (including model-alias editing,
per-key model discovery, and the pre-save discover-models probe), routing
preferences, content-capture controls, and the excluded/allowed models +
providers surfaces all mount in `selfserve` exactly as they do in
`selfhosted`. The only intentional difference is the auth frontier
(operator password vs account cookie) and the login surface itself.

The operator password admin surface is deliberately absent in `selfserve`:
the dashboard authenticates through the account cookie, and `WithAccountCookie`
(not `WithAdminOnly`) gates the `/admin/v1/*` group. A valid `rk_` data-plane
key does **not** authorize it, and `/admin/v1/auth/login` isn't registered, so
there is no password avenue into the dashboard.

The account session cookie is `router_account_session`, HttpOnly with a
7-day TTL; `POST /account/v1/logout` clears it, after which `/admin/v1/*`
returns 401.

#### aiand identity probe

| Variable       | Default                    | Effect |
| -------------- | -------------------------- | ------ |
| `AIAND_IDENTITY_URL` | `https://api.aiand.com` | API root for the identity probe; the login key is validated against `GET <root>/v1/models`. This is the API **root**, not the OpenAI-compatible inference base in [Provider API keys](#provider-api-keys) (which uses `AIAND_API_URL` and already includes `/v1`). |

The probe validates an arbitrary user `sk-` key as a bearer token against
`/v1/models` — it never uses the deployment's own `AIAND_API_KEY`, so a
self-serve login can't spend platform budget. The same user key is stored as
installation BYOK and is what powers `GET /admin/v1/aiand/models` (Models page)
and the Playground — **`AIAND_API_KEY` is not required in env for those
surfaces in `selfserve`**. Responses map to the login outcomes: `200` → authenticated (identity stored), `401`/`403` → `invalid_key`,
`402` → `insufficient_credits`, `429` → rate-limited, anything else (including
a network failure) → `503 key_validation_unavailable`. Failed logins are
rate-limited per source IP (5 failures / 5 minutes).

#### Wipe on key revocation

`selfserve` treats key revocation as an account wipe. aiand's API exposes no
endpoint to retrieve a user's data or re-instantiate an account from a revoked
key, so when the user revokes their aiand key the router has no way to prove the
installation is still theirs and the account row is soft-deleted — with it, the
installation and all its data (API keys, BYOK secrets, config, metrics scoping)
are gone. The soft-delete keeps the row so audit trails survive.

Presenting a fresh aiand key for the same `aiand_user_id` re-instantiates a new
account + installation from scratch. There is no data restore: login never
keys into a deleted account (the unique index on `aiand_user_id` is partial on
`deleted_at IS NULL`).

Key **rotation** (logging in with a different valid key for the same aiand
user) does **not** wipe anything — the existing account and installation are
returned unchanged, so the dashboard shows the same data.

## Routing

| Variable                          | Default                      | Purpose |
| --------------------------------- | ---------------------------- | ------- |
| `ROUTER_DEFAULT_STRATEGY`         | `cluster`                    | Strategy used when an installation has no persisted strategy. Change only after the policy rollout gate passes. |
| `ROUTER_CLUSTER_VERSION`          | *(reads `artifacts/latest`)* | Pin a specific cluster artifact version (e.g. `v0.27`). |
| `ROUTER_CLUSTER_EMBED_TIMEOUT_MS` | `200`                        | Per-request ONNX embed timeout. Increase for slower hosts. |
| `ROUTER_EMBED_ONLY_USER_MESSAGE`  | `true`                       | Feed only user-role text to the embedder. Set `false` to embed the full concatenated turn. |
| `ROUTER_STICKY_DECISION_TTL_MS`   | `0` (disabled)               | Reuse a routing decision per API key for this many ms. |
| `ROUTER_SESSION_PIN_ENABLED`      | `true`                       | Pin a session to its first-routed model so multi-turn conversations stay coherent. |
| `ROUTER_HARD_PIN_MODEL`           | *(none)*                     | Force every request to a specific model, bypassing the cluster scorer. Debugging only. |
| `ROUTER_HARD_PIN_PROVIDER`        | *(none)*                     | Pair with `ROUTER_HARD_PIN_MODEL`. |
| `ROUTER_HARD_PIN_EXPLORE`         | `true`                       | Pin Claude Code Task-tool sub-agent turns to `ROUTER_HARD_PIN_MODEL`/`ROUTER_HARD_PIN_PROVIDER` (or the cheapest deployed model, if those are unset). Set `false` to route sub-agents through the scorer like any other turn. |
| `ROUTER_SUBAGENT_MODEL`           | *(none)*                     | Route Task-tool sub-agent turns to a distinct catalog model, independent of `ROUTER_HARD_PIN_MODEL` — e.g. a local OpenAI-compatible endpoint while the main loop keeps using whatever the scorer picks. Requires `ROUTER_SUBAGENT_PROVIDER`; either alone is ignored. Takes effect regardless of `ROUTER_HARD_PIN_EXPLORE`, but the HMM strategy keeps its own sub-agent handling and isn't affected. |
| `ROUTER_SUBAGENT_PROVIDER`        | *(none)*                     | Pair with `ROUTER_SUBAGENT_MODEL`. |
| `ROUTER_TRANSLATION_COMPATIBILITY_MODE` | `shadow` | Translation representability rollout: `off` disables broad filtering, `shadow` records candidate exclusions without changing routes, and `enforce` makes declared semantic requirements hard routing constraints. Native-only safety paths (such as unsupported Responses tool unions and native Gemini ingress) remain protected unless mode is `off`. |
| `ROUTER_SCOPED_SEARCH_REQUIREMENT` | `true` | Scopes the citations/search native-capability requirement to sessions that actually used a web-search tool this turn or recently, instead of every turn that merely advertises one. Advertised-only turns return to normal policy routing. |
| `ROUTER_SEARCH_REQUIREMENT_DECAY_TURNS` | `3` | With `ROUTER_SCOPED_SEARCH_REQUIREMENT`, how many routed turns after the last actual search-tool use keep the requirement before it decays. |
| `ROUTER_COMPACTION_PCT`           | `0.85`                       | Fraction of the largest eligible model's context window at which the proactive compaction cascade engages (clear old tool results → structured summary → trim). Range `(0,1]`; `0` disables compaction (over-window requests then 413). Mirrors Claude Code's ~0.85 auto-compact trigger. |
| `ROUTER_ONNX_ASSETS_DIR`          | `/opt/router/assets`         | Directory containing `model.onnx` + `tokenizer.json`. |
| `ROUTER_ONNX_LIBRARY_DIR`         | *(system default)*           | Path to `libonnxruntime` (e.g. `/opt/homebrew/lib` on Apple Silicon). |

If the cluster scorer can't run (missing model, embed timeout, etc.), the
router returns HTTP 503 — it does *not* silently fall back to a default
model. Failures are loud by design.

## Provider and model exclusions

Exclusions keep traffic away from a provider or model — the control to reach
for when an installation may only talk to, say, its own enterprise gateway.

| Variable                     | Default  | Purpose |
| ---------------------------- | -------- | ------- |
| `ROUTER_EXCLUDED_PROVIDERS`  | *(none)* | Comma-separated provider names no request may be routed to. Pins the list deployment-wide: per-installation edits are refused (403) while it is set. |
| `ROUTER_EXCLUDED_MODELS`     | *(none)* | Comma-separated model IDs no request may be routed to, same deployment-wide pinning. |

Without either env var the lists come from the installation, editable in the
dashboard or through `PUT /admin/v1/excluded-providers` and
`PUT /admin/v1/excluded-models`.

From a terminal, `npx @workweave/router models --claude` lists every deployed
model with its on/off state and `models enable` / `models disable` edit it,
reading the endpoint and key from the Claude Code install already on disk.
Claude Code gets the same thing as `/router-models` (alias `/models`). While
either env var is set the CLI surfaces the 403 verbatim rather than pretending
the edit landed. See [install/README.md](../install/README.md#choosing-which-models-the-router-may-pick).

Exclusions are authoritative, not a preference. An excluded provider is
subtracted from the request's eligible set before anything routes, so the
scorer, the turn-type hard pins, session pins, and cross-binding failover all
stay off it — including when the caller holds their own BYOK key for it.

That extends to explicit forcing. `/force-model` and the `x-weave-force-model`
header are refused when every provider that could serve the model is excluded:
the command answers with the reason and leaves routing (and any prior pin)
alone, and the header fails the request with HTTP 400. A model with one
permitted binding left is forced normally and served through that binding. A
live session whose forced pin is later excluded fails the same way rather than
quietly reverting to automatic routing — clear it with `/unforce-model`. The
same holds a level up: exclusions that empty a forced routing cluster fail the
request too (see [Forcing a model or a routing
cluster](#forcing-a-model-or-a-routing-cluster)).

Excluding every provider that serves the models you route to leaves requests
with nowhere to go (HTTP 503 from the scorer), so exclude deliberately.

## Forcing a model or a routing cluster

`/force-model <model>` (alias `/fm`) pins the session to one model, and the
inbound `model` field / `x-weave-force-model` header are its headless
equivalents (see [Routing intent via the `model` field](#routing-intent-via-the-model-field)).
The name is matched **exactly** — it must be a canonical catalog ID
(`qwen/qwen3.8-max`), that model's bare name without the vendor prefix
(`qwen3.8-max`), or an alias (`opus`, `qwen-max`), optionally with a `:level`
effort suffix (`opus:high`). There is no prefix, substring, or nearest-match
fallback: a name the router doesn't recognize is refused, never approximated.

That strictness is the point. Approximate matching served a model the caller
never named — `/fm qwen 3.8` resolved through the bare `qwen` alias to
`qwen/qwen3-coder` and acked as if the pin took. The whole rest of the command
line is now read as the model name, so that input is rejected instead. To pin
and prompt in one turn, put the prompt on the **next line**:

```
/force-model qwen/qwen3.8-max
now fix the failing test
```

Two request headers let a headless caller (eval harness, CI, any client whose
UI eats slash commands) override routing. The header is now a **fallback**:
the inbound `model` field is the primary mechanism (see [Routing intent via the
`model` field](#routing-intent-via-the-model-field)) — a resolvable `model`
field wins over a conflicting header, and the header only takes effect when the
field is empty or `auto`. Both carriers fail the request rather than routing
on, so a typo can't look like it took effect.

| Header | Effect |
| ------ | ------ |
| `x-weave-force-model` | COMpat/fallback path for `model="..."`. Pins the session to one model, exactly as `/force-model` does — same exact-match rule. Accepts a canonical catalog ID, a bare name, or an alias (`opus`, `gpt`, `qwen-max`, …) plus an optional `:level` effort suffix (`opus:high`). A value naming no catalog model is HTTP 400. |
| `x-weave-force-cluster` | Constrains serving to one of the policy sidecar's routing clusters, leaving the choice *within* it to the policy. |

`x-weave-force-cluster` takes an opaque label — the router holds no list of
valid ones. The live cluster vocabulary belongs to the deployed policy artifact
and changes when it does, so the only authority is the roster the sidecar
reports on that very request; a hardcoded list would silently go stale on the
next roster bump. Consequences:

- A label absent from the live roster is HTTP 400, whether it's a typo or a
  cluster the current artifact retired. Both are equally unservable.
- A label that *is* in the roster but has no eligible model for this request
  (everything in it excluded, over-window, or filtered out on capability) is
  also HTTP 400 — including when a per-key cluster model list empties it.
- The header only works on the `hmm` / `hmm_embedding` strategies. The default
  `cluster` strategy scores anonymous centroids with no named groups, so there
  is nothing to constrain to and the request is HTTP 400 rather than a silent
  no-op.
- A sidecar too old to report its clusters also 400s. The constraint can't be
  proven against a roster the router can't see, and serving anyway would ignore
  the force.

Unlike `x-weave-force-model` the cluster header writes no session pin: every
turn carrying it is constrained on its own merits. Which models make up a
cluster stays control-plane config (the dashboard's per-API-key "Cluster model
lists" panel) — the header only says *which* cluster this turn must come from,
and any list configured for that cluster still orders the arm that serves.

## Policy sidecars

Out-of-process policy routers use the versioned contract in
[Policy router harness](POLICY_ROUTER_HARNESS.md). The router remains the
authority for candidate eligibility, provider binding, dispatch, retries,
privacy context, and telemetry.

| Variable                           | Default | Purpose |
| ---------------------------------- | ------- | ------- |
| `ROUTER_POLICY_SIDECARS`           | *(none)* | JSON object mapping a new strategy ID to its sidecar origin, for example `{"quality-v2":"https://quality-v2.internal"}`. IDs must match `[a-z][a-z0-9_-]{0,63}`. At most 16 may be configured. `cluster`, `rl`, `hmm`, and `bandit` are reserved. |
| `ROUTER_POLICY_SIDECAR_AUTH`       | *(none)* | JSON object mapping configured generic strategy IDs to `none` or `google-id-token`, for example `{"quality-v2":"google-id-token"}`. Google ID-token mode uses the exact sidecar origin as token audience and fails router startup when application default credentials cannot build the client. |
| `ROUTER_POLICY_SIDECAR_TIMEOUT_MS` | `3000`  | Total timeout for each generic policy decision, including transient retries. Also bounds startup capability discovery. |
| `ROUTER_HMM_SIDECAR_URL`           | *(none)* | Legacy built-in HMM registration. Prefer the generic map for new strategies. |
| `ROUTER_HMM_SIDECAR_TIMEOUT_MS`    | `3000`  | Total HMM decision timeout. |
| `ROUTER_HMM_SIDECAR_ATTEMPT_TIMEOUT_MS` | 60% of the decision timeout | Bounds a single HMM attempt so one stalled sidecar instance cannot spend the whole decision budget before the retries run. Set it equal to `ROUTER_HMM_SIDECAR_TIMEOUT_MS`, or to `0`, to let one attempt use the full budget. |
| `ROUTER_HMM_SIDECAR_AUTH`          | `none`  | Authentication for the HMM sidecar. Use `google-id-token` for managed Cloud Run; the exact sidecar origin is used as the token audience. |
| `ROUTER_HMM_ROSTER_PATH`           | *(none)* | Path to a generated declarative roster JSON (`hmm_router_cluster_roster_v6`). When set, the roster is loaded and validated against the model catalog at startup (boot fails on any invalid arm) and a summary is logged. Load-and-validate only; nothing serves from it yet. |
| `ROUTER_RL_SIDECAR_URL`            | *(none)* | Legacy built-in RL registration. Prefer the generic map for new strategies. |
| `ROUTER_RL_SIDECAR_TIMEOUT_MS`     | `3000`  | Total RL decision timeout. |
| `ROUTER_RL_SIDECAR_MODAL_KEY`      | *(none)* | Optional Modal proxy token id (`Modal-Key`) when the RL sidecar is a Modal ASGI app with `requires_proxy_auth`. |
| `ROUTER_RL_SIDECAR_MODAL_SECRET`   | *(none)* | Optional Modal proxy token secret (`Modal-Secret`); required when `ROUTER_RL_SIDECAR_MODAL_KEY` is set. |

`GET /capabilities` is queried at router startup. A failed probe does not
silently remove the strategy: serving stays registered and fails closed if
`POST /route` is unavailable, while optional outcome and feedback callbacks
remain disabled until the next successful restart. This keeps persisted
rollout state visible without pretending that a different strategy served.

Policy route requests retry network failures and HTTP 500, 502, 503, and 504
up to three attempts within the configured total timeout. Other failures are
not retried. An unavailable or invalid policy decision returns HTTP 503; it
never falls back to cluster or another policy.

### Self-hosted frozen HMM sidecar

The repository includes an optional companion container under
`sidecars/hmm/`. Start it with `make up-hmm`; the normal `make up` and
`make full-setup` paths remain cluster-only. HMM is not selected unless an
operator explicitly chooses the `hmm` strategy.

| Variable | Default | Purpose |
| --- | --- | --- |
| `HMM_PACKAGE_URL` | Published `hmm-model-v1` GitHub Release asset | HTTPS URL for the portable frozen package. |
| `HMM_PACKAGE_PATH` | *(none)* | Local package path when running the sidecar outside Compose. Set exactly one of path or URL. |
| `HMM_PACKAGE_SHA256` | Pinned release digest in the sidecar image | Required digest for URL downloads; optional but recommended with a local path. |
| `HMM_ARTIFACT_CACHE_DIR` | `/tmp/workweave-hmm-artifacts` | Atomic download/extraction cache. |
| `HMM_EMBEDDING_PROVIDER` | `google` | `google` or `openai-compatible`. |
| `GOOGLE_API_KEY` | *(none)* | Google Gemini API key for the exact embedding model named by the artifact. |
| `HMM_EMBEDDING_BASE_URL` | *(none)* | Base URL for an OpenAI-compatible `/embeddings` endpoint. |
| `HMM_EMBEDDING_API_KEY` | *(none)* | Optional bearer token for that endpoint. |
| `HMM_EMBEDDING_MODEL` | Artifact model ID | Model sent to an OpenAI-compatible endpoint. |

The published v1 package is tied to `google/gemini-embedding-2` at 3,072
dimensions. Those embedding values are direct classifier features and define
the HMM emission space, so another 3,072-dimensional model is not a substitute.
At startup the sidecar embeds a fixed probe and compares it to the reference
vector stored in the artifact. Readiness fails closed when the endpoint serves
an incompatible vector space. A fully local embedder is supported only with a
separately trained package that declares and probes that embedder.

The self-hosted sidecar is frozen: it keeps only a bounded in-memory embedding
cache, advertises no learning/outcome/feedback callbacks, and never persists
request or response content.

Selection precedence is:

1. An authorized internal `x-weave-router-strategy` request override.
2. The installation's persisted strategy.
3. `ROUTER_DEFAULT_STRATEGY`.

The request header is ignored unless the installation explicitly enables
policy-header overrides. `x-weave-router-debug` follows the same authorization
rule and cannot enable training. Shadow decisions are always non-dispatching,
non-debug, and non-learning.

## BYOK encryption

| Variable                      | Default   | Purpose |
| ----------------------------- | --------- | ------- |
| `EXTERNAL_KEY_ENCRYPTION_KEY` | *(unset)* | Tink AES-256-GCM keyset (JSON) that encrypts customer-supplied upstream provider keys at rest. |

**If unset, BYOK secrets are stored unencrypted** and the router logs a
`WARN` at startup. Set this in any deployment that handles real customer
secrets. Generate with:

```bash
tinkey create-keyset --key-template AES256_GCM --out-format json
```

A *malformed* keyset still fails closed (the router refuses to boot); only a
genuinely absent value triggers the unencrypted bypass.

## Telemetry (OpenTelemetry)

The router exports per-request trace spans to any OTLP-compatible collector.
Each proxied request emits two spans (`router.decision` and `router.upstream`)
with routing decisions, token usage, cost estimates, and latency. Export is
async/non-blocking; when `OTEL_EXPORTER_OTLP_ENDPOINT` is unset, OTel is
fully disabled at zero runtime cost. Everything the router records leaves the
process over OTLP only — there is no hardcoded analytics endpoint.

### High-fidelity content capture (`router.call` log records)

When `WV_CAPTURE_CONTENT` is set, the router additionally emits a `router.call`
OTLP **log record** per upstream call to `${OTEL_EXPORTER_OTLP_ENDPOINT}/v1/logs`.
Each record carries the same routing/decision metadata as the spans plus the
call outcome, and — depending on the mode — the request/response bodies. This
is the ML-ready event stream (one record per LLM call, full inputs and
outputs). It is **opt-in**: with `WV_CAPTURE_CONTENT` unset (the default) no
log records are emitted and behavior is unchanged.

| Variable             | Default | Purpose |
| -------------------- | ------- | ------- |
| `WV_CAPTURE_CONTENT` | `off`   | `off` = no log records; `hashed` = metadata + SHA-256 content hashes (no raw text); `full` = metadata + raw request/response bodies. |
| `WV_CAPTURE_MAX_BYTES` | `1048576` | Max buffered response bytes; larger responses are dropped and flagged `io.truncated=true` (the client still receives the full stream). |

Captured bodies are in the client's native wire format (Anthropic / OpenAI /
Gemini, matching the inbound surface). The `router.deployment_mode` resource
attribute (`selfhosted` / `managed`) is stamped on every export so a collector
can branch redaction or content-opt-out by deployment.

`WV_CAPTURE_CONTENT` is the deployment-wide **ceiling**. An installation can
tighten it below that (`GET`/`PUT /admin/v1/content-capture`, body
`{"mode": "off" | "hashed" | "full"}`; `{"mode": null}` clears the override),
and the effective mode for a request is the stricter of the two — so a tenant
on a `full` deployment can opt down to `hashed` or `off`, but an installation
asking for `full` under a `hashed` deployment still gets `hashed`.


| Variable                         | Default      | Purpose |
| -------------------------------- | ------------ | ------- |
| `OTEL_EXPORTER_OTLP_ENDPOINT`    | *(disabled)* | Collector base URL (e.g. `https://api.honeycomb.io`). Required to enable. |
| `OTEL_EXPORTER_OTLP_HEADERS`     | *(none)*     | Comma-separated `key=value` headers (e.g. auth tokens). |
| `OTEL_EXPORTER_OTLP_TIMEOUT`     | `10000`      | Per-export HTTP timeout in ms. |
| `OTEL_SERVICE_NAME`              | `router`     | `service.name` resource attribute. |
| `OTEL_RESOURCE_ATTRIBUTES`       | *(none)*     | Comma-separated `key=value` resource attributes. |
| `OTEL_BSP_MAX_QUEUE_SIZE`        | `1000`       | Span queue capacity. Spans drop when full. |
| `OTEL_BSP_MAX_EXPORT_BATCH_SIZE` | `50`         | Max spans per OTLP POST. |
| `OTEL_BSP_SCHEDULE_DELAY`        | `500`        | Partial-batch flush interval in ms. |
| `OTEL_EXPORT_WORKERS`            | `2`          | Export-goroutine count (spans and logs each get this many workers). |

The first five follow the [OTel SDK env spec](https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/);
`OTEL_BSP_*` follows the [Batch Span Processor spec](https://opentelemetry.io/docs/specs/otel/trace/sdk/#batch-span-processor).
`OTEL_EXPORT_WORKERS` is a router-specific extension.

## Cluster-routing artifacts

Each embedder the cluster scorer can use needs two files at runtime —
`model.onnx` (INT8-quantized) and `tokenizer.json` — in its own subdirectory
of the assets root, keyed by embedder ID:

- `jina-v2-base-code-int8/` — from the public
  [`jinaai/jina-embeddings-v2-base-code`](https://huggingface.co/jinaai/jina-embeddings-v2-base-code)
  HuggingFace repo (Jina's own INT8 export; we don't maintain our own
  quantization). Default for every bundle through v0.66; the flat legacy
  layout (`<root>/model.onnx`) still resolves for this embedder.
- `qwen3-embedding-0.6b-int8/` — produced by `scripts/export_qwen3_onnx.py`
  (Qwen3-Embedding-0.6B with last-token pooling baked into the graph) and
  uploaded to the public
  [`weave-eng/qwen3-embedding-0.6b-onnx-router`](https://huggingface.co/weave-eng/qwen3-embedding-0.6b-onnx-router)
  HF repo. Only needed when serving a bundle whose `metadata.yaml` declares
  this embedder; the runtime loads embedders lazily.

Neither is committed to git.

**Docker (default):** the Dockerfile downloads the files at image build time
into `/opt/router/assets/<embedder-id>/`. Both repos are public — no token
needed (the optional `hf_token` secret still works for rate-limit headroom);
set `HF_QWEN_REPO=` (empty) to skip the Qwen pull for Jina-only deploys.

**`make dev` (host-mode hot reload):** fetch the Jina files once into a local
directory and point `ROUTER_ONNX_ASSETS_DIR` at it:

```bash
mkdir -p assets/jina-v2-base-code-int8
BASE="https://huggingface.co/jinaai/jina-embeddings-v2-base-code/resolve/516f4baf13dec4ddddda8631e019b5737c8bc250"
curl -L "$BASE/onnx/model_quantized.onnx" -o assets/jina-v2-base-code-int8/model.onnx
curl -L "$BASE/tokenizer.json" -o assets/jina-v2-base-code-int8/tokenizer.json
echo "ROUTER_ONNX_ASSETS_DIR=$(pwd)/assets" >> .env.local
```

To also serve Qwen bundles locally, run `scripts/export_qwen3_onnx.py
--out-dir assets/qwen3-embedding-0.6b-int8` (or download the uploaded export
into that directory).

The pinned revisions (`HF_MODEL_REVISION`, `HF_QWEN_REVISION`) in the
Dockerfile keep local dev and the container build on the same weights. Bump
deliberately if you want a newer export.

The committed cluster artifacts (centroids, rankings, model registry,
metadata) live under `internal/router/cluster/artifacts/v<X.Y>/`. The
`artifacts/latest` pointer selects the default served version;
`ROUTER_CLUSTER_VERSION` overrides per-deployment.
