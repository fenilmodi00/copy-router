# Router (provider glossary)

Shared language for providers, wire families, catalog IDs, client aliases, harnesses, and ingress surfaces on the aiand-only deploy.

## Language

**Provider**:
Named upstream credential and dispatch identity (`aiand`, `anthropic`, `openai`, gateways, and other BYOK/fixture names). Deploy baseline registers **aiand** only. Other provider names remain for wire-family / BYOK / gateway / test fixtures.
_Avoid_: Collapsing `ProviderAnthropic` into `ProviderAiand`; treating Anthropic as the deploy upstream

**TranslationFamily**:
Wire-format family a provider speaks (OpenAI-compat vs Anthropic Messages, and kin). Cross-format translation and dispatch key off family, not an enumerated provider-name list.
_Avoid_: Equating family with provider name; assuming Anthropic wire implies an Anthropic upstream

**CatalogModelID**:
Canonical model identity in the deploy catalog (open-weight rows such as `moonshotai/…`, `z-ai/…`, `deepseek-ai/…`). Routing, pricing, and pins speak catalog IDs. Claude-era strings are not catalog rows.
_Avoid_: Keeping Claude catalog rows so a harness can keep sending Claude names

**UpstreamID**:
Model ID the chosen provider binding sends on the wire to that upstream. May match the catalog ID or differ when the upstream’s published name diverges (example: catalog `moonshotai/kimi-k2.7` vs upstream `moonshotai/kimi-k2.7-code`).
_Avoid_: Treating every client-facing string as an upstream ID; inventing Claude upstream IDs for aiand

**ClientModelAlias**:
Retired **catalog** IDs kept resolvable via `catalog.aliases` (e.g. `zai-org/glm-5.2` → `zai-org/glm-5.3`). Not a Claude-era short-name table — those remap inputs were deleted; unknown force-model values 400.
_Avoid_: Documenting Appendix-F Claude→catalog remaps that no longer exist in code; treating Claude harness names as catalog rows

**ClientHarness**:
Optional client tool that speaks a peripheral wire (notably Claude Code on Anthropic Messages). A harness is not a provider and not a reason to keep Claude catalog rows; it reaches aiand through translation. Pin with catalog IDs, not Claude short names.
_Avoid_: Treating Claude Code as deploy baseline; equating harness presence with Anthropic-as-upstream

**IngressSurface**:
HTTP product surface clients call. Primary: OpenAI-compatible `/v1/chat/completions`. Peripheral: `/v1/messages` (Anthropic wire in, translated before aiand OpenAI-compat dispatch). Ingress is not the same as Provider or TranslationFamily.
_Avoid_: Treating `/v1/messages` as a second upstream; leading with Anthropic ingress as the product story

## Identity & login

**hosted mode**:
The single deployment mode. Dashboard authenticates by aiand user key; each aiand user maps 1:1 to their own router installation.
_Avoid_: Inventing additional deployment modes (selfhosted/managed are gone)

**Account**:
The dashboard identity. One Account per aiand user, created/looked-up at login via the aiand identity probe. Soft-deleted when the user revokes their aiand key (the router can no longer prove the installation is theirs).
_Avoid_: Equating Account with Installation; the account is the login identity, the installation is the tenancy row

**Account ↔ Installation invariant**:
`account.id` doubles as the installation `external_id` — a 1:1 mapping. Every dashboard row (metrics, API keys, BYOK, config, exclusions) is scoped to that installation; there is no account-without-installation or installation-without-account state.
_Avoid_: Introducing a many-accounts-per-installation or many-installations-per-account shape

**LoginSession**:
The 7-day TTL session cookie (`router_account_session`, HttpOnly) minted by `POST /account/v1/login` after the aiand probe succeeds. `POST /account/v1/logout` clears it; `WithAccountCookie` rejects requests without a valid one with 401.
_Avoid_: Treating the `rk_` data-plane bearer as a dashboard credential; it never authorizes the dashboard data plane

**WithAccountCookie**:
Resolves the LoginSession cookie to the Account, then to its installation, and stashes both on ctx so the existing per-installation admin handlers scope unchanged. A valid `rk_` data-plane key does **not** satisfy it.
_Avoid_: Adding `rk_`-bearer acceptance to the dashboard `/v1/*` group; that would blur the dashboard-vs-data-plane credential boundary

