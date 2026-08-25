# Weave Router — build.io Deployment Guide

Deploy the router as a **single Docker container** on [build.io](https://www.build.io). Postgres comes from **Supabase** via a manual `DATABASE_URL` config var (no Schema To Go / Ave To Go addon).

## Current status

| Item | Status |
|------|--------|
| build.io app `router` | Exists (`ap-northeast-1`, stack `dockerfile`) |
| Default URL | `https://router-<app-id>.onbld.com` (example: `https://router-568f5bb0.onbld.com`) |
| Postgres | **Supabase** session-pooler `DATABASE_URL` (not a Build.io DB addon) |
| Pub/Sub | **Removed** (adapter deleted; leave all `PUBSUB_*` unset; `SERVER_REPLICAS=1`) |
| Env on Build.io | Set from `.env.production.local` / `.env.buildio` |

## Architecture

```
Internet ──► build.io LB (TLS) ──► router dyno ($PORT)
                                         │
                                         ▼
                              Supabase Postgres (DATABASE_URL)
```

The GCP Pub/Sub adapter (`internal/pubsub`) is deleted. Installation cache invalidation is always NoOp. Run a single replica; the 5-minute cache TTL is the safety net. Leave all `PUBSUB_*` unset (they are ignored if present).

## Prerequisites

1. [build.io](https://www.build.io/dashboard) account (team `fenil`, app `router`)
2. Supabase project (org `fenil`) with session-pooler URI
3. Secrets in `.env.production.local` (gitignored)
4. Optional: Docker Hub + `build_and_push.ps1` if you prefer a prebuilt image

Optional CLI: [bld](https://docs.build.io) for config/deploy.

---

## Step 1: Supabase Postgres

1. Create/open a Supabase project (same region as the app when possible: `ap-northeast-1`).
2. Dashboard → **Connect** → **Session** pooler (`:5432`). Copy the URI.
3. Set Build.io config var `DATABASE_URL` to that URI (`sslmode=require` if not already present).
4. **Do not** attach Schema To Go or Ave To Go. They would compete on `DATABASE_URL`.

### Migrations (required once)

The `/server` binary does **not** migrate on boot. Against Supabase, run migrate from a machine that can reach the DB (direct or session pooler `:5432`):

```bash
# Example: migrate CLI with DATABASE_URL from Supabase Connect
migrate -path db/migrations -database "$DATABASE_URL" up
```

Or use your usual `make migrate-up` if `DATABASE_URL` is exported locally.

Avoid Supabase **transaction** pooler (`:6543`) for migrate and for the Go `pgx` pool unless you disable prepared statements.

---

## Step 2: Secrets template

```powershell
Copy-Item .env.buildio .env.production.local
# Edit: AIAND_API_KEY, ROUTER_ADMIN_PASSWORD, EXTERNAL_KEY_ENCRYPTION_KEY, DATABASE_URL
```

| Variable | Purpose |
|----------|---------|
| `DATABASE_URL` | Supabase session-pooler URI |
| `AIAND_API_KEY` | Provider key (selfhosted) |
| `ROUTER_ADMIN_PASSWORD` | Admin UI |
| `EXTERNAL_KEY_ENCRYPTION_KEY` | Tink AES-256-GCM JSON for BYOK |
| `SERVER_REPLICAS` | `1` (single-replica; no cross-replica invalidation) |

Generate Tink keyset:

```bash
tinkey create-keyset --key-template AES256_GCM --out-format json
```

Never commit `.env.production.local`.

---

## Step 3: Image

### Preferred: Build.io native Dockerfile

App stack is already `dockerfile`. Connect the GitHub repo on the Deploy tab and deploy the branch. Build uses root `Dockerfile` / `heroku.yml`.

### Optional: Docker Hub prebuild

```powershell
docker login
.\build_and_push.ps1 -DockerHubUser YOUR_DOCKERHUB_USERNAME -Tag v1 -Push
```

Then set the container image in the dashboard (or `CONTAINER_IMAGE` if your Build.io plan uses that) to `YOUR_DOCKERHUB_USERNAME/weave-router:v1`.

---

## Step 4: Config vars on Build.io

Set from `.env.production.local`. Minimum keys: `DATABASE_URL`, `SERVER_REPLICAS=1`, plus provider/admin/Tink secrets.

**Leave unset** all `PUBSUB_*` vars (`PUBSUB_DISABLED`, `PUBSUB_PROJECT_ID`, `PUBSUB_EMULATOR_HOST`, topic/subscription). The adapter is gone; leftover values are ignored and are not required for boot.

### Option A: helper script (API token)

Create an API token in Build.io Account settings. Then:

```powershell
$env:BUILDIO_API_TOKEN = '<token>'
$env:DATABASE_URL = '<supabase-session-pooler-uri>'
.\scripts\buildio-set-safe-config.ps1 -EnvFile .env.production.local
```

The script PATCHes safe defaults and DELETEs leftover `PUBSUB_*` keys.

### Option B: `bld` CLI

```bash
bld login
bld config:set DATABASE_URL="<supabase-session-pooler-uri>" --app router
bld config:set SERVER_REPLICAS=1 PORT=8080 --app router
# plus AIAND_API_KEY, ROUTER_ADMIN_PASSWORD, EXTERNAL_KEY_ENCRYPTION_KEY, ...
```

### Option C: Dashboard

App `router` → Settings → Config Vars. Paste the same keys manually.

---

## Step 5: Deploy and verify

```bash
bld deploy router
bld logs router --tail 100
curl -skSf https://router-568f5bb0.onbld.com/health
curl -skSf https://router-568f5bb0.onbld.com/v1/version
```

Use `-k` if the platform cert is expired/misissued while debugging. Fix ACM under Overview → Hosts for production.

Boot should log: `Installation cache invalidation is NoOp; single-replica OK (5-min TTL)`.

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Liveness |
| `GET /v1/version` | Build SHA / cluster version |
| `POST /v1/messages` | Anthropic-compatible |
| `POST /v1/chat/completions` | OpenAI-compatible |
| `GET /ui/*` | Admin (selfhosted) |

---

## Pub/Sub adapter removed

The GCP Pub/Sub package (`internal/pubsub`) and compose emulator (`db/pubsub`) are deleted. There is no runtime path to turn Pub/Sub back on.

**What that means:** no cross-replica API-key / cluster-list cache invalidation (stale up to ~5 min) and no managed autopay Pub/Sub signals. Fine for one dyno. Keep `SERVER_REPLICAS=1`.

---

## Troubleshooting

### 502 / TLS / clients cannot connect

1. Confirm the hostname is `https://router-<app-id>.onbld.com` (Overview → Go), not `router.onbld.com`.
2. If curl reports `SEC_E_CERT_EXPIRED` / certificate expired, fix ACM under Overview → Hosts. Until then, debugging may need `curl -k` (do not ship that to clients).
3. `bld logs router` — look for missing env or DB connection errors
4. Confirm `DATABASE_URL` is Supabase session pooler and migrations ran
5. Confirm image listens on `$PORT` (Build injects it)

### Container exits immediately

1. `bld logs router` — look for missing env var or DB connection errors
2. Confirm image tag / Dockerfile deploy succeeded
3. Confirm secrets are set (see Step 4)

### Rate limit / 429

App analytics rate limit returns **429**, not 502. Upstream provider 429s are retried/failover. Persistent 502 with empty logs usually means crash-loop (boot panic), not rate limit.

### Wrong database addon

Detach Schema To Go / Ave To Go. Use only manual Supabase `DATABASE_URL`.

---

## Files reference

| File | Purpose |
|------|---------|
| `Dockerfile` / `Dockerfile.buildio` | Production image |
| `heroku.yml` | Dockerfile build |
| `build_and_push.ps1` | Optional Docker Hub helper |
| `scripts/buildio-set-safe-config.ps1` | PATCH safe config vars via Build.io API |
| `app.json` | Manifest (no DB addon; `DATABASE_URL` required) |
| `.env.buildio` | Commit-safe template |
| `.env.production.local` | Secrets (gitignored) |
| `docker-compose.buildio.yml` | Local full stack only |

---

## Checklist

- [ ] Supabase project + session-pooler `DATABASE_URL`
- [ ] `migrate up` against Supabase
- [ ] `.env.production.local` filled (leave `PUBSUB_*` unset)
- [ ] Config vars on Build.io app `router`
- [ ] Deploy (native Dockerfile or Docker Hub image)
- [ ] `curl https://router.onbld.com/health`
- [ ] Admin UI + API key + test `/v1/messages`

Support: [build.io docs](https://docs.build.io) · [Supabase connecting](https://supabase.com/docs/guides/database/connecting-to-postgres)
