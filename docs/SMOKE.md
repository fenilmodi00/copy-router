# Router pre-merge smoke suite

The smoke suite drives a live router with deterministic request fixtures against
the **aiand-only** catalog, asserting behavior that in-process unit and
conformance tests cannot see: HTTP status, response/usage shape, prompt-cache
accounting, decision headers, and (via a pinned `openai/gpt-oss-120b` call)
tool-schema translation on the OpenAI-compat path.

**Local default is host mode** (`make smoke-host`): an already-running router from
`make setup` / `make dev` against Supabase session pooler (`PUBSUB_DISABLED=true`,
no compose Postgres, no pubsub-emulator). See [HOST_WSL_SUPABASE.md](HOST_WSL_SUPABASE.md).
CI still uses the compose + MITM path for key-free cassette replay.

It exists because the regression class it targets is invisible to `go test`. Two
concrete examples that motivated it:

- #820 turned on router `cache_control` breakpoint injection for the
  Anthropic→Anthropic path and could emit breakpoint combinations that only the
  *real* upstream API rejects (a 5th breakpoint past the 4-cap, or a router `5m`
  breakpoint ordered before a client `ttl=1h` one → hard 400). #821 fixed it hours
  later. The conformance suite stops at `proxy.Service` with a mock upstream, so it
  never observed the 400.
- A tool with a genuinely typeless optional parameter (no `type`/`anyOf`/`enum`
  at all — by design, meaning "accept any JSON value") 400'd against the real
  OpenAI Responses API: `strictifyOpenAISchema`'s nullable-wrapping fallback
  wrapped the bare node in an invalid `anyOf` without checking it carried a
  strict-expressible type first. Caught unit-level in
  `internal/translate/strictify_openai_test.go`, and end-to-end in
  `smoke/openai_test.go` — the unit test proves the translator produces the
  right JSON; the smoke scenario proves the real API actually accepts it.

## Architecture: a record/replay proxy sits between the router and its providers

`smoke/mitmproxy/` is a small MITM (man-in-the-middle) forward proxy. The
router's HTTP transport already honors `http.ProxyFromEnvironment`
(`internal/providers/httputil`), so pointing the `server` container at it via
`HTTPS_PROXY` — and trusting its ephemeral CA via `SSL_CERT_DIR` — intercepts
every outbound call with **zero router code changes**. It mints a TLS leaf cert
per CONNECT-target hostname. With the aiand-only catalog, replay targets the
aiand OpenAI-compat upstream; replay CI needs no upstream API key.

Three modes (`SMOKE_PROXY_MODE`):

| Mode | What it does | Needs a key? |
|---|---|---|
| `replay-only` (CI default) | Serves cassettes committed under `smoke/mitmproxy/cassettes/`; a cache miss is a clean 502, not a hang | No |
| `record` | Always calls the real API and (re)writes cassettes | Yes (`AIAND_API_KEY`) |
| `replay-or-record` (local default) | Serves from cache, falls back to live + record on a miss | Only for the first run of a new scenario |

Cassettes are keyed by `sha256(method + path + body)`. The fixtures are
byte-deterministic (`smoke/fixtures/system_prompt.txt` never changes), so a
given scenario hashes identically run to run — this is what makes `replay-only`
CI runs deterministic and free. Response headers are sanitized before a
cassette is written (`Authorization` / `x-api-key` / org identifiers / rate-limit
and request-id noise never get persisted), so it's safe for these files to be
committed and reviewed in a normal PR diff.

This means the CI job needs **no provider API keys at all** for its normal
path-gated run — it replays what's already checked in. Keys are only needed to
*record*, which happens locally or in a scheduled nightly refresh
(`AIAND_API_KEY`).

## When it runs

- **Not on every PR.** The CI job (`.github/workflows/smoke.yml`) is path-gated to
  the regression-prone surfaces: `internal/proxy/**`, `internal/translate/**`,
  `internal/providers/**`, `internal/router/catalog/**`, `cmd/router/**`,
  `smoke/**`, `docker-compose.yml`, `Dockerfile`. Docs, artifacts, the HMM
  sidecar, and the frontend never trigger it. It runs in `replay-only` mode —
  no secret needed.
- **On demand** via the workflow's `workflow_dispatch` button.
- **Locally** before merging a risky router change, or to refresh cassettes:
  `make smoke` (replay-only by default) or
  `AIAND_API_KEY=… SMOKE_PROXY_MODE=record make smoke` (real API, updates
  cassettes).

When `SMOKE_ROUTER_KEY` is unset (e.g. bare `go test -tags smoke ./smoke/`
without the orchestrator), `TestMain` prints a skip message and exits 0 so the
package stays green in unit-test invocations.

## Running locally (host / Supabase)

Preferred on WSL and Build.io-shaped deploys. No Docker daemon required for the
suite itself.

```bash
make setup && make dev                              # router on :8080, Supabase
make smoke-host                                     # or: SMOKE_HOST=1 make smoke
# optional: reuse a key
SMOKE_ROUTER_KEY=rk_… make smoke-host
```

`scripts/smoke/run.sh` in host mode:

1. Checks `${SMOKE_BASE_URL:-http://localhost:8080}/health`.
2. Uses `SMOKE_ROUTER_KEY`, or seeds one with `go run ./cmd/seed` against
   `DATABASE_URL`.
3. Runs `go test -tags smoke -count=1 -v ./smoke/`.
4. Does not start or stop compose, MITM, or pubsub-emulator.

Host mode talks to whatever upstream the running router is wired for. With
`AIAND_API_KEY` in `.env.local` and no `HTTPS_PROXY`, that is live aiand (a few
cents). The Anthropic-native "overflow rejected cleanly" cache subtest is
skipped in host mode (aiand OpenAI-compat models do not hit that translate
gate). Cassette replay of that case stays on the compose MITM path.

Host mode also activates automatically when `SMOKE_ROUTER_KEY` is set and `/health`
is already up (so a second `make smoke` against a live `make dev` does not try
to boot compose).

## Running via compose (CI / cassette refresh)

```bash
make smoke                                          # compose + MITM, replay-only
AIAND_API_KEY=… SMOKE_PROXY_MODE=record make smoke  # refresh aiand cassettes
```

Compose path steps:

1. Writes `docker-compose.smoke-run.override.yml` with placeholder/real provider
   keys for the `server` service (no pubsub-emulator stub).
2. `docker compose -f docker-compose.yml -f docker-compose.smoke-run.override.yml
   -f smoke/mitmproxy/docker-compose.yml up -d --build server mitmproxy` and
   waits for `/health`.
3. `docker compose run --rm seed` and parses the `rk_…` router key.
4. `go test -tags smoke -count=1 -v ./smoke/`.
5. On failure, dumps the last ~150 ANSI-stripped `server` + `mitmproxy` log
   lines, then tears the stack down.

Iterating on a compose scenario:

```bash
SMOKE_KEEP_STACK=1 make smoke
SMOKE_ROUTER_KEY=rk_… go test -tags smoke -count=1 -v ./smoke/ -run TestCaching
docker compose -f docker-compose.yml -f docker-compose.smoke-run.override.yml \
  -f smoke/mitmproxy/docker-compose.yml down -v
rm -f docker-compose.smoke-run.override.yml
```

## Cost

`replay-only` runs make zero upstream calls. `record`/`replay-or-record` pin
every Messages scenario to `deepseek-ai/deepseek-v4-flash` and OpenAI-path
scenarios to `openai/gpt-oss-120b` (both via `x-weave-force-model` onto
provider `aiand`), and cap `max_tokens` — a full refresh is ~15 real calls, a
few cents.

## What it covers

See `smoke/*_test.go`. Defaults:

| Knob | Default |
|---|---|
| `SMOKE_PIN_MODEL` | `deepseek-ai/deepseek-v4-flash` |
| `SMOKE_OPENAI_PIN_MODEL` | `openai/gpt-oss-120b` |
| Expected `x-router-provider` | `aiand` |

## Adding a scenario

1. Add a `Test…` under `smoke/` (`smoke` build tag).
2. Prefer `newRequest(…).build(t)` + `call` / `callModel` helpers in `harness_test.go`.
3. Record once with `AIAND_API_KEY=… SMOKE_PROXY_MODE=record make smoke`, commit
   the new cassette under `smoke/mitmproxy/cassettes/`, and confirm
   `make smoke` (replay-only) stays green.
