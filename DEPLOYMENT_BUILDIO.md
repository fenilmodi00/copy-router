# build.io Deployment Guide — Weave Router

This document describes how to deploy the Weave Router on build.io with all components (PostgreSQL, Pub/Sub, Router, and optional Nginx proxy) running in Docker containers.

## Architecture

```
                    ┌─────────────────────────────────────┐
                    │            build.io                  │
                    │                                      │
  Internet ───────► │  Nginx (TLS) ─────► Router:8080      │
                    │                         │             │
                    │                         ▼             │
                    │                    PostgreSQL         │
                    │                    (container)        │
                    │                                       │
                    │                    Pub/Sub Emulator   │
                    │                    (cache inv.)       │
                    └─────────────────────────────────────┘
```

## Prerequisites

1. **build.io account** with access to the app "aiand-relay" (team: fenil)
2. **Docker image registry** (Docker Hub, GCP Artifact Registry, etc.)
3. **PostgreSQL password** for the router user
4. **AI& API key** (production key from aiand.com)
5. **BYOK encryption keyset** (for encrypting customer API keys at rest)
6. **OTel collector endpoint** (optional, for telemetry)
7. **SSL certificates** (for production HTTPS via Nginx)

## Deployment Steps

### 1. Build the Docker Image

```bash
# Build the production image
docker build -t your-registry/router:latest \
  --build-arg ROUTER_SHA=$(git rev-parse HEAD) \
  --build-arg ROUTER_BUILD_TIME=$(date -u +'%Y-%m-%dT%H:%M:%SZ') \
  --build-arg TARGETARCH=amd64 \
  -f Dockerfile.buildio .

# Push to registry
docker push your-registry/router:latest
```

### 2. Prepare Environment Variables

Copy the template and fill in production values:

```bash
cp .env.buildio .env.buildio.local
# Edit .env.buildio.local with your production values
```

Required environment variables:
- `POSTGRES_PASSWORD` — database password
- `AIAND_API_KEY` — production AI& API key
- `ROUTER_ADMIN_PASSWORD` — dashboard admin password
- `EXTERNAL_KEY_ENCRYPTION_KEY` — Tink keyset JSON for BYOK encryption

### 3. Deploy on build.io

Use the `docker-compose.buildio.yml` to deploy all services together:

```bash
# On build.io, deploy using the docker-compose file
# All services run in containers on the build.io platform
```

## Configuration Reference

### PostgreSQL
- **Image**: `postgres:15-alpine`
- **Port**: 5432 (internal)
- **Volume**: `postgres_data` (persistent)
- **Credentials**: router/router_production (change in .env.buildio.local)
- **Schema**: router (created by migrations)

### Pub/Sub Emulator
- **Image**: `gcr.io/google.com/cloudsdktool/google-cloud-cli:emulators`
- **Port**: 8085 (internal only)
- **Purpose**: Cross-replica cache invalidation
- **Production alternative**: Replace with GCP Pub/Sub or a Redis-based solution

### Router Server
- **Build context**: Project root
- **Build tags**: `ORT` (ONNX Runtime support)
- **Ports**: 8080 (HTTP)
- **Replicas**: 2 (configurable via `SERVER_REPLICAS`)
- **Resources**: 2GB RAM, 1 CPU limit

### Nginx (Optional)
- **Purpose**: TLS termination, reverse proxy, security headers
- **Ports**: 80 (HTTP redirect), 443 (HTTPS)
- **Volumes**: SSL certificates mounted at `/etc/nginx/ssl`

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `POSTGRES_PASSWORD` | Yes | PostgreSQL password for router user |
| `AIAND_API_KEY` | Yes (selfhosted) | AI& API key |
| `AIAND_API_URL` | No | AI& API base URL (default: https://api.aiand.com/v1) |
| `ROUTER_DEPLOYMENT_MODE` | No | selfhosted or managed (default: selfhosted) |
| `ROUTER_ADMIN_PASSWORD` | Yes | Admin dashboard password |
| `EXTERNAL_KEY_ENCRYPTION_KEY` | Yes (prod) | Tink keyset for BYOK encryption |
| `DATABASE_URL` | No | Override DB connection string |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | No | OTel collector endpoint |
| `PUBSUB_PROJECT_ID` | No | GCP project ID (default: router-prod) |
| `SERVER_REPLICAS` | No | Number of router replicas (default: 1) |

## Cluster Artifact

The router uses cluster artifacts for model routing decisions:
- **Current version**: v0.76 (promoted to `latest`)
- **Location**: `internal/router/cluster/artifacts/v0.76/`
- **Models**: 5 ai&-only models (kimi-k3, motif-3, flash, glm-5.2, deepseek-v4-flash)
- **Embedder**: jina-v2-base-code-int8 (frozen centroids, byte-identical to v0.75)

To update to a newer cluster version:
```bash
# Update the artifacts/latest pointer
echo "v0.77" > internal/router/cluster/artifacts/latest

# Rebuild and redeploy
```

## Scaling Considerations

### Horizontal Scaling
- Set `SERVER_REPLICAS` to the desired number of replicas
- All replicas share the same PostgreSQL database
- Pub/Sub ensures cache invalidation across replicas
- Nginx (or build.io load balancer) distributes traffic

### Resource Limits
Each router replica requires:
- **Memory**: 2 GB (1 GB reservation)
- **CPU**: 1 core (0.5 core reservation)
- **Storage**: ~2 GB for ONNX assets + container overhead

### Database
- Single PostgreSQL instance for development/staging
- For production, consider:
  - Managed PostgreSQL (Cloud SQL, etc.)
  - Connection pooling (PgBouncer)
  - Read replicas for scaling reads

## Security Considerations

1. **BYOK Encryption**: Customer API keys are encrypted at rest using Tink AES-256-GCM
2. **Admin Dashboard**: Protected by `ROUTER_ADMIN_PASSWORD`
3. **Network**: Use Nginx TLS termination for HTTPS
4. **Secrets**: Never commit `.env.buildio.local` to git
5. **Pub/Sub**: The emulator is for development; use GCP Pub/Sub in production

## Monitoring

The router exposes:
- **Health endpoint**: `GET /health`
- **Version endpoint**: `GET /v1/version`
- **Dashboard**: `/ui/*` (selfhosted mode only)
- **OTel traces**: Configurable via `OTEL_EXPORTER_OTLP_ENDPOINT`

## Troubleshooting

### Router fails to start
Check:
1. PostgreSQL connectivity: `DATABASE_URL` is correct
2. ONNX assets are present: `/opt/router/assets/jina-v2-base-code-int8/model.onnx`
3. Environment variables are set correctly

### Cache invalidation not working
- Verify Pub/Sub emulator is running
- Check `PUBSUB_*` environment variables

### Authentication failures
- Verify `AIAND_API_KEY` is set (for selfhosted mode)
- Check `ROUTER_ADMIN_PASSWORD` for dashboard access

## Cleanup

To remove all deployed resources:
```bash
docker-compose -f docker-compose.buildio.yml down -v
```
