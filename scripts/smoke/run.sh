#!/usr/bin/env bash
# Router pre-merge smoke suite orchestrator.
#
# Two modes:
#   Host (preferred locally): drive an already-running router from `make setup` /
#   `make dev` against Supabase. No docker compose, no pubsub-emulator.
#   Compose (CI / full stack): boots docker compose + the MITM record/replay
#   proxy (smoke/mitmproxy/) and seeds a key.
#
# Used by `make smoke`, `make smoke-host`, and .github/workflows/smoke.yml.
#
# Env:
#   SMOKE_HOST=1          skip compose; require a healthy router at SMOKE_BASE_URL
#                          (also auto-selected when SMOKE_ROUTER_KEY is set and
#                          /health is already up)
#   SMOKE_ROUTER_KEY      rk_… key (required in host mode unless seed can run)
#   SMOKE_BASE_URL        default http://localhost:8080
#   SMOKE_PROXY_MODE      replay-only (default) | record | replay-or-record
#                          record / replay-or-record need AIAND_API_KEY (compose
#                          MITM path). Host mode hits whatever upstream the
#                          running router is configured for (live aiand when
#                          HTTPS_PROXY is unset).
#   AIAND_API_KEY         required only when SMOKE_PROXY_MODE != replay-only
#                          on the compose path (ANTHROPIC_API_KEY still accepted
#                          as a legacy alias)
#   SMOKE_KEEP_STACK=1    leave the compose stack running after the tests
#   SMOKE_PIN_MODEL       model every Messages scenario pins
#   SMOKE_OPENAI_PIN_MODEL  model for OpenAI-path scenarios
#   SMOKE_CI_CACHE=1      layer-cache builds via GitHub Actions cache (CI only)
#
# Cost: compose replay-only makes zero upstream calls. Host mode without MITM
# spends real aiand tokens (~few cents for a full suite).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

OVERRIDE_FILE="$REPO_ROOT/docker-compose.smoke-run.override.yml"
BASE_URL="${SMOKE_BASE_URL:-http://localhost:8080}"
PROXY_MODE="${SMOKE_PROXY_MODE:-replay-only}"

log() { printf '\n\033[1;36m[smoke]\033[0m %s\n' "$*"; }
err() { printf '\n\033[1;31m[smoke]\033[0m %s\n' "$*" >&2; }

router_healthy() {
  curl -sf "${BASE_URL}/health" >/dev/null 2>&1
}

parse_router_key() {
  printf '%s\n' "$1" | grep -oE 'rk_[A-Za-z0-9_-]+' | head -1
}

seed_router_key_host() {
  log "seeding a router key via go run ./cmd/seed"
  local out
  out="$(go run ./cmd/seed 2>&1)" || {
    err "host seed failed:"
    printf '%s\n' "$out" >&2
    exit 1
  }
  parse_router_key "$out"
}

case "$PROXY_MODE" in
  replay-only) ;;
  record|replay-or-record)
    if [[ -z "${AIAND_API_KEY:-${ANTHROPIC_API_KEY:-}}" ]]; then
      err "SMOKE_PROXY_MODE=$PROXY_MODE needs AIAND_API_KEY or ANTHROPIC_API_KEY (only replay-only runs key-free on the compose MITM path)."
      exit 2
    fi
    ;;
  *)
    err "invalid SMOKE_PROXY_MODE=$PROXY_MODE (want replay-only | record | replay-or-record)"
    exit 2
    ;;
esac

HOST_MODE=0
if [[ "${SMOKE_HOST:-0}" == "1" ]]; then
  HOST_MODE=1
elif [[ -n "${SMOKE_ROUTER_KEY:-}" ]] && router_healthy; then
  HOST_MODE=1
  log "router already healthy at ${BASE_URL} with SMOKE_ROUTER_KEY set — host mode (no compose)"
fi

run_tests() {
  local key="$1"
  log "running the smoke suite (proxy mode: $PROXY_MODE, host=${HOST_MODE})"
  SMOKE_ROUTER_KEY="$key" \
  SMOKE_BASE_URL="$BASE_URL" \
  SMOKE_OPENAI_ENABLED="$( [[ -n "${OPENAI_API_KEY:-}" || "$PROXY_MODE" == "replay-only" || "$HOST_MODE" == "1" ]] && echo 1 || echo 0 )" \
    go test -tags smoke -count=1 -v ./smoke/
  log "smoke suite passed"
}

if [[ "$HOST_MODE" == "1" ]]; then
  if ! router_healthy; then
    err "SMOKE_HOST=1 but ${BASE_URL}/health is not up. Start the host router first (make setup && make dev)."
    exit 1
  fi
  log "host mode: using router at ${BASE_URL} (no docker compose)"
  ROUTER_KEY="${SMOKE_ROUTER_KEY:-}"
  if [[ -z "$ROUTER_KEY" ]]; then
    ROUTER_KEY="$(seed_router_key_host)"
  fi
  if [[ -z "$ROUTER_KEY" ]]; then
    err "could not obtain an rk_... key (set SMOKE_ROUTER_KEY or ensure go run ./cmd/seed works against DATABASE_URL)"
    exit 1
  fi
  log "using router key ${ROUTER_KEY:0:8}…"
  export SMOKE_HOST=1
  run_tests "$ROUTER_KEY"
  exit 0
fi

# --- compose path (CI / full stack) ---

COMPOSE_FILES=(-f docker-compose.yml -f "$OVERRIDE_FILE" -f smoke/mitmproxy/docker-compose.yml)
if [[ "${SMOKE_CI_CACHE:-0}" == "1" ]]; then
  COMPOSE_FILES+=(-f smoke/mitmproxy/docker-compose.ci-cache.yml)
fi
COMPOSE="docker compose ${COMPOSE_FILES[*]}"

# Router registers only aiand (AIAND_API_KEY). Replay-only uses a placeholder
# so the deployment has a keyed provider; MITM never calls upstream.
SERVER_AIAND_KEY="${AIAND_API_KEY:-${ANTHROPIC_API_KEY:-sk-aiand-smoke-placeholder-unused-in-replay-only}}"

cleanup() {
  local code=$?
  if [[ $code -ne 0 ]]; then
    err "smoke run failed (exit $code); dumping server + mitmproxy logs:"
    $COMPOSE logs server mitmproxy --since=10m 2>&1 | sed -E 's/\x1b\[[0-9;]*m//g' | tail -150 || true
  fi
  if [[ "${SMOKE_KEEP_STACK:-0}" == "1" ]]; then
    log "SMOKE_KEEP_STACK=1 — leaving the stack up. Tear down with: $COMPOSE down -v"
  else
    $COMPOSE down -v >/dev/null 2>&1 || true
    rm -f "$OVERRIDE_FILE"
  fi
  exit $code
}
trap cleanup EXIT

log "writing $(basename "$OVERRIDE_FILE")"
cat > "$OVERRIDE_FILE" <<EOF
services:
  server:
    environment:
      AIAND_API_KEY: "${SERVER_AIAND_KEY}"
EOF

log "building and starting the router stack (proxy mode: $PROXY_MODE)"
SMOKE_PROXY_MODE="$PROXY_MODE" $COMPOSE up -d --build server mitmproxy

log "waiting for /health at ${BASE_URL}"
deadline=$((SECONDS + 120))
until router_healthy; do
  if (( SECONDS >= deadline )); then
    err "router did not become healthy within 120s"
    exit 1
  fi
  sleep 2
done
log "router healthy"

log "seeding a router key"
SEED_OUTPUT="$($COMPOSE run --rm seed 2>/dev/null)"
ROUTER_KEY="$(parse_router_key "$SEED_OUTPUT")"
if [[ -z "$ROUTER_KEY" ]]; then
  err "could not parse an rk_... key from the seed output:"
  printf '%s\n' "$SEED_OUTPUT" >&2
  exit 1
fi
log "seeded router key ${ROUTER_KEY:0:8}…"

run_tests "$ROUTER_KEY"

if [[ "$PROXY_MODE" != "replay-only" ]]; then
  if ! git diff --quiet -- smoke/mitmproxy/cassettes/ 2>/dev/null || \
     [[ -n "$(git status --porcelain -- smoke/mitmproxy/cassettes/ 2>/dev/null)" ]]; then
    log "cassettes changed — review and commit smoke/mitmproxy/cassettes/ if this run should update the fixtures"
  fi
fi
