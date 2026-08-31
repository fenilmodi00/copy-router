# Router pre-merge smoke suite

The smoke suite boots the real router (docker compose stack) and drives it with
deterministic, Claude-Code-shaped request fixtures against the **aiand-only**
catalog, asserting the behavior that in-process unit and conformance tests
cannot see: HTTP status, response/usage shape, prompt-cache accounting,
decision headers (`provider=aiand`), and (via an OpenAI-ingress scenario) tool
schema translation that still lands on an aiand upstream.

## Architecture: a record/replay proxy sits between the router and its providers

`smoke/mitmproxy/` is a small MITM forward proxy. The router's HTTP transport
honors `http.ProxyFromEnvironment` (`internal/providers/httputil`), so pointing
the `server` container at it via `HTTPS_PROXY` — and trusting its ephemeral CA
via `SSL_CERT_DIR` — intercepts every outbound call with **zero router code
changes**. It mints a TLS leaf cert per CONNECT-target hostname, so the same
proxy intercepts calls to the aiand OpenAI-compat host (and any other upstream
hostname the router dials).

Three modes (`SMOKE_PROXY_MODE`):

| Mode | What it does | Needs a key? |
|---|---|---|
| `replay-only` (CI default) | Serves cassettes under `smoke/mitmproxy/cassettes/`; a cache miss is a clean 502 | No |
| `record` | Always calls the real API and (re)writes cassettes | Yes (`AIAND_API_KEY`) |
| `replay-or-record` (local default) | Serves from cache, falls back to live + record on a miss | Only for the first run of a new scenario |

Cassettes are keyed by `sha256(method + path + body)`, with volatile fields
removed from the body first (`normalizeRequestBody` in
`smoke/mitmproxy/store.go` — today only `prompt_cache_key`, the router's
session-affinity hint, which is derived from the API key id the smoke script
mints fresh on every run). Fixtures are otherwise
byte-deterministic (`smoke/fixtures/system_prompt.txt` never changes), so a
given scenario hashes identically run to run. A request field that varies per
run has to be added to `volatileBodyFields` or every cassette for that path
becomes a permanent miss. Response headers are sanitized
before a cassette is written (`Authorization` / `x-api-key` / org identifiers /
rate-limit and request-id noise never get persisted).

**Replay-only needs no provider API keys** — including no `ANTHROPIC_API_KEY`.
Native Anthropic / OpenAI cassette requirements are retired with the aiand-only
catalog. Re-record against aiand before relying on committed cassettes after a
pin-model or fixture change.

## When it runs

- **Not on every PR.** The CI job (`.github/workflows/smoke.yml`) is path-gated to
  `internal/proxy/**`, `internal/translate/**`, `internal/providers/**`,
  `internal/router/catalog/**`, `cmd/router/**`, `smoke/**`, `docker-compose.yml`,
  `Dockerfile`. It runs in `replay-only` mode — no secret needed.
- **On demand** via the workflow's `workflow_dispatch` button.
- **Locally** before merging a risky router change, or to refresh cassettes:
  `make smoke` (replay-only by default) or
  `AIAND_API_KEY=… SMOKE_PROXY_MODE=record make smoke`.

## Running locally

```bash
make smoke                                          # replay-only, no key needed
AIAND_API_KEY=… SMOKE_PROXY_MODE=record make smoke  # refresh aiand cassettes
```

Defaults:

- `SMOKE_PIN_MODEL=deepseek-ai/deepseek-v4-flash` (Claude Code `/v1/messages` path)
- `SMOKE_OPENAI_PIN_MODEL=openai/gpt-oss-120b` (OpenAI-ingress path → still `provider=aiand`)

That runs `scripts/smoke/run.sh`, which:

1. Writes an ephemeral compose override and sets deployment keys (placeholders in
   `replay-only`; real `AIAND_API_KEY` in `record` / `replay-or-record`).
2. `docker compose … up -d --build server mitmproxy` and waits for `/health`.
3. `docker compose run --rm seed` and parses the `rk_…` router key.
4. `go test -tags smoke -count=1 -v ./smoke/`.
5. On failure, dumps recent `server` + `mitmproxy` logs, then tears the stack down.

Iterating on a scenario? Keep the stack up between runs:

```bash
SMOKE_KEEP_STACK=1 make smoke
# ...edit a scenario...
SMOKE_ROUTER_KEY=rk_… go test -tags smoke -count=1 -v ./smoke/ -run TestCaching
# tear down when done:
docker compose -f docker-compose.yml -f smoke/mitmproxy/docker-compose.yml down
```

Without `SMOKE_ROUTER_KEY` (no stack), `go test -tags smoke ./smoke/` exits 0 and
skips — so package-level unit verify does not require Compose.

## Adding a scenario

1. Add `smoke/<name>_test.go` with the `//go:build smoke` tag.
2. Prefer `cfg.PinModel` / `assertServedByPin` so decisions stay on aiand.
3. Record once with `SMOKE_PROXY_MODE=record` and commit the new cassettes.
4. Confirm `SMOKE_PROXY_MODE=replay-only` (no `AIAND_API_KEY`) still passes.
