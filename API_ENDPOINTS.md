# API Endpoints — Weave Router

This document describes all available API endpoints on the Weave Router.

## Base URL

```
http://<your-buildio-domain>:8080
```

## Authentication

All endpoints (except `/health` and `/ui/`) require an API key in the Authorization header:

```
Authorization: Bearer <your-router-api-key>
```

The router API key is printed on first startup by the seed command. You can generate one with:

```bash
curl -X POST http://localhost:8080/admin/v1/keys \
  -H "Authorization: Bearer <admin-password-hash>" \
  -d '{"name":"external-client","scopes":["routing"]}'
```

## Endpoints

### Health Check

```
GET /health
```

Returns `200 OK` if the router is healthy.

### Validate API Key

```
POST /validate
Content-Type: application/json

{
  "api_key": "your-api-key"
}
```

Validates an API key and returns the associated installation info.

Response:
```json
{
  "valid": true,
  "installation_id": "inst_abc123",
  "scopes": ["routing"],
  "providers": ["aiand"]
}
```

### Anthropic Messages API

```
POST /v1/messages
Content-Type: application/json
Authorization: Bearer <api-key>

{
  "model": "claude-sonnet-4-5",
  "max_tokens": 1024,
  "messages": [
    {"role": "user", "content": "Hello!"}
  ]
}
```

The router intercepts this and routes to the best model based on cluster scoring.

### OpenAI Chat Completions

```
POST /v1/chat/completions
Content-Type: application/json
Authorization: Bearer <api-key>

{
  "model": "gpt-5",
  "messages": [
    {"role": "user", "content": "Hello!"}
  ]
}
```

### Routing Decision

```
POST /v1/route
Content-Type: application/json
Authorization: Bearer <api-key>

{
  "model": "claude-sonnet-4-5",
  "messages": [
    {"role": "user", "content": "Hello!"}
  ]
}
```

Returns the routing decision without proxying the request:

```json
{
  "decision": {
    "model": "motif-technologies/motif-3",
    "provider": "aiand",
    "confidence": 0.92,
    "tier": "medium"
  }
}
```

### Analytics Export

```
GET /v1/analytics/routing-decisions?start=2026-08-24T00:00:00Z&end=2026-08-25T00:00:00Z
Authorization: Bearer <api-key>
```

Returns routing decisions for the specified time range. Keys are scoped to `ra_`.

### Feedback Link

```
POST /f/<token>
Content-Type: application/json

{
  "rating": 5,
  "feedback": "Great routing!"
}
```

### Dashboard (selfhosted only)

```
GET /ui/dashboard
Authorization: Bearer <admin-password>
```

Returns the dashboard UI.

### Admin API (selfhosted only)

```
GET /admin/v1/keys
Authorization: Bearer <admin-password>
```

Lists all API keys.

```
POST /admin/v1/keys
Authorization: Bearer <admin-password>

{
  "name": "new-key",
  "scopes": ["routing"]
}
```

Creates a new API key.

## Example Usage

### Proxy a Claude Code request

```bash
curl -X POST http://localhost:8080/v1/messages \
  -H "Authorization: Bearer rk_your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-5",
    "max_tokens": 1000,
    "messages": [
      {"role": "user", "content": "Explain quantum computing"}
    ]
  }'
```

### Get the routing decision

```bash
curl -X POST http://localhost:8080/v1/route \
  -H "Authorization: Bearer rk_your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-5",
    "messages": [
      {"role": "user", "content": "Explain quantum computing"}
    ]
  }'
```

Response:

```json
{
  "decision": {
    "model": "motif-technologies/motif-3",
    "provider": "aiand",
    "confidence": 0.88,
    "tier": "medium",
    "cost_usd": 0.0005,
    "reason": "best fit for medium coding task"
  }
}
```n