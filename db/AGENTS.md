# db — AGENTS

> **Mirror notice.** Verbatim sync with [CLAUDE.md](CLAUDE.md). **Update both together** — divergence = bug.

Canonical schema + incremental migrations + SQLC query sources. Read [root CLAUDE.md](../CLAUDE.md) first. Adapter that consumes these lives in [`../internal/postgres`](../internal/postgres) — see [its CLAUDE.md](../internal/postgres/CLAUDE.md).

## Layout

- `init/00-create-schema.sql` — **canonical fresh-install schema.** `cmd/initdb` and docker-compose apply this to an empty `router` schema. sqlc parses it for type inference (`db/sqlc.yml`).
- `migrations/` — golang-migrate pairs (`NNNN_<name>.up.sql` + `.down.sql`). `0001_init` is the baseline (same terminal schema as the canonical file). Later files are incremental changes after that baseline.
- `queries/<table>.sql` — SQLC query sources.

## Hybrid workflow

| Path | When | Command |
|---|---|---|
| Fresh install | Empty DB / first compose boot | `make initdb` (or compose `db/init` mount) |
| Migrate-only empty DB | Prefer migrate over initdb | `make migrate-up` (applies `0001_init`, then later) |
| Stamp after initdb/compose | So later ups skip `0001` | `migrate -path db/migrations -database "$(DATABASE_URL)&search_path=router" force 1` |
| Incremental change | Existing baseline DB | `make migrate-up` |

**Schema changes always do both:** edit `init/00-create-schema.sql` (keeps sqlc + fresh installs correct) **and** add a new migration pair via `make migrate-create NAME=...`. Update `queries/` as needed, then `make generate`.

Requires [golang-migrate](https://github.com/golang-migrate/migrate) on the host (`brew install golang-migrate`). Compose has **no** migrate sidecar — incremental applies are host-side (including Supabase).

## Migration conventions

- Always wrap migrations in `BEGIN; ... COMMIT;`.
- Never create migration files manually — use `make migrate-create NAME=<name>` (sequential 4-digit prefix).
- **Down migrations must be precise rollbacks.** No `IF EXISTS` guards. Don't separately drop indexes when dropping tables.
- `0001_init.down.sql` must not `DROP SCHEMA router` — golang-migrate's `schema_migrations` table lives there.
- `organization_id` + `created_by` are opaque external identifiers — **never add foreign keys to tables outside the router's own schema.** Such tables don't exist in this project.
- Soft-delete via `deleted_at TIMESTAMP` on tables that need lifecycle. Hot-path queries filter `WHERE deleted_at IS NULL`.

## Query conventions

- **Always named params** (`@param::varchar`), never numbered (`$1`).
- **Always include type casts** so SQLC inference is unambiguous.
- Query names use consistent prefixes: `Insert*`, `Upsert*`, `Get*`, `Update*`, `Delete*`.
- Every query gets an explanatory comment (SQLC turns it into godoc on the generated function).
- No-rows single-row queries return an error — caller checks `errors.Is(err, sql.ErrNoRows)`.

## Regeneration

After touching anything in `queries/`, run `make generate` and commit the regenerated `../internal/sqlc/` alongside the query change. CI fails if generated code drifts from sources.
