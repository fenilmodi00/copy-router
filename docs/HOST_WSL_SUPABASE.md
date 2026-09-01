# Host WSL against Supabase (no local Postgres)

Run `make setup` and `make dev` on the host against Supabase session pooler.
Skip Compose Postgres. This matches the Build.io single-replica path.

For Build.io deploy steps see [BUILDIO_DEPLOYMENT_GUIDE.md](../BUILDIO_DEPLOYMENT_GUIDE.md).
Env reference: [CONFIGURATION.md](CONFIGURATION.md). Terms: [CONTEXT.md](../CONTEXT.md).

## Skip list

Do **not** run:

| Command | Why |
| --- | --- |
| `make db` | Starts Compose Postgres on port 5433. Host mode uses Supabase instead. |
| `make full-setup` | Boots Compose, then seed, then Claude Code wiring. Wrong for Supabase host mode. |
| `docker compose up` (Postgres) | Local DB is not part of this path. |

Use `make setup` (migrate + seed) and `make dev` only.

## Prerequisites

- Go 1.25+
- [golang-migrate](https://github.com/golang-migrate/migrate) on `PATH`
- [CompileDaemon](https://github.com/githubnemo/CompileDaemon) for `make dev`
- Network reachability to Supabase session pooler host on **port 5432**
- ONNX Runtime + `libtokenizers` on the host (see [ONNX on WSL](#onnx-on-wsl))

## Database URL: session pooler only

1. Supabase Dashboard → **Connect** → **Session** pooler.
2. Copy the URI. Port must be **5432**.
3. Prefer `sslmode=require` in the query string.

**Avoid** the **transaction** pooler on port **6543**. Migrate and the Go `pgx` pool use prepared statements; transaction pooler breaks that path unless you disable prepared statements (not supported here).

Put the URI in `.env.local` as `DATABASE_URL`. Do not commit `.env.local`.

## Minimal `.env.local`

aiand-only minimal set. Do not add Anthropic or OpenAI provider keys for this path.

```bash
# Required
DATABASE_URL=postgresql://USER:PASSWORD@aws-0-REGION.pooler.supabase.com:5432/postgres?sslmode=require
AIAND_API_KEY=sk-your-aiand-api-key

# Typical knobs
ROUTER_CLUSTER_VERSION=v0.76
PORT=8080

# ONNX (WSL paths; adjust after you unpack the libs)
ROUTER_ONNX_ASSETS_DIR=/absolute/path/to/repo/assets
ROUTER_ONNX_LIBRARY_DIR=/absolute/path/to/onnxruntime/lib
CGO_LDFLAGS=-L/absolute/path/to/onnxruntime/lib -L/absolute/path/to/libtokenizers -lonnxruntime
```


Optional override only when you need a non-default aiand base:

```bash
# AIAND_API_URL=https://api.aiand.com/v1
```

## Commands

From the repo root, with `.env.local` loaded by the Makefile (`-include .env.local`):

```bash
# Migrate + seed (prints an rk_ API key). Idempotent when schema is current.
make setup

# Hot-reload server (needs ONNX env above).
make dev
```

When the remote project already has schema `router` but the DB role cannot `CREATE SCHEMA` or cannot write `router.schema_migrations` (common on a shared Supabase project already migrated by another role), `make setup` / `make migrate-up` may fail with `permission denied`. If migrations are already applied (no pending schema drift), run seed only:

```bash
make seed
```

Otherwise ask a role that owns schema `router` to run migrate, then `make seed` on the host.

Check:

```bash
curl -sf http://127.0.0.1:8080/health
curl -sf -H "Authorization: Bearer rk_YOUR_SEEDED_KEY" http://127.0.0.1:8080/validate
curl -sf http://127.0.0.1:8080/ui/ | head
```

## ONNX on WSL

`make dev` builds with `-tags ORT` and needs the same native libs the Dockerfile downloads.

Versions match the root `Dockerfile` (`ONNXRUNTIME_VERSION=1.25.1`, `TOKENIZERS_VERSION=v1.27.0`). For `amd64` WSL:

```bash
ORT_VER=1.25.1
TOK_VER=v1.27.0
mkdir -p "$HOME/opt/onnxruntime" "$HOME/opt/libtokenizers"
curl -fsSL \
  "https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VER}/onnxruntime-linux-x64-${ORT_VER}.tgz" \
  | tar -xz -C "$HOME/opt/onnxruntime" --strip-components=1
curl -fsSL \
  "https://github.com/daulet/tokenizers/releases/download/${TOK_VER}/libtokenizers.linux-x86_64.tar.gz" \
  | tar -xz -C "$HOME/opt/libtokenizers"
```

Embedder assets (Jina INT8) into `./assets` as in [CONFIGURATION.md](CONFIGURATION.md#cluster-routing-artifacts), then set:

```bash
export ROUTER_ONNX_ASSETS_DIR="$(pwd)/assets"
export ROUTER_ONNX_LIBRARY_DIR="$HOME/opt/onnxruntime/lib"
export CGO_LDFLAGS="-L$HOME/opt/onnxruntime/lib -L$HOME/opt/libtokenizers -lonnxruntime"
export LD_LIBRARY_PATH="$HOME/opt/onnxruntime/lib:${LD_LIBRARY_PATH:-}"
```

Without these, the cluster scorer fails closed at boot.

## Compose path (not this guide)

Local Compose Postgres remains documented in [CONTRIBUTING.md](../CONTRIBUTING.md) (`make db` on port 5433). Use that only when you intentionally run a disposable local database. Host WSL + Supabase operators stay on this page.

