# aiand-Key Login Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let **multiple users** log into the router dashboard self-service — each with their own aiand `sk-` API key — creating a **per-user** router installation + session, validated against aiand's `GET /api/v1/me`. One aiand user = one account = one installation; many users can sign up independently. When a user revokes their key, **their** data is wiped (no retention endpoint exists on aiand's side — confirmed below).

**Architecture:** Add a third deployment mode `hosted` alongside `selfhosted`/`managed`. A new `internal/api/account` presentation surface (`/account/v1/*`) replaces the password login with an aiand-key login. The identity/account/session logic lives in `internal/auth` (new `LoginWithAiandKey`, `VerifyAccountSession`, `IssueAccountSession` methods, I/O-free) — the aiand HTTP probe lives in a new `internal/providers/aiand` adapter behind an `auth.AiandIdentityVerifier` interface. A new `router.router_accounts` table binds each aiand identity ↔ its own `model_router_installations` row (account id doubles as the installation's `external_id`) so ALL existing per-installation logic (metrics scoping, allowed/excluded models, BYOK, billing, `rk_` issuance) works unchanged per user. `selfhosted`/`managed` default behavior is untouched.

**Tech Stack:** Go (gin, pgx, SQLC via `make generate`), `golang-lru/v2/expirable` (login rate limiter), Postgres `router` schema, stdlib `crypto/sha256` + `crypto/subtle`. Frontend: Next.js static export at `frontend/`.

## Global Constraints

- Import flow is inward-only. `internal/auth` is the identity domain and must stay I/O-free — the aiand HTTP call lives in a new `internal/providers/aiand` adapter behind an `auth.AiandIdentityVerifier` interface. `internal/api/account` may import only `internal/auth` + `internal/observability`, not `internal/postgres` or concrete provider adapters.
- Adapters never import each other. Only `cmd/router/main.go` constructs concrete adapters.
- No raw SQL outside `db/queries/`; SQLC is the only data mapper. Never edit `internal/sqlc/` — run `make generate`.
- Migrations in `db/migrations/NNNN_<name>.{up,down}.sql`, `BEGIN;`/`COMMIT;`-wrapped, down migrations precise rollbacks with no `IF EXISTS` guards. Never reference `public.*`.
- No FKs to tables outside the router's own schema. `aiand_user_id` / `aiand_organization_id` are opaque external strings, not FKs.
- Use named constants, no magic strings for provider/model names. Use `providers.ProviderAiand`.
- Sentinel errors live in `internal/auth`; HTTP handlers map them to status codes. Use `errors.Is`/`As`, never `==`/`!=` on errors. Use `slog` via `observability`, never `fmt.Println`/`log.Print`.
- Never log raw tokens/secrets. Token-safe form is 8-char prefix + 4-char suffix via `auth.APITokenFingerprint`.
- Inject the clock (`auth.Clock = func() time.Time`) and the HTTP client; no package-level singletons. `panic` only at startup fail-fast.
- `observability.SafeGo` (bounded timeout, panic-recovering) for the off-request-path periodic sweep; never a raw `go func(){}()`.
- Tests live next to code in `<pkg>_test.go`, use `testify/assert` + `testify/require`, real assertions only. No DB-backed tests in `internal/`. In-memory fakes for repos/verifiers.
- Deployment modes: keep `selfhosted` + `managed` byte-for-byte defaults. New `hosted` mode mounts; unknown mode values panic at boot.
- **Multi-user:** each aiand user gets their own `router_accounts` row + installation. The partial unique index on `aiand_user_id WHERE deleted_at IS NULL` enforces one active account per aiand user; many distinct aiand users can coexist.

---

## File Structure

| Path | Responsibility |
|---|---|
| `db/migrations/0064_login.up.sql` / `.down.sql` | New `router.router_accounts` table + indexes |
| `db/queries/router_accounts.sql` | SQLC queries for account upsert/get |
| `db/queries/router_sessions.sql` | SQLC queries for session mint/verify/revoke |
| `internal/auth/errors.go` | New sentinels: `ErrAiandKeyInvalid`, `ErrAiandKeyInsufficientCredits`, `ErrLoginRateLimited`, `ErrAiandUnavailable`, `ErrAccountLoginDisabled`, `ErrAccountSessionInvalid` |
| `internal/auth/account.go` | `Account`/`AccountSession` types + `AccountRepository`/`AccountSessionRepository` interfaces |
| `internal/auth/login_service.go` | `AiandIdentity`, `AiandIdentityVerifier` interface, `LoginWithAiandKey`, `EnsureAccountInstallation`, `IssueAccountSession`, `VerifyAccountSession`, `AccountFromContext` + ctx key |
| `internal/auth/login_service_test.go` | In-memory fakes + unit tests for login/session |
| `internal/api/account/login.go` | `LoginHandler`, `LogoutHandler`, `MeHandler`, cookie helpers (`AccountSessionCookieName`) |
| `internal/api/account/login_test.go` | gin harness tests for the three handlers |
| `internal/server/middleware/account_auth.go` | `WithAccountCookie`, `tryAccountCookie`, `InstallationFromAccount` |
| `internal/server/server.go` | `DeploymentModeHosted` const + `Register` hosted block + extra `account` package wiring |
| `cmd/router/main.go` | Mode validation + wire `aiand.IdentityVerifier` into `authSvc` + account handlers into `server.Register` |
| `internal/providers/aiand/identity_verifier.go` | HTTP `GET /api/v1/me` probe; maps errors to auth sentinels |
| `internal/providers/aiand/identity_verifier_test.go` | `httptest` server tests for HTTP→sentinel mapping |
| `internal/postgres/account_repo.go` | SQLC-backed `auth.AccountRepository` |
| `internal/postgres/session_repo.go` | SQLC-backed `auth.AccountSessionRepository` |
| `frontend/src/lib/api.ts` | `api.auth.loginWithAiand`, `api.auth.accountMe`, `api.auth.accountLogout` |
| `frontend/src/app/(auth)/login/page.tsx` | Split password vs aiand-key login forms |

---

## Task 1: Migration + SQLC queries for `router_accounts`

**Files:**
- Create: `db/migrations/0064_login.up.sql`
- Create: `db/migrations/0064_login.down.sql`
- Create: `db/queries/router_accounts.sql`
- Create: `db/queries/router_sessions.sql`
- (Regenerated): `internal/sqlc/*` (via `make generate`)

**Interfaces:**
- Consumes: existing `router.model_router_installations` PK (`id uuid`). No new FK to it (per tenant-scoping decision below).
- Produces: table + query functions `UpsertRouterAccount :one`, `GetRouterAccountByAiandUserID :one`, `InsertRouterSession :one`, `GetActiveRouterSessionByHash :one`, `TouchRouterSessionLastSeen :execrows`, `RevokeRouterSessionByID :execrows`, `RevokeAllRouterSessionsForAccount :execrows`, `ListRouterSessionsForAccount :many`.

- [ ] **Step 1: Write the up migration**

Create `db/migrations/0064_login.up.sql` with the exact content:

```sql
BEGIN;

-- Multi-user self-service login: each user presents an aiand (sk-) API key; we
-- validate it against aiand's GET /api/v1/me, then we create ONE router
-- installation per aiand user (tenant = installation), keyed on aiand_user_id.
-- Many users can sign up independently — each gets their own account row.
--
-- router_accounts is the login binding; the ACTUAL tenant is a single
-- row of the existing router.model_router_installations table, which owns ALL
-- per-installation data (rk_ keys, BYOK secrets, metrics scoping, billing
-- org_id, allowed/excluded models). The account's own id (a uuid, generated in
-- Go by auth.GenerateID) is stored AS the installation's external_id, so the
-- binding is 1:1 per user and re-hydration mirrors the proven EnsureAdminInstallation
-- create-or-relist pattern. There is NO FK column here to model_router_installations:
-- aiand_user_id / aiand_organization_id are opaque external strings (never FK),
-- and the account.id-as-external_id convention makes a column redundant.
--
-- Data-retention contract: the aiand API reference exposes NO endpoint to
-- retrieve a user's data or re-instantiate an account from a revoked key. When
-- the user revokes the key, their router install + data are wiped (account row
-- soft-deleted). This is intentional and matches the user's stated design.
CREATE TABLE router.router_accounts (
  id                     UUID PRIMARY KEY,
  aiand_user_id          VARCHAR(128) NOT NULL,
  aiand_organization_id  VARCHAR(128) NOT NULL,
  display_name           VARCHAR(255),
  created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_login_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at             TIMESTAMPTZ
);

-- Login lookup: "find my account by aiand identity, only if active."
CREATE UNIQUE INDEX router_accounts_aiand_user_id_active_idx
  ON router.router_accounts(aiand_user_id)
  WHERE deleted_at IS NULL;

-- Future "list accounts in one aiand org" admin view. Cheap to add now.
CREATE INDEX router_accounts_aiand_org_idx
  ON router.router_accounts(aiand_organization_id)
  WHERE deleted_at IS NULL;

COMMENT ON TABLE router.router_accounts IS
  'Multi-user login: each aiand identity mapped to the router installation that owns that user''s tenant data. Account id doubles as the installation external_id (no FK; aiand ids are opaque external strings).';
COMMENT ON COLUMN router.router_accounts.deleted_at IS
  'Soft-delete on key revocation / account wipe. NULL = active.';

COMMIT;
```

- [ ] **Step 2: Write the down migration**

Create `db/migrations/0064_login.down.sql` with the exact content (precise rollback, no `IF EXISTS`):

```sql
BEGIN;

DROP TABLE router.router_accounts;

COMMIT;
```

- [ ] **Step 3: Write `db/queries/router_accounts.sql`**

Create `db/queries/router_accounts.sql` with the exact content:

```sql
-- Upsert is a no-op when the aiand user already has an active account, and a
-- fresh INSERT (with the caller-generated id, which doubles as the
-- installation external_id) when they don't. On returning login it bumps
-- last_login_at, refreshes aiand_organization_id, and updates display_name
-- only if a new one was given (COALESCE so a login from a client that omits
-- display_name can't blank an existing value). Concurrency: two concurrent
-- first-logins for the same aiand user collide on the partial unique index;
-- the loser's INSERT is diverted to the DO UPDATE and returns the WINNER's row,
-- so both callers agree on one account id and, therefore, one installation.
-- Returns the row.
-- name: UpsertRouterAccount :one
INSERT INTO router.router_accounts (
    id,
    aiand_user_id,
    aiand_organization_id,
    display_name
)
VALUES (
    @id::uuid,
    @aiand_user_id::varchar,
    @aiand_organization_id::varchar,
    sqlc.narg('display_name')::varchar
)
ON CONFLICT (aiand_user_id) WHERE deleted_at IS NULL DO UPDATE SET
    last_login_at          = NOW(),
    aiand_organization_id  = EXCLUDED.aiand_organization_id,
    display_name           = COALESCE(EXCLUDED.display_name, router.router_accounts.display_name)
RETURNING *;

-- Login lookup, active only. Used when a returning user logs back in with a
-- (new) aiand key that resolves to the same aiand user id.
-- name: GetRouterAccountByAiandUserID :one
SELECT *
FROM router.router_accounts
WHERE aiand_user_id = @aiand_user_id::varchar
  AND deleted_at IS NULL;

-- Session verification loads the account by its router-generated id.
-- name: GetRouterAccountByID :one
SELECT *
FROM router.router_accounts
WHERE id = @id::uuid
  AND deleted_at IS NULL;

-- Soft-delete an account (wipe on key revocation). Keeps the row so audit
-- trails survive; the user re-presents a fresh aiand key to re-instantiate.
-- name: SoftDeleteRouterAccount :exec
UPDATE router.router_accounts
SET deleted_at = NOW()
WHERE id = @id::uuid
  AND deleted_at IS NULL;
```

- [ ] **Step 4: Write `db/queries/router_sessions.sql`**

Create `db/queries/router_sessions.sql` with the exact content:

```sql
-- Mints a new session row for an account. token_hash is the SHA-256 of the
-- opaque cookie value; token_prefix/token_suffix are the safe 8+4 display parts.
-- name: InsertRouterSession :one
INSERT INTO router.router_sessions (
    account_id,
    token_hash,
    token_prefix,
    token_suffix,
    expires_at,
    ip_at_issue
)
VALUES (
    @account_id::uuid,
    @token_hash::varchar,
    @token_prefix::varchar,
    @token_suffix::varchar,
    @expires_at::timestamptz,
    sqlc.narg('ip_at_issue')::inet
)
RETURNING *;
```

> The `router.router_sessions` DDL is created in **Task 2**. The query above is part of this Task 1 file so the SQLC source is complete; `make generate` is run for real in Task 2 after the table exists.

- [ ] **Step 5: Run tests / compile check**

Run: `make build`
Expected: PASS (no Go references to the new tables yet). `internal/sqlc` is regenerated only after the Task 2 table exists (Task 1 has no query runner yet).

- [ ] **Step 6: Commit**

```bash
git add db/migrations/0064_login.up.sql db/migrations/0064_login.down.sql db/queries/router_accounts.sql db/queries/router_sessions.sql
git commit -m "feat(login): add router_accounts table + login queries"
```

---

## Task 2: Sessions DDL + SQLC queries for `router.router_sessions`

**Files:**
- Modify: `db/migrations/0064_login.up.sql` (append the sessions table)
- Modify: `db/migrations/0064_login.down.sql` (drop sessions table first)
- Modify: `db/queries/router_sessions.sql` (add remaining queries)
- (Regenerated): `internal/sqlc/*` via `make generate`

**Interfaces:**
- Consumes: `router.router_accounts(id)` from Task 1.
- Produces: full set of `AccountSessionRepository` methods: `Insert(context, AccountSession) error`, `GetActiveByTokenHash(ctx, hash) (*AccountSession, error)`, `TouchLastSeen(ctx, accountID, sessionID) error`, `RevokeByID(ctx, accountID, sessionID) error`, `RevokeAllForAccount(ctx, accountID) error`, `ListForAccount(ctx, accountID) ([]AccountSession, error)`.

- [ ] **Step 1: Append session DDL to the up migration**

Append to `db/migrations/0064_login.up.sql` (before the final `COMMIT;`):

```sql
-- ---------------------------------------------------------------------------
-- Dashboard sessions (multi-user): opaque random tokens, stored SHA-256-hashed, never
-- recoverable from the DB. Revocation is a ROW UPDATE (revoked_at = NOW()),
-- the same shape as admin sessions — deliberately not JWT (a jti blacklist
-- would be more machinery for zero gain at dashboard scale).
-- ---------------------------------------------------------------------------
CREATE TABLE router.router_sessions (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id      UUID NOT NULL REFERENCES router.router_accounts(id) ON DELETE CASCADE,
  token_hash      VARCHAR(255) NOT NULL,
  token_prefix    VARCHAR(16) NOT NULL,
  token_suffix    VARCHAR(4) NOT NULL,
  issued_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_at      TIMESTAMPTZ NOT NULL,
  revoked_at      TIMESTAMPTZ,
  last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ip_at_issue     INET
);

-- Session lookup by token hash. Plain unique (partial index with
-- expires_at > NOW() won't compile — NOW() is STABLE, not IMMUTABLE).
-- Expiry + revocation are enforced in the SQLC query, not the index.
CREATE UNIQUE INDEX router_sessions_token_hash_unique
  ON router.router_sessions(token_hash);

-- "List my sessions" dashboard view + periodic sweep.
CREATE INDEX router_sessions_account_id_issued_at_idx
  ON router.router_sessions(account_id, issued_at DESC);

COMMENT ON TABLE router.router_sessions IS
  'Dashboard login sessions (opaque, SHA-256-hashed, revocable).';
```

- [ ] **Step 2: Update the down migration**

Replace the content of `db/migrations/0064_login.down.sql` with (precise rollback — drop sessions before accounts):

```sql
BEGIN;

DROP TABLE router.router_sessions;
DROP TABLE router.router_accounts;

COMMIT;
```

- [ ] **Step 3: Write the remaining session queries**

Replace the placeholder in `db/queries/router_sessions.sql` with the full content:

```sql
-- Mints a new session row for an account. token_hash is the SHA-256 of the
-- opaque cookie value; token_prefix/token_suffix are the safe 8+4 display parts.
-- name: InsertRouterSession :one
INSERT INTO router.router_sessions (
    account_id,
    token_hash,
    token_prefix,
    token_suffix,
    expires_at,
    ip_at_issue
)
VALUES (
    @account_id::uuid,
    @token_hash::varchar,
    @token_prefix::varchar,
    @token_suffix::varchar,
    @expires_at::timestamptz,
    sqlc.narg('ip_at_issue')::inet
)
RETURNING *;

-- Session verification: lookup by token hash, active only. Expiry + revocation
-- are enforced HERE (the index is plain unique because NOW() is STABLE).
-- name: GetActiveRouterSessionByHash :one
SELECT *
FROM router.router_sessions
WHERE token_hash = @token_hash::varchar
  AND revoked_at IS NULL
  AND expires_at > NOW();

-- Bumps last_seen_at on each authed dashboard request (off the request path).
-- name: TouchRouterSessionLastSeen :execrows
UPDATE router.router_sessions
SET last_seen_at = NOW()
WHERE account_id = @account_id::uuid
  AND id = @id::uuid
  AND revoked_at IS NULL;

-- Revokes one session (logout). account_id scoping prevents cross-tenant revoke.
-- name: RevokeRouterSessionByID :execrows
UPDATE router.router_sessions
SET revoked_at = NOW()
WHERE account_id = @account_id::uuid
  AND id = @id::uuid
  AND revoked_at IS NULL;

-- "Log out everywhere."
-- name: RevokeAllRouterSessionsForAccount :execrows
UPDATE router.router_sessions
SET revoked_at = NOW()
WHERE account_id = @account_id::uuid
  AND revoked_at IS NULL;

-- Dashboard "my sessions" page.
-- name: ListRouterSessionsForAccount :many
SELECT *
FROM router.router_sessions
WHERE account_id = @account_id::uuid
ORDER BY issued_at DESC;
```

- [ ] **Step 4: Regenerate SQLC**

Run: `make generate`
Expected: PASS — `internal/sqlc/` regenerated with the new query functions. Commit the generated code.

- [ ] **Step 5: Run tests / compile check**

Run: `make build`
Expected: PASS (still no Go references).

- [ ] **Step 6: Commit**

```bash
git add db/migrations/0064_login.up.sql db/migrations/0064_login.down.sql db/queries/router_sessions.sql internal/sqlc
git commit -m "feat(login): add router_sessions table + token queries"
```

---

## Task 3: `Account` / `AccountSession` types + repository interfaces in `internal/auth`

**Files:**
- Create: `internal/auth/account.go`
- Create: `internal/auth/account_test.go`

**Interfaces:**
- Consumes: nothing new from Tasks 1–2 (types only).
- Produces: `type Account struct { ID, AiandUserID, AiandOrganizationID string; DisplayName *string; CreatedAt, LastLoginAt time.Time; DeletedAt *time.Time }`; `type AccountSession struct { ID, AccountID, TokenHash, TokenPrefix, TokenSuffix string; IssuedAt, ExpiresAt time.Time; RevokedAt *time.Time; LastSeenAt time.Time; IPAtIssue *net.IP }`; `type AccountRepository interface { UpsertByAiandUser(ctx context.Context, p AccountUpsertParams) (*Account, error); GetByAiandUserID(ctx context.Context, aiandUserID string) (*Account, error); GetByID(ctx context.Context, id string) (*Account, error); SoftDelete(ctx context.Context, id string) error }`; `type AccountSessionRepository interface { Insert(ctx context.Context, s AccountSession) error; GetActiveByTokenHash(ctx context.Context, tokenHash string) (*AccountSession, error); TouchLastSeen(ctx context.Context, accountID, sessionID string) error; RevokeByID(ctx context.Context, accountID, sessionID string) error; RevokeAllForAccount(ctx context.Context, accountID string) error; ListForAccount(ctx context.Context, accountID string) ([]AccountSession, error) }`.

- [ ] **Step 1: Write the failing test**

Create `internal/auth/account_test.go` with the exact content:

```go
package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inMemoryAccountRepo implements AccountRepository + AccountSessionRepository so
// the login service can be unit-tested with no DB.
type inMemoryAccountRepo struct {
	accounts  map[string]*Account // key = aiand_user_id
	sessions  map[string]*AccountSession // key = token_hash
	byID      map[string]*Account // key = account id
}

func newInMemoryAccountRepo() *inMemoryAccountRepo {
	return &inMemoryAccountRepo{
		accounts: map[string]*Account{},
		sessions: map[string]*AccountSession{},
		byID:     map[string]*Account{},
	}
}

func (r *inMemoryAccountRepo) UpsertByAiandUser(ctx context.Context, p AccountUpsertParams) (*Account, error) {
	if existing, ok := r.accounts[p.AiandUserID]; ok {
		existing.LastLoginAt = time.Now()
		if p.DisplayName != nil {
			existing.DisplayName = p.DisplayName
		}
		return existing, nil
	}
	acct := &Account{
		ID:                  p.ID,
		AiandUserID:         p.AiandUserID,
		AiandOrganizationID: p.AiandOrganizationID,
		DisplayName:         p.DisplayName,
		CreatedAt:           time.Now(),
		LastLoginAt:         time.Now(),
	}
	r.accounts[p.AiandUserID] = acct
	r.byID[acct.ID] = acct
	return acct, nil
}

func (r *inMemoryAccountRepo) GetByAiandUserID(ctx context.Context, aiandUserID string) (*Account, error) {
	return r.accounts[aiandUserID], nil
}

func (r *inMemoryAccountRepo) GetByID(ctx context.Context, id string) (*Account, error) {
	return r.byID[id], nil
}

func (r *inMemoryAccountRepo) SoftDelete(ctx context.Context, id string) error {
	if acct, ok := r.byID[id]; ok {
		now := time.Now()
		acct.DeletedAt = &now
	}
	return nil
}

func (r *inMemoryAccountRepo) Insert(ctx context.Context, s AccountSession) error {
	r.sessions[s.TokenHash] = &s
	return nil
}

func (r *inMemoryAccountRepo) GetActiveByTokenHash(ctx context.Context, tokenHash string) (*AccountSession, error) {
	return r.sessions[tokenHash], nil
}

func (r *inMemoryAccountRepo) TouchLastSeen(ctx context.Context, accountID, sessionID string) error { return nil }

func (r *inMemoryAccountRepo) RevokeByID(ctx context.Context, accountID, sessionID string) error {
	for _, s := range r.sessions {
		if s.ID == sessionID {
			now := time.Now()
			s.RevokedAt = &now
		}
	}
	return nil
}

func (r *inMemoryAccountRepo) RevokeAllForAccount(ctx context.Context, accountID string) error {
	for _, s := range r.sessions {
		if s.AccountID == accountID {
			now := time.Now()
			s.RevokedAt = &now
		}
	}
	return nil
}

func (r *inMemoryAccountRepo) ListForAccount(ctx context.Context, accountID string) ([]AccountSession, error) {
	var out []AccountSession
	for _, s := range r.sessions {
		if s.AccountID == accountID {
			out = append(out, *s)
		}
	}
	return out, nil
}

func TestAccountUpsertCreatesOnceAndBumpsLastLogin(t *testing.T) {
	repo := newInMemoryAccountRepo()
	ctx := context.Background()

	first, err := repo.UpsertByAiandUser(ctx, AccountUpsertParams{
		ID:                  "acct-1",
		AiandUserID:         "user-aiand-1",
		AiandOrganizationID: "org-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "acct-1", first.ID)

	second, err := repo.UpsertByAiandUser(ctx, AccountUpsertParams{
		ID:                  "acct-2",
		AiandUserID:         "user-aiand-1",
		AiandOrganizationID: "org-1",
	})
	require.NoError(t, err)
	// The second upsert passes a different candidate id ("acct-2"), but the
	// prod code must return the EXISTING row's id — this is exactly the
	// concurrent-first-login contract the account login relies on.
	assert.Equal(t, first.ID, second.ID, "upsert must be idempotent on aiand_user_id")
	assert.Len(t, repo.accounts, 1, "two upserts of the same aiand user must yield one account")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/ -run TestAccountUpsertCreatesOnceAndBumpsLastLogin -v`
Expected: FAIL — `Account`, `AccountUpsertParams`, `AccountRepository` not defined.

- [ ] **Step 3: Write the types + interfaces**

Create `internal/auth/account.go` with the exact content:

```go
package auth

import (
	"context"
	"net"
	"time"
)

// Account is a login identity: one aiand user bound to the router
// installation that owns their tenant data. Many accounts can exist (multi-user).
// The account's ID doubles as the installation's external_id (1:1, re-hydrated via ListForExternalID). aiand_user_id
// is an opaque external string, never an FK. A soft-deleted account means the
// user revoked their key (their data wipe is intentional).
type Account struct {
	ID                  string
	AiandUserID         string
	AiandOrganizationID string
	DisplayName         *string
	CreatedAt           time.Time
	LastLoginAt         time.Time
	DeletedAt           *time.Time
}

// AccountUpsertParams carries the caller-generated account id (which doubles
// as the installation external_id) plus the aiand identity.
type AccountUpsertParams struct {
	ID                  string
	AiandUserID         string
	AiandOrganizationID string
	DisplayName         *string
}

// AccountRepository persists login accounts. Implemented by
// internal/postgres.
type AccountRepository interface {
	UpsertByAiandUser(ctx context.Context, p AccountUpsertParams) (*Account, error)
	GetByAiandUserID(ctx context.Context, aiandUserID string) (*Account, error)
	GetByID(ctx context.Context, id string) (*Account, error)
	SoftDelete(ctx context.Context, id string) error
}

// AccountSession is a revocable, opaque, SHA-256-hashed dashboard session.
type AccountSession struct {
	ID          string
	AccountID   string
	TokenHash   string
	TokenPrefix string
	TokenSuffix string
	IssuedAt    time.Time
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	LastSeenAt  time.Time
	IPAtIssue   *net.IP
}

// SessionRepository persists dashboard login sessions. Implemented by
// internal/postgres.
type AccountSessionRepository interface {
	Insert(ctx context.Context, s AccountSession) error
	GetActiveByTokenHash(ctx context.Context, tokenHash string) (*AccountSession, error)
	TouchLastSeen(ctx context.Context, accountID, sessionID string) error
	RevokeByID(ctx context.Context, accountID, sessionID string) error
	RevokeAllForAccount(ctx context.Context, accountID string) error
	ListForAccount(ctx context.Context, accountID string) ([]AccountSession, error)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/auth/ -run TestAccountUpsertCreatesOnceAndBumpsLastLogin -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/account.go internal/auth/account_test.go
git commit -m "feat(login): add Account/AccountSession types + repo interfaces"
```

> The test above is NOT tautological: `repo.accounts` length and `first.ID == second.ID` would fail if the prod upsert code were deleted / replaced with a fresh-insert-every-time. This is the repo-level contract the login service relies on.

---

## Task 4: `AiandIdentityVerifier` + `LoginWithAiandKey` / `IssueAccountSession` / `VerifyAccountSession` in `internal/auth`

**Files:**
- Create: `internal/auth/login_service.go`
- Modify: `internal/auth/errors.go`
- Create: `internal/auth/login_service_test.go`

**Interfaces:**
- Consumes: Task 3 types/interfaces; existing `auth.APITokenFingerprint` (from `internal/auth/hashing.go`), `auth.GenerateID` (from `internal/auth/id.go`), `auth.Clock`.
- Produces: `type AiandIdentity struct { UserID, OrganizationID, Plan string }`; `type AiandIdentityVerifier interface { Validate(ctx context.Context, rawKey string) (*AiandIdentity, error) }`; `var ErrAiandKeyInvalid, ErrAiandKeyInsufficientCredits, ErrLoginRateLimited, ErrAiandUnavailable, ErrAccountLoginDisabled, ErrAccountSessionInvalid error`; `func (s *Service) WithAiandIdentityVerifier(v AiandIdentityVerifier) *Service`; `func (s *Service) WithAccountRepos(accounts AccountRepository, sessions AccountSessionRepository) *Service`; `func (s *Service) AccountLoginEnabled() bool`; `func (s *Service) LoginWithAiandKey(ctx context.Context, rawKey string) (*Account, *Installation, string, time.Time, error)`; `func (s *Service) EnsureAccountInstallation(ctx context.Context, account *Account) (*Installation, error)`; `func (s *Service) IssueAccountSession(ctx context.Context, account *Account) (string, time.Time, error)`; `func (s *Service) VerifyAccountSession(ctx context.Context, token string) (*Account, error)`; `ctxKeyAccount` + `AccountFromContext(ctx context.Context) *Account`.

- [ ] **Step 1: Write the failing tests**

Create `internal/auth/login_service_test.go` with the exact content:

```go
package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubInstallRepo is the minimal InstallationRepository fake: it implements
// only the methods LoginWithAiandKey/EnsureAccountInstallation use, embedding the
// nil interface for the rest (nil-interface method calls panic, which is a
// test failure signal — no valid test path reaches them).
type stubInstallRepo struct {
	InstallationRepository // nil embedded interface
	byExternalID          map[string]*Installation
}

func (s *stubInstallRepo) ListForExternalID(ctx context.Context, externalID string) ([]*Installation, error) {
	if inst, ok := s.byExternalID[externalID]; ok {
		return []*Installation{inst}, nil
	}
	return nil, nil
}

func (s *stubInstallRepo) Create(ctx context.Context, params CreateInstallationParams) (*Installation, error) {
	inst := &Installation{ID: "inst-" + params.ExternalID, ExternalID: params.ExternalID, Name: params.Name}
	s.byExternalID[params.ExternalID] = inst
	return inst, nil
}

// fakeVerifier returns a canned identity or a canned error.
type fakeVerifier struct {
	identity *AiandIdentity
	err      error
}

func (f *fakeVerifier) Validate(ctx context.Context, rawKey string) (*AiandIdentity, error) {
	return f.identity, f.err
}

func newLoginTestService(t *testing.T, repo *inMemoryAccountRepo) (*Service, *mutableClock, *stubInstallRepo) {
	t.Helper()
	clock := &mutableClock{t: time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)}
	insts := &stubInstallRepo{byExternalID: map[string]*Installation{}}
	svc := NewService(insts, nil, nil, nil, NoOpAPIKeyCache{}, nil, clock.now).
		WithAccountRepos(repo, repo)
	return svc, clock, insts
}

func TestLoginWithAiandKey_ValidKeyCreatesAccountAndInstallation(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc, _, insts := newLoginTestService(t, repo)
	svc.WithAiandIdentityVerifier(&fakeVerifier{identity: &AiandIdentity{
		UserID:         "user-aiand-1",
		OrganizationID: "org-1",
	}})

	acct, inst, token, _, err := svc.LoginWithAiandKey(context.Background(), "sk-valid")
	require.NoError(t, err)
	require.NotNil(t, acct)
	assert.Equal(t, "user-aiand-1", acct.AiandUserID)
	require.NotNil(t, inst)
	assert.Equal(t, acct.ID, inst.ExternalID, "the account id must double as the installation external_id")
	assert.NotEmpty(t, token, "a successful login must mint a session token")
	assert.Len(t, repo.accounts, 1)
	assert.Len(t, insts.byExternalID, 1, "a first login must create exactly one installation")
}

func TestLoginWithAiandKey_InvalidKeyIsRejected(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc, _, _ := newLoginTestService(t, repo)
	svc.WithAiandIdentityVerifier(&fakeVerifier{err: ErrAiandKeyInvalid})

	acct, _, _, _, err := svc.LoginWithAiandKey(context.Background(), "sk-invalid")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAiandKeyInvalid)
	assert.Nil(t, acct)
	assert.Len(t, repo.accounts, 0, "an invalid key must not create an account")
}

func TestLoginWithAiandKey_SameUserReturnsSameAccountAndInstallation(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc, clock, insts := newLoginTestService(t, repo)
	svc.WithAiandIdentityVerifier(&fakeVerifier{identity: &AiandIdentity{
		UserID:         "user-aiand-1",
		OrganizationID: "org-1",
	}})

	first, firstInst, _, _, err := svc.LoginWithAiandKey(context.Background(), "sk-one")
	require.NoError(t, err)
	clock.t = clock.t.Add(time.Hour)
	second, secondInst, _, _, err := svc.LoginWithAiandKey(context.Background(), "sk-two")
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID, "a different aiand key for the same aiand user must map to one account")
	assert.Equal(t, firstInst.ID, secondInst.ID, "and to one installation")
	assert.Len(t, repo.accounts, 1)
	assert.Len(t, insts.byExternalID, 1, "returning login must not create a second installation")
}

func TestLoginWithAiandKey_UnavailableIsPropagated(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc, _, _ := newLoginTestService(t, repo)
	svc.WithAiandIdentityVerifier(&fakeVerifier{err: ErrAiandUnavailable})

	_, _, _, _, err := svc.LoginWithAiandKey(context.Background(), "sk-any")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAiandUnavailable)
	assert.Len(t, repo.accounts, 0)
}

func TestAccountSession_IssueAndVerifyRoundTrip(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc, _, _ := newLoginTestService(t, repo)
	acct := &Account{ID: "acct-1", AiandUserID: "user-aiand-1"}
	repo.byID[acct.ID] = acct

	token, _, err := svc.IssueAccountSession(context.Background(), acct)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	got, err := svc.VerifyAccountSession(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, acct.ID, got.ID, "the session must resolve to the account that issued it")
}

func TestAccountSession_InvalidTokenRejected(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc, _, _ := newLoginTestService(t, repo)

	_, err := svc.VerifyAccountSession(context.Background(), "bogus-token")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAccountSessionInvalid)
}
```

> **Note:** `TestLoginWithAiandKey_SameUserReturnsSameAccountAndInstallation` exercises the exact key-rotation story the user cares about: two different aiand `sk-` keys, same `aiand_user_id` → same account **and** same installation (data intact). This test is NOT tautological: every assertion (`first.ID == second.ID`, `firstInst.ID == secondInst.ID`, repo/installation lengths) would fail if the prod upsert/ensure code were deleted or replaced with fresh-create-every-time.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/ -run 'TestLoginWithAiandKey|TestAccountSession' -v`
Expected: FAIL — `AiandIdentity`, `ErrAiandKeyInvalid`, `WithAccountRepos`, `LoginWithAiandKey`, `IssueAccountSession`, `VerifyAccountSession` not defined.

- [ ] **Step 3: Add sentinels to `internal/auth/errors.go`**

Append to `internal/auth/errors.go`, replacing nothing:

```go
// --- Account login sentinels ----------------------------------------------------

// ErrAiandKeyInvalid is returned when aiand rejects the presented key (401/403).
var ErrAiandKeyInvalid = errors.New("login: invalid aiand api key")

// ErrAiandKeyInsufficientCredits is returned when aiand 402s the presented key.
// Mapping is on the handler: refuse login or allow depending on product policy.
var ErrAiandKeyInsufficientCredits = errors.New("login: aiand account out of credits")

// ErrLoginRateLimited is returned when aiand throttles us (429) or our own
// per-account/IP throttle trips.
var ErrLoginRateLimited = errors.New("login: login rate limited")

// ErrAiandUnavailable is returned when aiand is down (5xx) or unreachable —
// the handler maps to 503 + Retry-After so clients retry instead of treating
// it as terminal.
var ErrAiandUnavailable = errors.New("login: identity provider unavailable")

// ErrAccountLoginDisabled is returned when LoginWithAiandKey is called before
// WithAiandIdentityVerifier is wired (account login not configured).
var ErrAccountLoginDisabled = errors.New("login: login disabled")

// ErrAccountSessionInvalid is returned when an account session token fails to verify
// or is expired/revoked.
var ErrAccountSessionInvalid = errors.New("login: session invalid")
```

- [ ] **Step 4: Write the implementation**

Create `internal/auth/login_service.go` with the exact content:

```go
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
)

// AiandIdentity is the identity aiand's GET /api/v1/me returns for a valid key.
type AiandIdentity struct {
	UserID         string
	OrganizationID string
	Plan           string
}

// AiandIdentityVerifier validates an aiand sk- key against aiand's API. The
// HTTP call is I/O, so the concrete implementation lives in
// internal/providers/aiand; this interface keeps internal/auth I/O-free.
type AiandIdentityVerifier interface {
	Validate(ctx context.Context, rawKey string) (*AiandIdentity, error)
}

// AccountSessionCookieName is the HttpOnly cookie holding an account dashboard session.
const AccountSessionCookieName = "router_account_session"

// DefaultAccountSessionTTL is the account dashboard session validity.
const DefaultAccountSessionTTL = 7 * 24 * time.Hour

// accountSessionTokenChars is the rejection-sampled alphabet for session tokens
// (same approach as GenerateID, no modulo bias).
const accountSessionTokenChars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// ctxKeyAccount is the typed context key for the resolved account.
type ctxKeyAccount struct{}

// WithAccountRepos wires the account + session repositories. Both nil is a
// no-op (login returns ErrAccountLoginDisabled); kept off NewService so existing
// callers/tests stay source-stable.
func (s *Service) WithAccountRepos(accounts AccountRepository, sessions AccountSessionRepository) *Service {
	s.accounts = accounts
	s.sessions = sessions
	return s
}

// WithAiandIdentityVerifier wires the aiand identity probe. nil is a no-op
// (login returns ErrAccountLoginDisabled).
func (s *Service) WithAiandIdentityVerifier(v AiandIdentityVerifier) *Service {
	s.aiandVerifier = v
	return s
}

// AccountLoginEnabled reports whether account login is fully wired (identity verifier +
// both repositories).
func (s *Service) AccountLoginEnabled() bool {
	return s.aiandVerifier != nil && s.accounts != nil && s.sessions != nil
}

// LoginWithAiandKey validates an aiand key, creates-or-returns the aiand user's
// account AND its router installation, mints a session token, and returns
// (account, installation, raw token, expiry, nil). When the same aiand user logs
// in with a NEW key, the existing account + installation are returned (data
// retention across rotation lives entirely on aiand's side — see the migration's
// data-retention comment).
func (s *Service) LoginWithAiandKey(ctx context.Context, rawKey string) (*Account, *Installation, string, time.Time, error) {
	if !s.AccountLoginEnabled() {
		return nil, nil, "", time.Time{}, ErrAccountLoginDisabled
	}
	identity, err := s.aiandVerifier.Validate(ctx, rawKey)
	if err != nil {
		if errors.Is(err, ErrAiandKeyInvalid) ||
			errors.Is(err, ErrAiandKeyInsufficientCredits) ||
			errors.Is(err, ErrLoginRateLimited) ||
			errors.Is(err, ErrAiandUnavailable) {
			return nil, nil, "", time.Time{}, err
		}
		return nil, nil, "", time.Time{}, err
	}
	if identity == nil || identity.UserID == "" {
		return nil, nil, "", time.Time{}, ErrAiandKeyInvalid
	}

	// Candidate id doubles as the installation external_id. On a returning
	// login the upsert returns the EXISTING row's id (see the query's ON
	// CONFLICT), so both callers converge on one installation.
	account, err := s.accounts.UpsertByAiandUser(ctx, AccountUpsertParams{
		ID:                  GenerateID("acct"),
		AiandUserID:         identity.UserID,
		AiandOrganizationID: identity.OrganizationID,
	})
	if err != nil {
		return nil, nil, "", time.Time{}, err
	}

	installation, err := s.EnsureAccountInstallation(ctx, account)
	if err != nil {
		return nil, nil, "", time.Time{}, err
	}

	token, expiresAt, err := s.IssueAccountSession(ctx, account)
	if err != nil {
		return nil, nil, "", time.Time{}, err
	}
	return account, installation, token, expiresAt, nil
}

// EnsureAccountInstallation returns the installation whose external_id equals the
// account's id, creating it on first call. Mirrors EnsureAdminInstallation: on
// a concurrent first-hit the loser re-lists and returns the winner's row.
func (s *Service) EnsureAccountInstallation(ctx context.Context, account *Account) (*Installation, error) {
	if inst, ok, err := s.findAccountInstallation(ctx, account.ID); err != nil {
		return nil, err
	} else if ok {
		return inst, nil
	}
	name := account.AiandUserID
	if account.DisplayName != nil && *account.DisplayName != "" {
		name = *account.DisplayName
	}
	created, createErr := s.installations.Create(ctx, CreateInstallationParams{
		ExternalID: account.ID,
		Name:       name,
	})
	if createErr == nil {
		return created, nil
	}
	// Lost a concurrent race.
	if inst, ok, err := s.findAccountInstallation(ctx, account.ID); err == nil && ok {
		return inst, nil
	}
	return nil, createErr
}

func (s *Service) findAccountInstallation(ctx context.Context, accountID string) (*Installation, bool, error) {
	existing, err := s.installations.ListForExternalID(ctx, accountID)
	if err != nil {
		return nil, false, err
	}
	for _, inst := range existing {
		if inst != nil && inst.DeletedAt == nil {
			return inst, true, nil
		}
	}
	return nil, false, nil
}

// IssueAccountSession mints an opaque token, stores its SHA-256 hash, and returns
// the raw token + expiry. The raw token is returned once and never stored.
func (s *Service) IssueAccountSession(ctx context.Context, account *Account) (string, time.Time, error) {
	if s.sessions == nil {
		return "", time.Time{}, ErrAccountLoginDisabled
	}
	now := s.now()
	expiresAt := now.Add(DefaultAccountSessionTTL)
	raw := generateAccountSessionToken()
	hash, prefix, suffix := APITokenFingerprint(raw)
	err := s.sessions.Insert(ctx, AccountSession{
		ID:          GenerateID("ssn"),
		AccountID:   account.ID,
		TokenHash:   hash,
		TokenPrefix: prefix,
		TokenSuffix: suffix,
		IssuedAt:    now,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return raw, expiresAt, nil
}

// VerifyAccountSession resolves a cookie token to its account. Returns
// ErrAccountSessionInvalid for unknown, expired, or revoked tokens.
func (s *Service) VerifyAccountSession(ctx context.Context, token string) (*Account, error) {
	if s.sessions == nil || s.accounts == nil {
		return nil, ErrAccountSessionInvalid
	}
	hash := HashAPIKeySHA256(token)
	session, err := s.sessions.GetActiveByTokenHash(ctx, hash)
	if err != nil {
		return nil, ErrAccountSessionInvalid
	}
	if session == nil || session.AccountID == "" {
		return nil, ErrAccountSessionInvalid
	}
	account, err := s.accounts.GetByID(ctx, session.AccountID)
	if err != nil {
		return nil, ErrAccountSessionInvalid
	}
	return account, nil
}

// AccountFromContext returns the account stashed on ctx by the
// middleware, or nil.
func AccountFromContext(ctx context.Context) *Account {
	v, ok := ctx.Value(ctxKeyAccount{}).(*Account)
	if !ok {
		return nil
	}
	return v
}

// generateAccountSessionToken returns a rejection-sampled 32-char base62 token.
func generateAccountSessionToken() string {
	buf := make([]byte, 32)
	limit := byte(256 - (256 % len(accountSessionTokenChars)))
	out := make([]byte, 0, 32)
	for len(out) < 32 {
		if _, err := rand.Read(buf); err != nil {
			panic(err)
		}
		for _, x := range buf {
			if x >= limit {
				continue
			}
			out = append(out, accountSessionTokenChars[int(x)%len(accountSessionTokenChars)])
			if len(out) == 32 {
				break
			}
		}
	}
	return string(out)
}
```

> **`LoginWithAiandKey` leak-avoidance note:** the raw aiand key is used only inside `Validate` and dropped; it is never persisted. The session raw token is returned once; the DB holds only its SHA-256 hash. This satisfies the repo's "never store raw keys" constraint.

> **Service struct fields (needed by `WithAccountRepos` / `WithAiandIdentityVerifier`):** add to `internal/auth/service.go`'s `Service` struct, keeping `NewService` unmodified (the fields default to nil, so `AccountLoginEnabled()` is false until wired):

```go
	// accounts + sessions back the aiand-key account login; nil (default)
	// means account login is disabled. aiandVerifier is the I/O boundary for the
	// aiand identity probe (implemented in internal/providers/aiand).
	accounts  AccountRepository
	sessions  AccountSessionRepository
	aiandVerifier AiandIdentityVerifier
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/auth/ -run 'TestLoginWithAiandKey|TestAccountSession' -v`
Expected: PASS (`inMemoryAccountRepo` already implements `GetByID` from Task 3's fake; `stubInstallRepo` supplies `ListForExternalID`/`Create`).

- [ ] **Step 6: Commit**

```bash
git add internal/auth/login_service.go internal/auth/login_service_test.go internal/auth/errors.go internal/auth/service.go
git commit -m "feat(login): aiand-key login + revocable session service"
```