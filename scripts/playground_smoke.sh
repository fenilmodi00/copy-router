#!/usr/bin/env bash
# Playground smoke: login, route preview, streaming chat with assistant text.
#
# Expects a router at BASE_URL. Does not start the router.
#
# Env:
#   BASE_URL                    default http://localhost:8080
#   ROUTER_DEPLOYMENT_MODE      selfhosted (default) or selfserve
#   ROUTER_ADMIN_PASSWORD       selfhosted admin password (default admin)
#   PLAYGROUND_ACCOUNT_KEY      selfserve: aiand sk- key for /account/v1/login
#   PLAYGROUND_SMOKE_SKIP_CHAT  set to 1 to skip chat
#   PLAYGROUND_SMOKE_MODEL      forced model for chat (default auto)
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
MODE="${ROUTER_DEPLOYMENT_MODE:-selfhosted}"
ADMIN_PASSWORD="${ROUTER_ADMIN_PASSWORD:-admin}"
CHAT_MODEL="${PLAYGROUND_SMOKE_MODEL:-auto}"
JAR="$(mktemp)"
trap 'rm -f "$JAR"' EXIT

log() { printf '\n\033[1;36m[playground-smoke]\033[0m %s\n' "$*"; }
fail() { printf '\n\033[1;31m[playground-smoke]\033[0m %s\n' "$*" >&2; exit 1; }

extract_openai_deltas() {
	python3 - "$1" <<'PY'
import json, sys
text = sys.argv[1]
out = []
for block in text.split("\n\n"):
    block = block.strip()
    if not block.startswith("data:"):
        continue
    payload = block[5:].strip()
    if payload == "[DONE]":
        continue
    try:
        obj = json.loads(payload)
    except json.JSONDecodeError:
        continue
    if "choices" not in obj:
        continue
    delta = (obj.get("choices") or [{}])[0].get("delta") or {}
    content = delta.get("content")
    if isinstance(content, str) and content and not content.startswith("✦ **Weave Router**"):
        out.append(content)
print("".join(out))
PY
}

log "health check ${BASE_URL}/health"
curl -sf "${BASE_URL}/health" >/dev/null || fail "router not healthy at ${BASE_URL}"

if [[ "$MODE" == "selfserve" ]]; then
	[[ -n "${PLAYGROUND_ACCOUNT_KEY:-}" ]] || fail "PLAYGROUND_ACCOUNT_KEY required for selfserve"
	log "account login"
	login_code="$(curl -sS -o /dev/null -w '%{http_code}' -c "$JAR" \
		-X POST "${BASE_URL}/account/v1/login" \
		-H 'content-type: application/json' \
		-d "{\"key\":\"${PLAYGROUND_ACCOUNT_KEY}\"}")"
	[[ "$login_code" == "200" ]] || fail "account login failed (HTTP ${login_code})"
else
	log "admin login"
	login_code="$(curl -sS -o /dev/null -w '%{http_code}' -c "$JAR" \
		-X POST "${BASE_URL}/admin/v1/auth/login" \
		-H 'content-type: application/json' \
		-d "{\"password\":\"${ADMIN_PASSWORD}\"}")"
	[[ "$login_code" == "200" ]] || fail "admin login failed (HTTP ${login_code})"
fi

log "POST /admin/v1/playground/route"
route_body="$(curl -sS -b "$JAR" \
	-X POST "${BASE_URL}/admin/v1/playground/route" \
	-H 'content-type: application/json' \
	-d '{"model":"auto","messages":[{"role":"user","content":"playground smoke"}]}')"
echo "$route_body" | grep -q '"model"' || fail "route response missing model"
echo "$route_body" | grep -q '"provider"' || fail "route response missing provider"
log "route preview OK"

if [[ "${PLAYGROUND_SMOKE_SKIP_CHAT:-0}" == "1" ]]; then
	log "skipping chat (PLAYGROUND_SMOKE_SKIP_CHAT=1)"
	exit 0
fi

log "POST /admin/v1/playground/chat (stream:true, model=${CHAT_MODEL})"
chat_out="$(mktemp)"
trap 'rm -f "$JAR" "$chat_out"' EXIT
chat_code="$(curl -sS -o "$chat_out" -w '%{http_code}' -b "$JAR" \
	-X POST "${BASE_URL}/admin/v1/playground/chat" \
	-H 'content-type: application/json' \
	-H 'X-Playground-Session: playground-smoke' \
	-H 'X-Weave-Routing-Marker: off' \
	-d "{\"model\":\"${CHAT_MODEL}\",\"messages\":[{\"role\":\"user\",\"content\":\"Reply with exactly: pong\"}],\"stream\":true}")"

chat_body="$(cat "$chat_out")"
if [[ "$chat_code" == "200" ]]; then
	assistant_text="$(extract_openai_deltas "$chat_body")"
	if [[ -n "$assistant_text" ]]; then
		log "chat stream returned assistant text (${#assistant_text} chars)"
	elif echo "$chat_body" | grep -q '\[DONE\]'; then
		fail "chat 200 with [DONE] but no assistant delta text"
	elif echo "$chat_body" | grep -q '"error"'; then
		log "chat returned classified error envelope in body"
	else
		fail "chat 200 but no assistant text or [DONE]: ${chat_body:0:200}"
	fi
elif echo "$chat_body" | grep -q '"error"'; then
	log "chat classified error (HTTP ${chat_code})"
else
	fail "chat unexpected response (HTTP ${chat_code}): ${chat_body:0:300}"
fi

log "playground smoke passed"
