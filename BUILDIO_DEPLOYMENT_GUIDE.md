# Weave Router - Build.io Deployment Guide

## Overview

This guide walks you through deploying the Weave Router on build.io with all services running in Docker containers.

## Prerequisites

1. **build.io account** - Sign up at [build.io](https://www.build.io)
2. **Docker Hub account** - For pushing your Docker image
3. **Production AI& API key** - From [api.aiand.com](https://api.aiand.com)

## Step 1: Install Build.io CLI

### macOS
```bash
brew install buildio/cli/bld
```

### Linux (Ubuntu/Debian)
```bash
# Install Homebrew
sudo apt install libpcre3-dev build-essential
NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# Add to PATH
echo 'export HOMEBREW_NO_SANDBOX_LINUX=1' >> ~/.bashrc
echo 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv bash)"' >> ~/.bashrc
source ~/.bashrc

# Install CLI
brew install buildio/cli/bld
```

### Windows
Use WSL2 or the build.io web dashboard.

## Step 2: Login to Build.io

```bash
bld login
```

This will open a browser window. Sign in and authorize the CLI.

Verify:
```bash
bld whoami
```

## Step 3: Create the Application

```bash
# Create the router app
bld create app router --stack heroku-24

# Add PostgreSQL addon (this provides DATABASE_URL)
bld addons:create router DATABASE_URL --plan default
```

## Step 4: Configure Environment Variables

Copy these from the prepared `.env.production.local` file:

```bash
# Database Configuration
bld config:set POSTGRES_PASSWORD=fenil2004 --app router
bld config:set POSTGRES_USER=router --app router
bld config:set POSTGRES_DB=router --app router

# Server Configuration
bld config:set PORT=8080 --app router
bld config:set ROUTER_DEPLOYMENT_MODE=selfhosted --app router
bld config:set ROUTER_CLUSTER_VERSION=v0.76 --app router
bld config:set ROUTER_CLUSTER_BUILD_ALL_VERSIONS=false --app router
bld config:set ROUTER_EMBED_ONLY_USER_MESSAGE=false --app router
bld config:set ROUTER_SESSION_PIN_ENABLED=true --app router
bld config:set ROUTER_HARD_PIN_EXPLORE=true --app router
bld config:set ROUTER_PLANNER_ENABLED=true --app router
bld config:set ROUTER_SCORE_TOOL_RESULT_TURNS=true --app router
bld config:set ROUTER_SEMANTIC_CACHE_ENABLED=true --app router
bld config:set ROUTER_SSE_KEEPALIVE_INTERVAL_SECONDS=15 --app router
bld config:set SERVER_REPLICAS=2 --app router

# Provider API Keys (REQUIRED for selfhosted mode)
bld config:set AIAND_API_KEY=sk-7611c41e9cdcd243ce6600107b4621464016425bc71b641fd56df04dd2b462bb --app router
bld config:set AIAND_API_URL=https://api.aiand.com/v1 --app router

# Admin Dashboard
bld config:set ROUTER_ADMIN_PASSWORD=fenil2004 --app router

# BYOK Encryption (generate a real Tink keyset for production)
bld config:set EXTERNAL_KEY_ENCRYPTION_KEY='{"keyData":{"type":"AES256_GCM","cryptoKey":{"keyDataSize":256,"localKeyId":"001"}}}' --app router

# Telemetry / OTel
bld config:set OTEL_SERVICE_NAME=router --app router

# Pub/Sub for Cache Invalidation
bld config:set PUBSUB_PROJECT_ID=router-prod --app router
bld config:set PUBSUB_TOPIC_ROUTER_INVALIDATION=router-installation-invalidate --app router
bld config:set PUBSUB_SUBSCRIPTION_ROUTER_INVALIDATION=router-installation-invalidate --app router

# Deployment Metadata
bld config:set GIT_COMMIT=latest --app router
bld config:set BUILD_TIME=2026-08-24T00:00:00Z --app router
bld config:set SERVER_REPLICAS=2 --app router

# Optional: Logging
bld config:set LOG_LEVEL=info --app router
bld config:set LOG_FORMAT=json --app router
```

## Step 5: Build and Push Docker Image

```bash
# Build the production image
cd d:\router
docker build -t your-dockerhub-username/router:v1 \
  --build-arg ROUTER_SHA=$(git rev-parse HEAD) \
  --build-arg ROUTER_BUILD_TIME=$(date -u +'%Y-%m-%dT%H:%M:%SZ') \
  --build-arg ROUTER_PR=1 \
  -f Dockerfile.buildio .

# Push to Docker Hub
docker push your-dockerhub-username/router:v1
```

## Step 6: Deploy on Build.io

### Option A: Using CLI
```bash
bld deploy router
```

### Option B: Using Dashboard
1. Go to [build.io/dashboard](https://www.build.io/dashboard)
2. Select your `router` app
3. Go to **Deploy** tab
4. Set your Docker image: `your-dockerhub-username/router:v1`
5. Click **Deploy**

## Step 7: Verify Deployment

After deployment completes (5-10 minutes):

```bash
# Check app status
bld info router

# View logs
bld logs router
```

## API Endpoints

Once deployed, your router will be accessible at:
- **HTTPS**: `https://router.build.io`
- **HTTP**: `http://router.build.io` (redirects to HTTPS)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/v1/messages` | POST | Anthropic Messages API |
| `/v1/chat/completions` | POST | OpenAI Chat Completions API |
| `/v1beta/models/:model` | POST | Gemini API |
| `/v1/route` | POST | Get routing decision |
| `/v1/analytics/*` | GET | Analytics export |
| `/f/<token>` | POST | Feedback link |
| `/ui/*` | GET | Admin dashboard |
| `/admin/v1/*` | GET | Admin API |

## Authentication

- **API Key**: `Authorization: Bearer <your-api-key>`
- The router API key is printed on first startup
- You can also set `ROUTER_API_KEY` in environment variables

## Scaling

- Adjust `SERVER_REPLICAS` to scale horizontally
- Each replica shares the same PostgreSQL database
- Pub/Sub ensures cache invalidation across replicas

## Troubleshooting

### Router fails to start
1. Check PostgreSQL connectivity
2. Verify ONNX assets are present
3. Check environment variables

### Database connection issues
```bash
bld run router --command "psql -U router -d router"
```

### View logs
```bash
bld logs router
```

## Cleanup

To remove all deployed resources:
```bash
bld destroy router
```

## Security Notes

1. **BYOK Encryption**: Generate a real Tink keyset for production:
   ```bash
   tinkey create-keyset --key-template AES256_GCM --out-format json
   ```

2. **Don't commit `.env.buildio.local`** to git

3. **Use HTTPS** in production (Nginx is included)

4. **Rotate API keys** regularly

## Support

- [build.io Docs](https://docs.build.io)
- [Weave Router Documentation](./DEPLOYMENT_BUILDIO.md)
