# Build.io Deployment — Quick Start

Full guide: **[BUILDIO_DEPLOYMENT_GUIDE.md](./BUILDIO_DEPLOYMENT_GUIDE.md)**

## What's ready

| Artifact | Status |
|----------|--------|
| `Dockerfile` / `Dockerfile.buildio` | Production ONNX/CGO image |
| `heroku.yml` | `SERVER_REPLICAS=1` (Pub/Sub adapter removed) |
| `app.json` | No Schema To Go; requires `DATABASE_URL` |
| Supabase project `router` | `ap-northeast-1` (ref `ssmcjrszhaxbxlyfgthn`) |
| build.io app `router` | `https://router-568f5bb0.onbld.com` |
| `.env.production.local` | Fill secrets (gitignored) |

## Deploy in 4 steps

```powershell
# 1. Secrets
Copy-Item .env.buildio .env.production.local
# Set DATABASE_URL from Supabase Connect → Session (:5432)
# Leave all PUBSUB_* unset (adapter deleted; values ignored)

# 2. Migrate once (direct or session pooler)
# migrate -path db/migrations -database "$env:DATABASE_URL" up

# 3. Push config (API token from Build.io Account settings)
# $env:BUILDIO_API_TOKEN='...'; $env:DATABASE_URL='...'
# .\scripts\buildio-set-safe-config.ps1 -EnvFile .env.production.local
# Then deploy via Dockerfile stack (or Docker Hub image)

# 4. Verify
curl -skSf https://router-568f5bb0.onbld.com/health
```

## Do not

- Attach Schema To Go / Ave To Go
- Rely on `PUBSUB_*` for cache invalidation (adapter deleted; single-replica only)
- Commit `.env.production.local`
