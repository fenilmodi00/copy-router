# Weave Router - Build.io Deployment

## Summary

All deployment files are prepared. The Weave Router is ready to be deployed on build.io with all services containerized.

## Files Created/Modified

| File | Purpose |
|------|---------|
| `.env.production.local` | Production environment config with AI& key |
| `app.json` | Build.io app manifest |
| `docker-compose.buildio.yml` | All services (Postgres, Pub/Sub, Router, Nginx) |
| `Dockerfile.buildio` | Production Docker image |
| `DEPLOY.sh` | Automation script |
| `BUILDIO_DEPLOYMENT_GUIDE.md` | Complete deployment guide |
| `setup_wsl.sh` | WSL setup + Docker build |

## Current State

- **Router**: Ready with v0.76 cluster artifacts
- **AI& API key**: Configured: `sk-7611c41e9cdcd243ce6600107b4621464016425bc71b641fd56df04dd2b462bb`
- **Admin password**: `fenil2004`
- **Database**: PostgreSQL configured
- **Pub/Sub**: Configured for cache invalidation
- **Replicas**: 2

## Next Steps (Choose One)

### Option 1: Build.io Dashboard (Easiest)
1. Visit [build.io/dashboard](https://www.build.io/dashboard)
2. Create app: `router` (stack: heroku-24)
3. Add addon: `ave-to-go` (plan: default)
4. Set environment variables from `.env.production.local`
5. Deploy Docker image: `your-username/router:v1`

### Option 2: bld CLI (Requires Crystal)
```bash
# Install Crystal
curl -fsSL https://crystal-lang.org/install.sh | bash

# Install build.io CLI
git clone https://github.com/buildio/cli.git
cd cli && shards build

# Deploy
bld login
bld create app router --stack heroku-24
bld addons:create router DATABASE_URL --plan default
bld deploy router
```

### Option 3: WSL (Windows)
```bash
# Enable WSL
wsl --install -d Ubuntu-24.04

# Run setup script
cd /mnt/d/router
./setup_wsl.sh
```

## API Endpoints

Once deployed at `https://router.build.io`:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/v1/messages` | POST | Anthropic Messages API |
| `/v1/chat/completions` | POST | OpenAI Chat Completions API |
| `/v1/route` | POST | Get routing decision |
| `/ui/*` | GET | Admin dashboard |

## Important Notes

1. **BYOK Encryption**: Replace `EXTERNAL_KEY_ENCRYPTION_KEY` with a real Tink keyset before production
2. **Don't commit** `.env.buildio.local` to git
3. **Use HTTPS** in production (Nginx included)
4. **Rotate API keys** regularly

## Support

- [Build.io Docs](https://docs.build.io)
- [Weave Router Docs](./DEPLOYMENT_BUILDIO.md)
