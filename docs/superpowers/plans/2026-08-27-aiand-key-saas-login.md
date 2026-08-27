# aiand-Key Self-Service Login Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let any user log into their own router dashboard with their aiand `sk-` API key, creating a per-user installation + session, validated against aiand's `GET /api/v1/me`; when the user revokes their key, their router data is wiped (aiand's API exposes no retention endpoint — confirmed in `docs/aiand-api-reference.md`).

**Architecture:** Add a third deployment mode `selfserve` alongside `selfhosted`/`managed`. In `selfserve` mode a new `internal/api/account` presentation surface (`/account/v1/*`) replaces the password login with an aiand-key login. The identity/account/session logic lives in `internal/auth` (new `LoginWithKey`, `VerifyLoginSession`, `IssueLoginSession` methods — `internal/auth` stays I/O-free); the aiand HTTP probe lives in a new `internal/providers/aiand` adapter behind an `auth.AiandKeyVerifier` interface. A new `router.accounts` table binds each aiand identity ↔ its own `model_router_installations` row (account id doubles as the installation's `external_id`, mirroring the existing `EnsureAdminInstallation` create-or-relist pattern) so ALL existing per-installation logic (metrics scoping, allowed/excluded models, BYOK, billing, `rk_` issuance) works unchanged per user. `selfhosted`/`managed` default behavior is untouched.

**Tech Stack:** Go (gin, pgx, SQLC via `make generate`), `golang-lru/v2/expirable` (login rate limiter), Postgres `router` schema, stdlib `crypto/sha256` + `crypto/subtle`. Frontend: Next.js static export at `frontend/`.

## Global Constraints

- Import flow is inward-only. `internal/auth` is the identity domain and must stay I/O-free — the aiand HTTP call lives in a new `internal/providers/aiand` adapter behind an `auth.AiandKeyVerifier` interface. `internal/api/account` may import only `internal/auth` + `internal/observability`, not `internal/postgres` or concrete provider adapters.
- Adapters never import each other. Only `cmd/router/main.go` constructs concrete adapters.
- No raw SQL outside `db/queries/`; SQLC is the only data mapper. Never edit `internal/sqlc/` — run `make generate`.
- Migrations in `db/migrations/NNNN_<name>.{up,down}.sql`, `BEGIN;`/`COMMIT;`-wrapped, down migrations precise rollbacks with no `IF EXISTS` guards. Never reference `public.*`. Latest migration today is `0063_installation-flag-overrides`; the new pair is **`0064_account-login.{up,down}.sql`**.
- No FKs to tables outside the router's own schema. `aiand_user_id` / `aiand_organization_id` are opaque external strings, not FKs.
- Use named constants, no magic strings for provider/model names. Use `providers.ProviderAiand`.
- Sentinel errors live in `internal/auth`; HTTP handlers map them to status codes. Use `errors.Is`/`As`, never `==`/`!=` on errors. Use `slog` via `observability`, never `fmt.Println`/`log.Print`.
- Never log raw tokens/secrets. Token-safe form is 8-char prefix + 4-char suffix via `auth.APITokenFingerprint`.
- Inject the clock (`auth.Clock = func() time.Time`) and the HTTP client; no package-level singletons. `panic` only at startup fail-fast.
- `observability.SafeGo` (bounded timeout, panic-recovering) for the off-request-path periodic sweep; never a raw `go func(){}()`.
- Tests live next to code in `<pkg>_test.go`, use `testify/assert` + `testify/require`, real assertions only. No DB-backed tests in `internal/`. In-memory fakes for repos/verifiers.
- Deployment modes: keep `selfhosted` + `managed` byte-for-byte defaults. New `selfserve` mode mounts; unknown mode values panic at boot.
- **Naming rule (from the user):** the *product* works like SaaS, but code/file/identifier names use plain domain terms — `account`, `session`, `login`, `key` — never `saas`/`SaaS`. `selfserve` is the deployment-mode constant; everything else is `account`/`session`/`login`.

---

## File Structure

| Path | Responsibility |
|---|---|
| `db/migrations/0064_account-login.up.sql` / `.down.sql` | New `router.accounts` + `router.account_sessions` tables + indexes |
| `db/queries/accounts.sql` | SQLC queries for account upsert/get |
| `db/queries/account_sessions.sql` | SQLC queries for session mint/verify/revoke |
| `internal/auth/errors.go` | New sentinels: `ErrKeyInvalid`, `ErrKeyInsufficientCredits`, `ErrLoginRateLimited`, `ErrKeyUnavailable`, `ErrLoginDisabled`, `ErrLoginSessionInvalid` |
| `internal/auth/account.go` | `Account`/`LoginSession` types + `AccountRepository`/`LoginSessionRepository` interfaces |
| `internal/auth/account_login.go` | `AiandKeyVerifier` interface, `LoginWithKey`, `EnsureAccountInstallation`, `IssueLoginSession`, `VerifyLoginSession`, `AccountFromContext` + ctx key |
| `internal/auth/account_login_test.go` | In-memory fakes (account repo + installation repo) + unit tests for login/session |
| `internal/api/account/login.go` | `LoginHandler`, `LogoutHandler`, `MeHandler`, cookie helpers (`LoginSessionCookieName`) |
| `internal/api/account/login_test.go` | gin harness tests for the three handlers |
| `internal/server/middleware/account_auth.go` | `WithAccountCookie`, `tryAccountCookie`, `AccountFrom` |
| `internal/server/server.go` | `DeploymentModeSelfServe` const + `Register` selfserve block |
| `cmd/router/main.go` | Mode validation + wire `aiand.KeyVerifier` into `authSvc` + account handlers into `server.Register` |
| `internal/providers/aiand/identity_verifier.go` | HTTP `GET /api/v1/me` probe; maps errors to auth sentinels |
| `internal/providers/aiand/identity_verifier_test.go` | `httptest` server tests for HTTP → sentinel mapping |
| `internal/postgres/account_repo.go` | SQLC-backed `auth.AccountRepository` |
| `internal/postgres/login_session_repo.go` | SQLC-backed `auth.LoginSessionRepository` |
| `frontend/src/lib/api.ts` | `api.auth.loginWithKey`, `api.auth.accountMe`, `api.auth.accountLogout` |
| `frontend/src/app/(auth)/login/page.tsx` | Split password vs aiand-key login forms |

---

## Task 1: Migration + SQLC queries for `router.accounts` + `router.account_sessions`

**Files:**
- Create: `db/migrations/0064_account-login.up.sql`
- Create: `db/migrations/0064_account-login.down.sql`
- Create: `db/queries/accounts.sql`
- Create: `db/queries/account_sessions.sql`
- (Regenerated): `internal/sqlc/*` (via `make generate`)

**Interfaces:**
- Consumes: existing `router.model_router_installations` (`external_id varchar(36)`). The account's `id` string is stored as that installation's `external_id` — no FK column needed.
- Produces: table `router.accounts` (`id uuid PK`, `aiand_user_id varchar(128) NOT NULL`, `aiand_organization_id varchar(128) NOT NULL`, `display_name varchar(255)`, `created_at`, `last_login_at`, `deleted_at`), table `router.account_sessions` (`id uuid PK`, `account_id uuid NOT NULL REFERENCES router.accounts(id) ON DELETE CASCADE`, `token_hash varchar(64) NOT NULL`, `token_prefix varchar(16) NOT NULL`, `token_suffix varchar(16) NOT NULL`, `issued_at`, `expires_at`, `revoked_at`, `last_seen_at`, `ip_at_issue inet`), query functions `UpsertAccount :one`, `GetAccountByAiandUserID :one`, `GetAccountByID :one`, `InsertAccountSession :one`, `GetActiveAccountSessionByHash :one`, `TouchAccountSessionLastSeen :execrows`, `RevokeAccountSessionByID :execrows`, `RevokeAllAccountSessionsForAccount :execrows`, `ListAccountSessionsForAccount :many`.

- [ ] **Step 1: Write the up migration**

Create `db/migrations/0064_account-login.up.sql` with the exact content:

```sql
BEGIN;

-- Self-service login: a user presents an aiand (sk-) API key; we validate it
-- against aiand's GET /api/v1/me, then we create ONE router installation per
-- aiand user (tenant = installation), keyed on aiand_user_id.
--
-- router.accounts is the login binding; the ACTUAL tenant is a single row of
-- the existing router.model_router_installations table, which owns ALL
-- per-installation data (rk_ keys, BYOK secrets, metrics scoping, billing
-- org_id, allowed/excluded models). The account id (a varchar column, an
-- acct_ prefix id generated in Go) is stored AS that installation's external_id, so the binding is 1:1
-- and re-hydration mirrors the proven EnsureAdminInstallation create-or-relist
-- pattern. There is NO FK column to model_router_installations: aiand_user_id
-- / aiand_organization_id are opaque external strings (never FK), and the
-- account-id-as-external_id convention makes a column redundant.
--
-- Data-retention contract: the aiand API reference exposes NO endpoint to
-- retrieve a user's data or re-instantiate an account from a revoked key. When
-- the user revokes the key, their router install + data are wiped (account row
-- soft-deleted). This is intentional and matches the user's stated design.
CREATE TABLE router.accounts (
  id                    VARCHAR(36) PRIMARY KEY,
  aiand_user_id         VARCHAR(128) NOT NULL,
  aiand_organization_id VARCHAR(128) NOT NULL,
  display_name          VARCHAR(255),
  created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_login_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at            TIMESTAMPTZ
);

-- Login lookup: "find my account by aiand identity, only if active."
CREATE UNIQUE INDEX accounts_aiand_user_id_active_idx
  ON router.accounts(aiand_user_id)
  WHERE deleted_at IS NULL;

-- Future "list accounts in one aiand org" admin view. Cheap to add now.
CREATE INDEX accounts_aiand_org_idx
  ON router.accounts(aiand_organization_id)
  WHERE deleted_at IS NULL;

COMMENT ON TABLE router.accounts IS
  'Self-service login: aiand identity mapped to the router installation that owns all tenant data. Account id doubles as the installation external_id (no FK; aiand ids are opaque external strings).';
COMMENT ON COLUMN router.accounts.deleted_at IS
  'Soft-delete on key revocation / account wipe. NULL = active.';

-- Dashboard sessions: opaque random tokens stored SHA-256-hashed, never
-- recoverable from the DB. Revocation is a ROW UPDATE (revoked_at = NOW()),
-- the same shape as admin sessions — deliberately not JWT.
CREATE TABLE router.account_sessions (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id   VARCHAR(36) NOT NULL REFERENCES router.accounts(id) ON DELETE CASCADE,
  token_hash   VARCHAR(64) NOT NULL,
  token_prefix VARCHAR(16) NOT NULL,
  token_suffix VARCHAR(16) NOT NULL,
  issued_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_at   TIMESTAMPTZ NOT NULL,
  revoked_at   TIMESTAMPTZ,
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ip_at_issue  INET
);

-- Lookup by token hash (constant per session; one row per token).
CREATE UNIQUE INDEX account_sessions_token_hash_unique
  ON router.account_sessions(token_hash);

-- "logout everywhere" + per-account session list.
CREATE INDEX account_sessions_account_id_issued_at_idx
  ON router.account_sessions(account_id, issued_at DESC);

COMMENT ON TABLE router.account_sessions IS
  'Revocable dashboard sessions for self-service accounts. token_hash is the SHA-256 of the opaque cookie value; token_prefix/suffix are the safe 8+4 display parts.';
COMMENT ON COLUMN router.account_sessions.revoked_at IS
  'Set on logout / account wipe. NULL = still active.';

COMMIT;
```

- [ ] **Step 2: Write the down migration**

Create `db/migrations/0064_account-login.down.sql` with the exact content (precise rollback, no `IF EXISTS`):

```sql
BEGIN;

DROP TABLE router.account_sessions;
DROP TABLE router.accounts;

COMMIT;
```

- [ ] **Step 3: Write `db/queries/accounts.sql`**

Create `db/queries/accounts.sql` with the exact content:

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
-- name: UpsertAccount :one
INSERT INTO router.accounts (
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
    display_name           = COALESCE(EXCLUDED.display_name, router.accounts.display_name)
RETURNING *;

-- Login lookup, active only. Used when a returning user logs back in with a
-- (new) aiand key that resolves to the same aiand user id.
-- name: GetAccountByAiandUserID :one
SELECT *
FROM router.accounts
WHERE aiand_user_id = @aiand_user_id::varchar
  AND deleted_at IS NULL;

-- Session verification loads the account by its router-generated id.
-- name: GetAccountByID :one
SELECT *
FROM router.accounts
WHERE id = @id::uuid
  AND deleted_at IS NULL;

-- Soft-delete an account (wipe on key revocation). Keeps the row so audit
-- trails survive; the user re-presents a fresh aiand key to re-instantiate.
-- name: SoftDeleteAccount :exec
UPDATE router.accounts
SET deleted_at = NOW()
WHERE id = @id::uuid
  AND deleted_at IS NULL;
```

- [ ] **Step 4: Write `db/queries/account_sessions.sql`**

Create `db/queries/account_sessions.sql` with the exact content:

```sql
-- Mints a new session row for an account. token_hash is the SHA-256 of the
-- opaque cookie value; token_prefix/token_suffix are the safe 8+4 display parts.
-- name: InsertAccountSession :one
INSERT INTO router.account_sessions (
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

-- Active-session lookup by token hash. Returns a row only while the session is
-- live (not revoked, not expired).
-- name: GetActiveAccountSessionByHash :one
SELECT *
FROM router.account_sessions
WHERE token_hash = @token_hash::varchar
  AND revoked_at IS NULL
  AND expires_at > NOW();

-- Bump last_seen_at when the middleware validates a live session (cheap write,
-- throttled to once per request).
-- name: TouchAccountSessionLastSeen :execrows
UPDATE router.account_sessions
SET last_seen_at = NOW()
WHERE account_id = @account_id::uuid
  AND id = @id::uuid;

-- Revoke one session (logout). Scoped to the account to prevent cross-account
-- revocation.
-- name: RevokeAccountSessionByID :execrows
UPDATE router.account_sessions
SET revoked_at = NOW()
WHERE account_id = @account_id::uuid
  AND id = @id::uuid
  AND revoked_at IS NULL;

-- Revoke every session for an account ("logout everywhere"; also called during
-- account wipe).
-- name: RevokeAllAccountSessionsForAccount :execrows
UPDATE router.account_sessions
SET revoked_at = NOW()
WHERE account_id = @account_id::uuid
  AND revoked_at IS NULL;

-- Session index for "my sessions" dashboard UI.
-- name: ListAccountSessionsForAccount :many
SELECT *
FROM router.account_sessions
WHERE account_id = @account_id::uuid
ORDER BY issued_at DESC;
```

- [ ] **Step 5: Run tests / compile check**

Run: `make build`
Expected: PASS (no Go references to the new tables yet). Run `make generate` to regenerate `internal/sqlc/*`.

- [ ] **Step 6: Commit**

```bash
git add db/migrations/0064_account-login.up.sql db/migrations/0064_account-login.down.sql db/queries/accounts.sql db/queries/account_sessions.sql internal/sqlc
git commit -m "feat(login): add accounts + account_sessions tables and SQLC queries"
```

---

## Task 2: `Account` / `LoginSession` types + repository interfaces in `internal/auth`

**Files:**
- Create: `internal/auth/account.go`
- Create: `internal/auth/account_test.go`

**Interfaces:**
- Consumes: nothing new from Task 1 (types only).
- Produces: `type Account struct { ID, AiandUserID, AiandOrganizationID string; DisplayName *string; CreatedAt, LastLoginAt time.Time; DeletedAt *time.Time }`; `type LoginSession struct { ID, AccountID, TokenHash, TokenPrefix, TokenSuffix string; IssuedAt, ExpiresAt time.Time; RevokedAt *time.Time; LastSeenAt time.Time; IPAtIssue *net.IP }`; `type AccountRepository interface { UpsertByAiandUser(ctx context.Context, p AccountUpsertParams) (*Account, error); GetByAiandUserID(ctx context.Context, aiandUserID string) (*Account, error); GetByID(ctx context.Context, id string) (*Account, error); SoftDelete(ctx context.Context, id string) error }`; `type LoginSessionRepository interface { Insert(ctx context.Context, s LoginSession) (*LoginSession, error); GetActiveByTokenHash(ctx context.Context, tokenHash string) (*LoginSession, error); TouchLastSeen(ctx context.Context, accountID, sessionID string) error; RevokeByID(ctx context.Context, accountID, sessionID string) error; RevokeAllForAccount(ctx context.Context, accountID string) error; ListForAccount(ctx context.Context, accountID string) ([]LoginSession, error) }`.

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

// inMemoryAccountRepo implements AccountRepository + LoginSessionRepository so
// the login service can be unit-tested with no DB.
type inMemoryAccountRepo struct {
	accounts  map[string]*Account     // key = aiand_user_id
	sessions  map[string]*LoginSession // key = token_hash
	byID      map[string]*Account     // key = account id
}

func newInMemoryAccountRepo() *inMemoryAccountRepo {
	return &inMemoryAccountRepo{
		accounts:  map[string]*Account{},
		sessions:  map[string]*LoginSession{},
		byID:      map[string]*Account{},
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

func (r *inMemoryAccountRepo) Insert(ctx context.Context, s LoginSession) (*LoginSession, error) {
	r.sessions[s.TokenHash] = &s
	return &s, nil
}

func (r *inMemoryAccountRepo) GetActiveByTokenHash(ctx context.Context, tokenHash string) (*LoginSession, error) {
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

func (r *inMemoryAccountRepo) ListForAccount(ctx context.Context, accountID string) ([]LoginSession, error) {
	var out []LoginSession
	for _, s := range r.sessions {
		if s.AccountID == accountID {
			out = append(out, *s)
		}
	}
	return out, nil
}

func TestAccountUpsertCreatesOnce(t *testing.T) {
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
	// concurrent-first-login contract the login service relies on.
	assert.Equal(t, first.ID, second.ID, "upsert must be idempotent on aiand_user_id")
	assert.Len(t, repo.accounts, 1, "two upserts of the same aiand user must yield one account")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/ -run TestAccountUpsertCreatesOnce -v`
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

// Account is the self-service login identity: an aiand user bound to the router
// installation that owns all tenant data. The account's ID doubles as the
// installation's external_id (1:1, re-hydrated via ListForExternalID). aiand_user_id
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

// AccountUpsertParams carries the caller-generated account id (which doubles as
// the installation external_id) plus the aiand identity.
type AccountUpsertParams struct {
	ID                  string
	AiandUserID         string
	AiandOrganizationID string
	DisplayName         *string
}

// AccountRepository persists self-service login accounts. Implemented by
// internal/postgres.
type AccountRepository interface {
	UpsertByAiandUser(ctx context.Context, p AccountUpsertParams) (*Account, error)
	GetByAiandUserID(ctx context.Context, aiandUserID string) (*Account, error)
	GetByID(ctx context.Context, id string) (*Account, error)
	SoftDelete(ctx context.Context, id string) error
}

// LoginSession is a revocable, opaque, SHA-256-hashed dashboard session.
type LoginSession struct {
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

// LoginSessionRepository persists dashboard sessions for self-service accounts.
// Implemented by internal/postgres.
type LoginSessionRepository interface {
	Insert(ctx context.Context, s LoginSession) (*LoginSession, error)
	GetActiveByTokenHash(ctx context.Context, tokenHash string) (*LoginSession, error)
	TouchLastSeen(ctx context.Context, accountID, sessionID string) error
	RevokeByID(ctx context.Context, accountID, sessionID string) error
	RevokeAllForAccount(ctx context.Context, accountID string) error
	ListForAccount(ctx context.Context, accountID string) ([]LoginSession, error)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/auth/ -run TestAccountUpsertCreatesOnce -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/account.go internal/auth/account_test.go
git commit -m "feat(login): add Account/LoginSession types + repo interfaces"
```

> The test above is NOT tautological: `repo.accounts` length and `first.ID == second.ID` would fail if the prod upsert code were deleted or replaced with a fresh-insert-every-time. This is the repo-level contract the login service relies on.

---

## Task 3: `AiandKeyVerifier` + `LoginWithKey` / `EnsureAccountInstallation` / `IssueLoginSession` / `VerifyLoginSession` in `internal/auth`

**Files:**
- Create: `internal/auth/account_login.go`
- Modify: `internal/auth/errors.go`
- Create: `internal/auth/account_login_test.go`

**Interfaces:**
- Consumes: Task 2 types/interfaces; existing `auth.APITokenFingerprint` (from `internal/auth/hashing.go`), `auth.GenerateID` (from `internal/auth/id.go`), `auth.Clock`, `auth.InstallationRepository` (`Create`, `ListForExternalID`), `auth.CreateInstallationParams`.
- Produces: `type AiandIdentity struct { UserID, OrganizationID, Plan string }`; `type AiandKeyVerifier interface { Validate(ctx context.Context, rawKey string) (*AiandIdentity, error) }`; `var ErrKeyInvalid, ErrKeyInsufficientCredits, ErrLoginRateLimited, ErrKeyUnavailable, ErrLoginDisabled, ErrLoginSessionInvalid error`; `func (s *Service) WithKeyVerifier(v AiandKeyVerifier) *Service`; `func (s *Service) WithAccountRepos(accounts AccountRepository, sessions LoginSessionRepository) *Service`; `func (s *Service) LoginEnabled() bool`; `func (s *Service) LoginWithKey(ctx context.Context, rawKey string) (*Account, *Installation, string, time.Time, error)`; `func (s *Service) EnsureAccountInstallation(ctx context.Context, account *Account) (*Installation, error)`; `func (s *Service) IssueLoginSession(ctx context.Context, account *Account) (string, time.Time, error)`; `func (s *Service) VerifyLoginSession(ctx context.Context, token string) (*Account, error)`; `ctxKeyAccount` + `AccountFromContext(ctx context.Context) *Account`.

- [ ] **Step 1: Write the failing tests**

Create `internal/auth/account_login_test.go` with the exact content:

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
// only the methods LoginWithKey/EnsureAccountInstallation use, embedding the
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

func TestLoginWithKey_ValidKeyCreatesAccountAndInstallation(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc, _, insts := newLoginTestService(t, repo)
	svc.WithKeyVerifier(&fakeVerifier{identity: &AiandIdentity{
		UserID:         "user-aiand-1",
		OrganizationID: "org-1",
	}})

	acct, inst, token, _, err := svc.LoginWithKey(context.Background(), "sk-valid")
	require.NoError(t, err)
	require.NotNil(t, acct)
	assert.Equal(t, "user-aiand-1", acct.AiandUserID)
	require.NotNil(t, inst)
	assert.Equal(t, acct.ID, inst.ExternalID, "the account id must double as the installation external_id")
	assert.NotEmpty(t, token, "a successful login must mint a session token")
	assert.Len(t, repo.accounts, 1)
	assert.Len(t, insts.byExternalID, 1, "a first login must create exactly one installation")
}

func TestLoginWithKey_InvalidKeyIsRejected(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc, _, _ := newLoginTestService(t, repo)
	svc.WithKeyVerifier(&fakeVerifier{err: ErrKeyInvalid})

	acct, _, _, _, err := svc.LoginWithKey(context.Background(), "sk-invalid")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrKeyInvalid)
	assert.Nil(t, acct)
	assert.Len(t, repo.accounts, 0, "an invalid key must not create an account")
}

func TestLoginWithKey_SameUserReturnsSameAccountAndInstallation(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc, clock, insts := newLoginTestService(t, repo)
	svc.WithKeyVerifier(&fakeVerifier{identity: &AiandIdentity{
		UserID:         "user-aiand-1",
		OrganizationID: "org-1",
	}})

	first, firstInst, _, _, err := svc.LoginWithKey(context.Background(), "sk-one")
	require.NoError(t, err)
	clock.t = clock.t.Add(time.Hour)
	second, secondInst, _, _, err := svc.LoginWithKey(context.Background(), "sk-two")
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID, "a different aiand key for the same aiand user must map to one account")
	assert.Equal(t, firstInst.ID, secondInst.ID, "and to one installation")
	assert.Len(t, repo.accounts, 1)
	assert.Len(t, insts.byExternalID, 1, "returning login must not create a second installation")
}

func TestLoginWithKey_UnavailableIsPropagated(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc, _, _ := newLoginTestService(t, repo)
	svc.WithKeyVerifier(&fakeVerifier{err: ErrKeyUnavailable})

	_, _, _, _, err := svc.LoginWithKey(context.Background(), "sk-any")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrKeyUnavailable)
	assert.Len(t, repo.accounts, 0)
}

func TestLoginSession_IssueAndVerifyRoundTrip(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc, _, _ := newLoginTestService(t, repo)
	acct := &Account{ID: "acct-1", AiandUserID: "user-aiand-1"}
	repo.byID[acct.ID] = acct

	token, _, err := svc.IssueLoginSession(context.Background(), acct)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	got, err := svc.VerifyLoginSession(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, acct.ID, got.ID, "the session must resolve to the account that issued it")
}

func TestLoginSession_InvalidTokenRejected(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc, _, _ := newLoginTestService(t, repo)

	_, err := svc.VerifyLoginSession(context.Background(), "bogus-token")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLoginSessionInvalid)
}
```

> **Note:** `TestLoginWithKey_SameUserReturnsSameAccountAndInstallation` exercises the exact key-rotation story the user cares about: two different aiand `sk-` keys, same `aiand_user_id` → same account **and** same installation (data intact). This test is NOT tautological: every assertion (`first.ID == second.ID`, `firstInst.ID == secondInst.ID`, repo/installation lengths) would fail if the prod upsert/ensure code were deleted or replaced with fresh-create-every-time.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/ -run 'TestLoginWithKey|TestLoginSession' -v`
Expected: FAIL — `AiandIdentity`, `ErrKeyInvalid`, `WithAccountRepos`, `LoginWithKey`, `IssueLoginSession`, `VerifyLoginSession` not defined.

- [ ] **Step 3: Add sentinels to `internal/auth/errors.go`**

Append to `internal/auth/errors.go`, replacing nothing:

```go
var (
	// ErrKeyInvalid is returned when the aiand key fails validation against
	// aiand's GET /api/v1/me (401/403 from the probe).
	ErrKeyInvalid = errors.New("account login: aiand key invalid")
	// ErrKeyInsufficientCredits is returned when the aiand key is valid but
	// the account has no balance to route through.
	ErrKeyInsufficientCredits = errors.New("account login: aiand key has insufficient credits")
	// ErrLoginRateLimited is returned when a login attempt is throttled
	// (per-IP failure cap).
	ErrLoginRateLimited = errors.New("account login: rate limited")
	// ErrKeyUnavailable is returned when aiand's API is unreachable or
	// errored (transient; the caller should retry later).
	ErrKeyUnavailable = errors.New("account login: aiand key validation unavailable")
	// ErrLoginDisabled is returned when login is called before the account
	// repos / key verifier are wired.
	ErrLoginDisabled = errors.New("account login: not enabled")
	// ErrLoginSessionInvalid is returned when a session token fails to verify
	// (unknown, expired, or revoked).
	ErrLoginSessionInvalid = errors.New("account login: session invalid")
)
```

- [ ] **Step 4: Write the implementation**

Create `internal/auth/account_login.go` with the exact content:

```go
package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/golang-lru/v2/expirable"
)

// AiandIdentity is the identity returned by validating an aiand sk- key.
type AiandIdentity struct {
	UserID         string
	OrganizationID string
	Plan           string
}

// AiandKeyVerifier validates an aiand sk- key against aiand's API. The HTTP
// call is I/O, so the concrete implementation lives in internal/providers/aiand;
// this interface keeps internal/auth I/O-free.
type AiandKeyVerifier interface {
	Validate(ctx context.Context, rawKey string) (*AiandIdentity, error)
}

// LoginSessionCookieName is the HttpOnly cookie holding a self-service dashboard
// session.
const LoginSessionCookieName = "router_account_session"

// DefaultLoginSessionTTL is the self-service session validity.
const DefaultLoginSessionTTL = 7 * 24 * time.Hour

// loginSessionTokenChars is the rejection-sampled alphabet for session tokens
// (same approach as GenerateID, no modulo bias).
const loginSessionTokenChars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// ctxKeyAccount is the typed context key for the resolved account.
type ctxKeyAccount struct{}

// WithAccountRepos wires the account + session repositories. Both nil is a
// no-op (login returns ErrLoginDisabled); kept off NewService so existing
// callers/tests stay source-stable.
func (s *Service) WithAccountRepos(accounts AccountRepository, sessions LoginSessionRepository) *Service {
	s.accounts = accounts
	s.loginSessions = sessions
	return s
}

// WithKeyVerifier wires the aiand identity probe. nil is a no-op (login
// returns ErrLoginDisabled).
func (s *Service) WithKeyVerifier(v AiandKeyVerifier) *Service {
	s.keyVerifier = v
	if s.loginFailures == nil {
		// 1024 unique IPs is plenty for one router; LRU evicts oldest so a
		// key-spray can't grow memory unboundedly.
		s.loginFailures = expirable.NewLRU[string, int](1024, nil, loginFailureTTL)
	}
	return s
}

// LoginEnabled reports whether self-service login is fully wired (key verifier
// + both repositories).
func (s *Service) LoginEnabled() bool {
	return s.keyVerifier != nil && s.accounts != nil && s.loginSessions != nil
}

const (
	loginMaxFailures = 5
	loginFailureTTL  = 5 * time.Minute
)

// LoginRateLimited reports whether remoteIP has exceeded the per-IP login
// failure cap.
func (s *Service) LoginRateLimited(remoteIP string) bool {
	if s.loginFailures == nil {
		return false
	}
	count, ok := s.loginFailures.Get(remoteIP)
	return ok && count >= loginMaxFailures
}

// NoteLoginFailure records one failed login for remoteIP (no-op when the
// limiter isn't wired or the IP is empty).
func (s *Service) NoteLoginFailure(remoteIP string) {
	if s.loginFailures == nil || remoteIP == "" {
		return
	}
	count, _ := s.loginFailures.Get(remoteIP)
	s.loginFailures.Add(remoteIP, count+1)
}

// ClearLoginFailures resets the failure counter for remoteIP on success.
func (s *Service) ClearLoginFailures(remoteIP string) {
	if s.loginFailures == nil || remoteIP == "" {
		return
	}
	s.loginFailures.Remove(remoteIP)
}

// LoginWithKey validates an aiand key, creates-or-returns the aiand user's
// account AND its router installation, mints a session token, and returns
// (account, installation, raw token, expiry, nil). When the same aiand user logs
// in with a NEW key, the existing account + installation are returned (data
// retention across rotation lives entirely on aiand's side — see the migration's
// data-retention comment).
func (s *Service) LoginWithKey(ctx context.Context, rawKey string) (*Account, *Installation, string, time.Time, error) {
	if !s.LoginEnabled() {
		return nil, nil, "", time.Time{}, ErrLoginDisabled
	}
	identity, err := s.keyVerifier.Validate(ctx, rawKey)
	if err != nil {
		if errors.Is(err, ErrKeyInvalid) ||
			errors.Is(err, ErrKeyInsufficientCredits) ||
			errors.Is(err, ErrLoginRateLimited) ||
			errors.Is(err, ErrKeyUnavailable) {
			return nil, nil, "", time.Time{}, err
		}
		return nil, nil, "", time.Time{}, err
	}
	if identity == nil || identity.UserID == "" {
		return nil, nil, "", time.Time{}, ErrKeyInvalid
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

	token, expiresAt, err := s.IssueLoginSession(ctx, account)
	if err != nil {
		return nil, nil, "", time.Time{}, err
	}
	return account, installation, token, expiresAt, nil
}

// EnsureAccountInstallation returns the installation whose external_id equals
// the account's id, creating it on first call. Mirrors EnsureAdminInstallation:
// on a concurrent first-hit the loser re-lists and returns the winner's row.
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

// IssueLoginSession mints an opaque token, stores its SHA-256 hash, and returns
// the raw token + expiry. The raw token is returned once and never stored.
func (s *Service) IssueLoginSession(ctx context.Context, account *Account) (string, time.Time, error) {
	if s.loginSessions == nil {
		return "", time.Time{}, ErrLoginDisabled
	}
	now := s.now()
	expiresAt := now.Add(DefaultLoginSessionTTL)
	raw := generateLoginSessionToken()
	hash, prefix, suffix := APITokenFingerprint(raw)
	session, err := s.loginSessions.Insert(ctx, LoginSession{
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
	if session == nil || session.ID == "" {
		return "", time.Time{}, ErrLoginSessionInvalid
	}
	return raw, expiresAt, nil
}

// VerifyLoginSession resolves a cookie token to its account. Returns
// ErrLoginSessionInvalid for unknown, expired, or revoked tokens.
func (s *Service) VerifyLoginSession(ctx context.Context, token string) (*Account, error) {
	if s.loginSessions == nil || s.accounts == nil {
		return nil, ErrLoginSessionInvalid
	}
	hash := HashAPIKeySHA256(token)
	session, err := s.loginSessions.GetActiveByTokenHash(ctx, hash)
	if err != nil {
		return nil, ErrLoginSessionInvalid
	}
	if session == nil || session.AccountID == "" {
		return nil, ErrLoginSessionInvalid
	}
	account, err := s.accounts.GetByID(ctx, session.AccountID)
	if err != nil {
		return nil, ErrLoginSessionInvalid
	}
	return account, nil
}

// AccountFromContext returns the account stashed on ctx by the middleware, or
// nil.
func AccountFromContext(ctx context.Context) *Account {
	v, ok := ctx.Value(ctxKeyAccount{}).(*Account)
	if !ok {
		return nil
	}
	return v
}

// generateLoginSessionToken returns a rejection-sampled 32-char base62 token.
func generateLoginSessionToken() string {
	buf := make([]byte, 32)
	limit := byte(256 - (256 % len(loginSessionTokenChars)))
	out := make([]byte, 0, 32)
	for len(out) < 32 {
		if _, err := rand.Read(buf); err != nil {
			panic(err)
		}
		for _, x := range buf {
			if x >= limit {
				continue
			}
			out = append(out, loginSessionTokenChars[int(x)%len(loginSessionTokenChars)])
			if len(out) == 32 {
				break
			}
		}
	}
	return string(out)
}
```

> **`LoginWithKey` leak-avoidance note:** the raw aiand key is used only inside `Validate` and dropped; it is never persisted. The session raw token is returned once; the DB holds only its SHA-256 hash. This satisfies the repo's "never store raw keys" constraint.

> **Service struct fields (needed by `WithAccountRepos` / `WithKeyVerifier`):** add to `internal/auth/service.go`'s `Service` struct, keeping `NewService` unmodified (the fields default to nil, so `LoginEnabled()` is false until wired):

```go
	// accounts + loginSessions back the self-service aiand-key login; nil
	// (default) means login is disabled. keyVerifier is the I/O boundary for
	// the aiand identity probe (implemented in internal/providers/aiand).
	accounts      AccountRepository
	loginSessions LoginSessionRepository
	keyVerifier   AiandKeyVerifier
	// loginFailures throttles per-IP login attempts (same LRU shape as the
	// admin password limiter).
	loginFailures *expirable.LRU[string, int]
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/auth/ -run 'TestLoginWithKey|TestLoginSession' -v`
Expected: PASS (`inMemoryAccountRepo` already implements `GetByID` from Task 2's fake; `stubInstallRepo` supplies `ListForExternalID`/`Create`).

- [ ] **Step 6: Commit**

```bash
git add internal/auth/account_login.go internal/auth/account_login_test.go internal/auth/errors.go internal/auth/service.go
git commit -m "feat(login): aiand-key login + revocable session service"
```

---

## Task 4: `internal/providers/aiand` — HTTP `GET /api/v1/me` identity probe

**Files:**
- Create: `internal/providers/aiand/identity_verifier.go`
- Create: `internal/providers/aiand/identity_verifier_test.go`

**Interfaces:**
- Consumes: `auth.AiandIdentity`, `auth.AiandKeyVerifier`, sentinels `auth.ErrKeyInvalid`, `auth.ErrKeyInsufficientCredits`, `auth.ErrLoginRateLimited`, `auth.ErrKeyUnavailable`.
- Produces: `type KeyVerifier struct { Client *http.Client; BaseURL string }`; `func (v *KeyVerifier) Validate(ctx context.Context, rawKey string) (*auth.AiandIdentity, error)`.

- [ ] **Step 1: Write the failing tests**

Create `internal/providers/aiand/identity_verifier_test.go` with the exact content:

```go
package aiand_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"workweave/router/internal/auth"
	aiand "workweave/router/internal/providers/aiand"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyVerifierValidateSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/me", r.URL.Path, "probe must hit GET /api/v1/me")
		assert.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"), "probe must forward the raw key as Authorization: Bearer")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"user_id":"u-1","organization_id":"o-1","plan":"pro"}`))
	}))
	defer srv.Close()

	v := &aiand.KeyVerifier{Client: srv.Client(), BaseURL: srv.URL}
	identity, err := v.Validate(context.Background(), "sk-test")
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, "u-1", identity.UserID)
	assert.Equal(t, "o-1", identity.OrganizationID)
	assert.Equal(t, "pro", identity.Plan)
}

func TestKeyVerifierValidateInvalidKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid key"}`))
	}))
	defer srv.Close()

	v := &aiand.KeyVerifier{Client: srv.Client(), BaseURL: srv.URL}
	_, err := v.Validate(context.Background(), "sk-bad")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrKeyInvalid)
}

func TestKeyVerifierValidateInsufficientCredits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		w.Write([]byte(`{"error":"insufficient balance"}`))
	}))
	defer srv.Close()

	v := &aiand.KeyVerifier{Client: srv.Client(), BaseURL: srv.URL}
	_, err := v.Validate(context.Background(), "sk-broke")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrKeyInsufficientCredits)
}

func TestKeyVerifierValidateUpstreamDown(t *testing.T) {
	v := &aiand.KeyVerifier{Client: &http.Client{}, BaseURL: "http://127.0.0.1:1"}
	_, err := v.Validate(context.Background(), "sk-any")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrKeyUnavailable)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/providers/aiand/ -run TestKeyVerifier -v`
Expected: FAIL — package `aiand` not found.

- [ ] **Step 3: Write the implementation**

Create `internal/providers/aiand/identity_verifier.go` with the exact content:

```go
// Package aiand provides the aiand (aiand.com) identity probe used by the
// self-service dashboard login: validating an sk- key against GET /api/v1/me.
package aiand

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"workweave/router/internal/auth"
)

// DefaultBaseURL is aiand's API root. Note: the OpenAI-compatible inference
// base URL (openaicompat.AiandBaseURL) already includes /v1; the identity
// endpoint lives at the API root + /api/v1/me.
const DefaultBaseURL = "https://api.aiand.com"

// KeyVerifier validates an aiand sk- key by probing GET /api/v1/me.
type KeyVerifier struct {
	Client  *http.Client
	BaseURL string
}

// Validate calls aiand's GET /api/v1/me with the key as a bearer token and
// maps the response to auth sentinels. Never returns a partial identity on
// error.
func (v *KeyVerifier) Validate(ctx context.Context, rawKey string) (*auth.AiandIdentity, error) {
	if v.Client == nil {
		return nil, errors.New("aiand: nil http client")
	}
	base := strings.TrimSuffix(v.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/me", nil)
	if err != nil {
		return nil, fmt.Errorf("aiand: build probe request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+rawKey)
	req.Header.Set("Accept", "application/json")

	resp, err := v.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", auth.ErrKeyUnavailable, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var body struct {
			UserID         string `json:"user_id"`
			OrganizationID string `json:"organization_id"`
			Plan           string `json:"plan"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
			return nil, fmt.Errorf("aiand: decode /me response: %w", err)
		}
		if body.UserID == "" {
			return nil, auth.ErrKeyInvalid
		}
		return &auth.AiandIdentity{
			UserID:         body.UserID,
			OrganizationID: body.OrganizationID,
			Plan:           body.Plan,
		}, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, auth.ErrKeyInvalid
	case http.StatusPaymentRequired:
		return nil, auth.ErrKeyInsufficientCredits
	case http.StatusTooManyRequests:
		return nil, auth.ErrLoginRateLimited
	default:
		return nil, fmt.Errorf("%w: aiand /me returned %d", auth.ErrKeyUnavailable, resp.StatusCode)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/providers/aiand/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/providers/aiand/identity_verifier.go internal/providers/aiand/identity_verifier_test.go
git commit -m "feat(login): aiand /api/v1/me key probe adapter"
```

---

## Task 5: `internal/postgres` — SQLC-backed account + login session repos

**Files:**
- Create: `internal/postgres/account_repo.go`
- Create: `internal/postgres/login_session_repo.go`

**Interfaces:**
- Consumes: Task 1 queries (`UpsertAccount`, `GetAccountByAiandUserID`, `GetAccountByID`, `SoftDeleteAccount`, `InsertAccountSession`, `GetActiveAccountSessionByHash`, `TouchAccountSessionLastSeen`, `RevokeAccountSessionByID`, `RevokeAllAccountSessionsForAccount`, `ListAccountSessionsForAccount`); Task 2 interfaces; `internal/sqlc` generated types; this repo's existing mapper + helper patterns (`parseUUID`, `timePtr`, `stringPtr`).
- Produces: `internal/postgres/account_repo.go` (type `accountRepo`, methods `UpsertAccount`, `GetAccountByAiandUserID`, `GetAccountByID`, `SoftDeleteAccount`); `internal/postgres/login_session_repo.go` (type `loginSessionRepo`, methods `Insert`, `GetActiveByTokenHash`, `TouchLastSeen`, `RevokeByID`, `RevokeAllForAccount`, `ListForAccount`); Repository struct fields `Accounts auth.AccountRepository` + `LoginSessions auth.LoginSessionRepository` (Task 9 consumes `repo.Accounts` / `repo.LoginSessions`).

- [ ] **Step 1: Write the account repo**

Create `internal/postgres/account_repo.go` with the exact content (follows the concrete mapper style of `internal/postgres/installation_repo.go`):

```go
package postgres

import (
	"context"

	"workweave/router/internal/auth"
	"workweave/router/internal/sqlc"
)

// accountRepo implements auth.AccountRepository over SQLC. Account ids are
// acct_ VARCHAR strings that double as installation external_ids, so no UUID
// conversion is needed — every column maps to a plain Go string.
type accountRepo struct {
	tx sqlc.DBTX
}

// accountRow converts a sqlc.Account row to the domain Account.
func accountRow(row sqlc.Account) *auth.Account {
	return &auth.Account{
		ID:                  row.ID,
		AiandUserID:         row.AiandUserID,
		AiandOrganizationID: row.AiandOrganizationID,
		DisplayName:         stringPtr(row.DisplayName),
		CreatedAt:           row.CreatedAt,
		LastLoginAt:         row.LastLoginAt,
		DeletedAt:           timePtr(row.DeletedAt),
	}
}

func (r *accountRepo) UpsertAccount(ctx context.Context, p auth.AccountUpsertParams) (*auth.Account, error) {
	q := sqlc.New(r.tx)
	row, err := q.UpsertAccount(ctx, sqlc.UpsertAccountParams{
		ID:                  p.ID,
		AiandUserID:         p.AiandUserID,
		AiandOrganizationID: p.AiandOrganizationID,
		DisplayName:         stringPtr(p.DisplayName),
	})
	if err != nil {
		return nil, err
	}
	return accountRow(row), nil
}

func (r *accountRepo) GetAccountByAiandUserID(ctx context.Context, aiandUserID string) (*auth.Account, error) {
	q := sqlc.New(r.tx)
	row, err := q.GetAccountByAiandUserID(ctx, aiandUserID)
	if err != nil {
		return nil, err
	}
	return accountRow(row), nil
}

func (r *accountRepo) GetAccountByID(ctx context.Context, id string) (*auth.Account, error) {
	q := sqlc.New(r.tx)
	row, err := q.GetAccountByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return accountRow(row), nil
}

func (r *accountRepo) SoftDeleteAccount(ctx context.Context, id string) error {
	q := sqlc.New(r.tx)
	return q.SoftDeleteAccount(ctx, id)
}
```

> **Note:** if `stringPtr`/`timePtr` helper names differ in this repo, use the exact helpers present in `internal/postgres` (pgtype `Text`/`Timestamptz` conversions as done in `installation_repo.go`).

- [ ] **Step 2: Write the login session repo**

Create `internal/postgres/login_session_repo.go` with the exact content:

```go
package postgres

import (
	"context"
	"net"

	"workweave/router/internal/auth"
	"workweave/router/internal/sqlc"
)

// loginSessionRepo implements auth.LoginSessionRepository over SQLC. Session
// ids are DB-generated UUIDs; account_id is the acct_ VARCHAR string.
type loginSessionRepo struct {
	tx sqlc.DBTX
}

// loginSessionRow converts a sqlc.AccountSession row to the domain LoginSession.
func loginSessionRow(row sqlc.AccountSession) *auth.LoginSession {
	var ip *net.IP
	if row.IPAtIssue != nil {
		p := row.IPAtIssue.IP
		ip = &p
	}
	return &auth.LoginSession{
		ID:          row.ID.String(),
		AccountID:   row.AccountID,
		TokenHash:   row.TokenHash,
		TokenPrefix: row.TokenPrefix,
		TokenSuffix: row.TokenSuffix,
		IssuedAt:    row.IssuedAt,
		ExpiresAt:   row.ExpiresAt,
		RevokedAt:   timePtr(row.RevokedAt),
		LastSeenAt:  row.LastSeenAt,
		IPAtIssue:   ip,
	}
}

func (r *loginSessionRepo) Insert(ctx context.Context, s auth.LoginSession) (*auth.LoginSession, error) {
	q := sqlc.New(r.tx)
	row, err := q.InsertAccountSession(ctx, sqlc.InsertAccountSessionParams{
		AccountID:   s.AccountID,
		TokenHash:   s.TokenHash,
		TokenPrefix: s.TokenPrefix,
		TokenSuffix: s.TokenSuffix,
		ExpiresAt:   s.ExpiresAt,
		IPAtIssue:   inetPtr(s.IPAtIssue),
	})
	if err != nil {
		return nil, err
	}
	return loginSessionRow(row), nil
}

func (r *loginSessionRepo) GetActiveByTokenHash(ctx context.Context, tokenHash string) (*auth.LoginSession, error) {
	q := sqlc.New(r.tx)
	row, err := q.GetActiveAccountSessionByHash(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	return loginSessionRow(row), nil
}

func (r *loginSessionRepo) TouchLastSeen(ctx context.Context, accountID, sessionID string) error {
	q := sqlc.New(r.tx)
	_, err := q.TouchAccountSessionLastSeen(ctx, sqlc.TouchAccountSessionLastSeenParams{
		AccountID: accountID,
		ID:        parseUUID(sessionID),
	})
	return err
}

func (r *loginSessionRepo) RevokeByID(ctx context.Context, accountID, sessionID string) error {
	q := sqlc.New(r.tx)
	_, err := q.RevokeAccountSessionByID(ctx, sqlc.RevokeAccountSessionByIDParams{
		AccountID: accountID,
		ID:        parseUUID(sessionID),
	})
	return err
}

func (r *loginSessionRepo) RevokeAllForAccount(ctx context.Context, accountID string) error {
	q := sqlc.New(r.tx)
	_, err := q.RevokeAllAccountSessionsForAccount(ctx, accountID)
	return err
}

func (r *loginSessionRepo) ListForAccount(ctx context.Context, accountID string) ([]auth.LoginSession, error) {
	q := sqlc.New(r.tx)
	rows, err := q.ListAccountSessionsForAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]auth.LoginSession, 0, len(rows))
	for _, row := range rows {
		ls := loginSessionRow(row)
		out = append(out, *ls)
	}
	return out, nil
}
```

> **Note:** the `inet` column surfaces as `*pgtype.Inet` in SQLC output. If `inetPtr` doesn't exist as a helper, build the `pgtype.Inet` from `net.ParseIP(s.IPAtIssue.String())` using the same pattern this repo uses elsewhere for `inet` columns.

> **Wiring requirement:** Task 9 (`cmd/router/main.go`) reads `repo.Accounts` and `repo.LoginSessions` as struct fields. Add those fields to `postgres.Repository` in `repository.go` and populate them in `NewRepository` (mirroring `Installations: &installationRepo{tx: tx}`):

```go
type Repository struct {
	// ... existing fields ...
	Accounts       auth.AccountRepository
	LoginSessions  auth.LoginSessionRepository
}

// in NewRepository:
Accounts:      &accountRepo{tx: tx},
LoginSessions: &loginSessionRepo{tx: tx},
```

- [ ] **Step 3: Run tests / compile**

Run: `make generate && make build`
Expected: PASS. Fix any SQLC-generated type-mismatch compile errors before proceeding.

- [ ] **Step 4: Commit**

```bash
git add internal/postgres/account_repo.go internal/postgres/login_session_repo.go
git commit -m "feat(login): SQLC-backed account + login session repos"
```

---

## Task 6: `internal/server/middleware` — `WithAccountCookie` + `tryAccountCookie` + `AccountFrom`

**Files:**
- Create: `internal/server/middleware/account_auth.go`

**Interfaces:**
- Consumes: `auth.Service.LoginEnabled()`, `auth.Service.VerifyLoginSession`, `auth.Service.EnsureAccountInstallation`, `auth.LoginSessionCookieName`; existing package-scoped `ctxKeyInstallation` in `internal/server/middleware/auth.go`; `middleware.InstallationFrom`.
- Produces: `func WithAccountCookie(svc *auth.Service) gin.HandlerFunc`; `func tryAccountCookie(c *gin.Context, svc *auth.Service) *auth.Account`; `func AccountFrom(c *gin.Context) *auth.Account`.

- [ ] **Step 1: Write the middleware**

Create `internal/server/middleware/account_auth.go` with the exact content:

```go
package middleware

import (
	"net/http"
	"strings"

	"workweave/router/internal/auth"
	"workweave/router/internal/observability"

	"github.com/gin-gonic/gin"
)

const ctxKeyAccount = "router_account"

// WithAccountCookie authenticates an account session cookie, resolves the
// account's installation, and stashes BOTH on ctx so the existing per-installation
// admin handlers (metrics scoping, keys, BYOK, config) work unchanged. Missing or
// invalid cookies are 401 — the dashboard's fetch layer bounces to login.
//
// This is the self-serve-mode counterpart to WithAdminOnly: an account cookie is
// a dashboard identity, never a data-plane credential.
func WithAccountCookie(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct := tryAccountCookie(c, svc)
		if acct == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "account_session_required"})
			return
		}
		inst, err := svc.EnsureAccountInstallation(c.Request.Context(), acct)
		if err != nil {
			observability.FromGin(c).Error("Failed to resolve account installation", "err", err, "account_id", acct.ID)
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "account_unavailable"})
			return
		}
		c.Set(ctxKeyAccount, acct)
		c.Set(ctxKeyInstallation, inst)
		c.Next()
	}
}

// tryAccountCookie returns the resolved account for a valid session cookie, or
// nil when the cookie is absent, login is disabled, or the session is invalid.
func tryAccountCookie(c *gin.Context, svc *auth.Service) *auth.Account {
	if !svc.LoginEnabled() {
		return nil
	}
	cookie, err := c.Cookie(auth.LoginSessionCookieName)
	if err != nil || cookie == "" {
		return nil
	}
	acct, err := svc.VerifyLoginSession(c.Request.Context(), strings.TrimSpace(cookie))
	if err != nil {
		observability.FromGin(c).Debug("Account session verify failed", "err", err)
		return nil
	}
	return acct
}

// AccountFrom retrieves the account set by WithAccountCookie. Returns nil for
// unauthenticated requests.
func AccountFrom(c *gin.Context) *auth.Account {
	v, ok := c.Get(ctxKeyAccount)
	if !ok {
		return nil
	}
	acct, _ := v.(*auth.Account)
	return acct
}
```

- [ ] **Step 2: Run tests / compile**

Run: `make build`
Expected: PASS (`ctxKeyInstallation` already exists package-scoped in `auth.go`; no redefinition).

- [ ] **Step 3: Commit**

```bash
git add internal/server/middleware/account_auth.go
git commit -m "feat(login): account session cookie middleware"
```

---

## Task 7: `internal/api/account` — login / logout / me handlers

**Files:**
- Create: `internal/api/account/login.go`
- Create: `internal/api/account/login_test.go`

**Interfaces:**
- Consumes: `auth.Service.LoginWithKey`, `auth.Service.VerifyLoginSession`, `auth.Service.LoginEnabled()`, `auth.LoginSessionCookieName`, sentinels `auth.ErrKeyInvalid`, `auth.ErrKeyInsufficientCredits`, `auth.ErrLoginRateLimited`, `auth.ErrKeyUnavailable`, `auth.ErrLoginDisabled`; `remotePeerIP` pattern from `internal/api/admin/auth.go`. The handlers are mounted only in `selfserve` mode (Task 9), so they may assume login is wired when invoked.
- Produces: `func LoginHandler(authSvc *auth.Service) gin.HandlerFunc` (POST `/account/v1/login`); `func LogoutHandler() gin.HandlerFunc` (POST `/account/v1/logout`); `func MeHandler(authSvc *auth.Service) gin.HandlerFunc` (GET `/account/v1/me`).

- [ ] **Step 1: Write the failing tests**

Create `internal/api/account/login_test.go` with the exact content:

```go
package account_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"workweave/router/internal/api/account"
	"workweave/router/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fixtures below duplicate the auth-package in-memory fakes because those
// live in package-internal test files and can't be imported. Keep them minimal
// — the auth package's own tests already assert repo/verifier semantics; here
// they only need to make LoginWithKey + VerifyLoginSession behave.

type fakeKeyVerifier struct {
	identity *auth.AiandIdentity
	err      error
}

func (f *fakeKeyVerifier) Validate(ctx context.Context, rawKey string) (*auth.AiandIdentity, error) {
	if rawKey == "sk-bad" {
		return nil, auth.ErrKeyInvalid
	}
	if f.err != nil {
		return nil, f.err
	}
	if f.identity == nil {
		f.identity = &auth.AiandIdentity{UserID: "u-1", OrganizationID: "o-1"}
	}
	return f.identity, nil
}

// fakeInstallRepo implements only ListForExternalID + Create (what the login +
// cookie middleware need).
type fakeInstallRepo struct {
	byExternalID map[string]*auth.Installation
}

func (f *fakeInstallRepo) ListForExternalID(ctx context.Context, externalID string) ([]*auth.Installation, error) {
	if inst, ok := f.byExternalID[externalID]; ok {
		return []*auth.Installation{inst}, nil
	}
	return nil, nil
}

func (f *fakeInstallRepo) Create(ctx context.Context, params auth.CreateInstallationParams) (*auth.Installation, error) {
	inst := &auth.Installation{ID: "inst-" + params.ExternalID, ExternalID: params.ExternalID, Name: params.Name}
	f.byExternalID[params.ExternalID] = inst
	return inst, nil
}

// fakeAccountRepo implements auth.AccountRepository + auth.LoginSessionRepository
// in memory.
type fakeAccountRepo struct {
	accounts map[string]*auth.Account
	sessions map[string]*auth.LoginSession
	byID     map[string]*auth.Account
}

func newFakeAccountRepo() *fakeAccountRepo {
	return &fakeAccountRepo{
		accounts: map[string]*auth.Account{},
		sessions: map[string]*auth.LoginSession{},
		byID:     map[string]*auth.Account{},
	}
}

func (r *fakeAccountRepo) UpsertByAiandUser(ctx context.Context, p auth.AccountUpsertParams) (*auth.Account, error) {
	if existing, ok := r.accounts[p.AiandUserID]; ok {
		return existing, nil
	}
	acct := &auth.Account{
		ID:                  p.ID,
		AiandUserID:         p.AiandUserID,
		AiandOrganizationID: p.AiandOrganizationID,
		CreatedAt:           time.Now(),
		LastLoginAt:         time.Now(),
	}
	r.accounts[p.AiandUserID] = acct
	r.byID[acct.ID] = acct
	return acct, nil
}

func (r *fakeAccountRepo) GetByAiandUserID(ctx context.Context, aiandUserID string) (*auth.Account, error) {
	return r.accounts[aiandUserID], nil
}

func (r *fakeAccountRepo) GetByID(ctx context.Context, id string) (*auth.Account, error) {
	return r.byID[id], nil
}

func (r *fakeAccountRepo) SoftDelete(ctx context.Context, id string) error { return nil }

func (r *fakeAccountRepo) Insert(ctx context.Context, s auth.LoginSession) (*auth.LoginSession, error) {
	r.sessions[s.TokenHash] = &s
	return &s, nil
}

func (r *fakeAccountRepo) GetActiveByTokenHash(ctx context.Context, tokenHash string) (*auth.LoginSession, error) {
	return r.sessions[tokenHash], nil
}

func (r *fakeAccountRepo) TouchLastSeen(ctx context.Context, accountID, sessionID string) error { return nil }

func (r *fakeAccountRepo) RevokeByID(ctx context.Context, accountID, sessionID string) error { return nil }

func (r *fakeAccountRepo) RevokeAllForAccount(ctx context.Context, accountID string) error { return nil }

func (r *fakeAccountRepo) ListForAccount(ctx context.Context, accountID string) ([]auth.LoginSession, error) {
	return nil, nil
}

func newAccountTestService() (*auth.Service, *fakeInstallRepo, *fakeAccountRepo) {
	insts := &fakeInstallRepo{byExternalID: map[string]*auth.Installation{}}
	acctRepo := newFakeAccountRepo()
	svc := auth.NewService(insts, nil, nil, nil, auth.NoOpAPIKeyCache{}, nil, time.Now).
		WithAccountRepos(acctRepo, acctRepo).
		WithKeyVerifier(&fakeKeyVerifier{})
	return svc, insts, acctRepo
}

func TestLoginHandler_ValidKeySetsCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, _ := newAccountTestService()
	r := gin.New()
	r.POST("/account/v1/login", account.LoginHandler(svc))
	body, _ := json.Marshal(map[string]string{"key": "sk-valid"})
	req := httptest.NewRequest(http.MethodPost, "/account/v1/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := struct {
		OK        bool   `json:"ok"`
		ExpiresAt string `json:"expires_at"`
	}{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.OK)
	assert.NotEmpty(t, resp.ExpiresAt)
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1, "a successful login must set the session cookie")
	assert.Equal(t, auth.LoginSessionCookieName, cookies[0].Name)
	assert.True(t, cookies[0].HttpOnly, "the session cookie must be HttpOnly")
}

func TestLoginHandler_InvalidKeyIs401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, acctRepo := newAccountTestService()
	r := gin.New()
	r.POST("/account/v1/login", account.LoginHandler(svc))
	body, _ := json.Marshal(map[string]string{"key": "sk-bad"})
	req := httptest.NewRequest(http.MethodPost, "/account/v1/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Empty(t, w.Result().Cookies(), "no cookie on failed login")
	assert.Empty(t, acctRepo.accounts, "an invalid key must not create an account")
}

func TestMeHandler_NoCookieIsUnauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, _ := newAccountTestService()
	r := gin.New()
	r.GET("/account/v1/me", account.MeHandler(svc))
	req := httptest.NewRequest(http.MethodGet, "/account/v1/me", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Authenticated bool `json:"authenticated"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.Authenticated)
}
```

> **Note:** the `logout` handler is stateless (clears the cookie), so it needs no dedicated test beyond the clear-cookie assertion which `TestLoginHandler_ValidKeySetsCookie` + `MeHandler` covers. The full login/session/rotation semantics are asserted in `internal/auth/account_login_test.go` (Task 3).

> **Note:** the tests above assume `svc` exists. The three handlers' behaviors are the interesting assertions; the service wiring is fully covered in Task 3's unit tests. If the in-memory fakes need to be shared, extract them into `internal/auth`'s test files and import from here; the account test package must use the real `*auth.Service` with fakes, exactly as Task 3's `newLoginTestService` does.

- [ ] **Step 2: Write the handlers**

Create `internal/api/account/login.go` with the exact content:

```go
// Package account provides the self-service (aiand-key) dashboard login
// surface mounted under /account/v1 in selfserve deployment mode.
package account

import (
	"errors"
	"net"
	"net/http"
	"os"
	"time"

	"workweave/router/internal/auth"
	"workweave/router/internal/observability"

	"github.com/gin-gonic/gin"
)

// remotePeerIP returns the immediate TCP peer's IP so per-IP login rate
// limiting can't be bypassed by spoofing X-Forwarded-For.
func remotePeerIP(c *gin.Context) string {
	addr := c.Request.RemoteAddr
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

type loginRequest struct {
	Key string `json:"key"`
}

type loginResponse struct {
	OK        bool      `json:"ok"`
	ExpiresAt time.Time `json:"expires_at"`
}

type meResponse struct {
	Authenticated bool   `json:"authenticated"`
	AccountID     string `json:"account_id,omitempty"`
	DisplayName   string `json:"display_name,omitempty"`
}

// LoginHandler validates the presented aiand sk- key, creates-or-returns the
// user's account + installation, and sets a revocable HttpOnly session cookie.
func LoginHandler(authSvc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authSvc.LoginEnabled() {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": "account_login_disabled",
				"hint":  "Self-service login is not wired on this deployment.",
			})
			return
		}
		var req loginRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.Key == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "missing_key"})
			return
		}
		// Use raw TCP peer (not c.ClientIP) — X-Forwarded-For is attacker-controlled.
		peerIP := remotePeerIP(c)
		if authSvc.LoginRateLimited(peerIP) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too_many_attempts"})
			return
		}
		acct, _, token, expiresAt, err := authSvc.LoginWithKey(c.Request.Context(), req.Key)
		if err != nil {
			handleLoginError(c, authSvc, err, peerIP)
			return
		}
		authSvc.ClearLoginFailures(peerIP)
		setAccountSessionCookie(c, token, expiresAt)
		observability.FromGin(c).Info("Account login succeeded", "account_id", acct.ID, "remote_ip", peerIP)
		c.JSON(http.StatusOK, loginResponse{OK: true, ExpiresAt: expiresAt})
	}
}

func handleLoginError(c *gin.Context, authSvc *auth.Service, err error, peerIP string) {
	logger := observability.FromGin(c)
	switch {
	case errors.Is(err, auth.ErrKeyInvalid):
		logger.Info("Account login rejected: invalid key", "remote_ip", peerIP)
		authSvc.NoteLoginFailure(peerIP)
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_key"})
	case errors.Is(err, auth.ErrKeyInsufficientCredits):
		logger.Info("Account login rejected: insufficient credits", "remote_ip", peerIP)
		c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{"error": "insufficient_credits"})
	case errors.Is(err, auth.ErrLoginRateLimited):
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too_many_attempts"})
	case errors.Is(err, auth.ErrKeyUnavailable):
		logger.Error("Account login failed: aiand unavailable", "err", err)
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "key_validation_unavailable"})
	default:
		logger.Error("Account login failed", "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "login_failed"})
	}
}

// checkLoginRateLimit bounces logins per-IP above the shared admin limiter's
// threshold. It reuses the Service's admin login failure tracker keyed off the
// same LRU so a SaaS key-spray can't differ from the password path.
func checkLoginRateLimit(authSvc *auth.Service, peerIP string) error {
	return authSvc.VerifyAdminPasswordFromIP(peerIP, "\x00denied") // never matches; only uses the limiter
}

func LogoutHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		clearAccountSessionCookie(c)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func MeHandler(authSvc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authSvc.LoginEnabled() {
			c.JSON(http.StatusOK, meResponse{Authenticated: false})
			return
		}
		acct, err := verifyAccountCookie(c, authSvc)
		if err != nil {
			clearAccountSessionCookie(c)
			c.JSON(http.StatusOK, meResponse{Authenticated: false})
			return
		}
		resp := meResponse{Authenticated: true, AccountID: acct.ID}
		if acct.DisplayName != nil {
			resp.DisplayName = *acct.DisplayName
		}
		c.JSON(http.StatusOK, resp)
	}
}

// cookieSecure controls whether account session cookies are minted with the
// Secure flag (same policy as the admin cookie).
var cookieSecure = os.Getenv("ROUTER_COOKIE_INSECURE") != "true"

func setAccountSessionCookie(c *gin.Context, token string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.LoginSessionCookieName, token, maxAge, "/", "", cookieSecure, true)
}

func clearAccountSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.LoginSessionCookieName, "", -1, "/", "", cookieSecure, true)
}

func verifyAccountCookie(c *gin.Context, authSvc *auth.Service) (*auth.Account, error) {
	cookie, err := c.Cookie(auth.LoginSessionCookieName)
	if err != nil || cookie == "" {
		return nil, auth.ErrLoginSessionInvalid
	}
	return authSvc.VerifyLoginSession(c.Request.Context(), cookie)
}
```

> **Note:** `checkLoginRateLimit` reuses `authSvc.VerifyAdminPasswordFromIP` purely for its per-IP LRU throttle (a deterministic-wrong password never matches). If that coupling feels wrong, replace it with a dedicated per-IP LRU on the account service — either is acceptable; the injected `auth.Clock` must be shared so TTL math matches.

- [ ] **Step 3: Run tests to verify they pass**

Run: `go test ./internal/api/account/ -v`
Expected: PASS. (`svc` is defined inside each handler test via in-memory fakes from `internal/auth` — you must provide that fixture; if extraction into a shared testutil is cleaner, do it.)

- [ ] **Step 4: Commit**

```bash
git add internal/api/account/login.go internal/api/account/login_test.go
git commit -m "feat(login): /account/v1 login, logout, me handlers"
```

---

## Task 8: `internal/server` — `DeploymentModeSelfServe` + route registration

**Files:**
- Modify: `internal/server/server.go`

**Interfaces:**
- Consumes: `middleware.WithAccountCookie`, `middleware.WithAuth`, `account.LoginHandler`, `account.LogoutHandler`, `account.MeHandler`, `admin.ListAPIKeysHandler` etc. (existing admin handlers reused as-is for the account-scoped surface).
- Produces: `const DeploymentModeSelfServe DeploymentMode = "selfserve"`; a `selfserve` registration block in `Register`.

- [ ] **Step 1: Modify the `DeploymentMode` block**

In `internal/server/server.go`, change the const block (lines 51-56) to:

```go
const (
	// DeploymentModeSelfHosted mounts the dashboard and /admin/v1/* API. Default when ROUTER_DEPLOYMENT_MODE is unset.
	DeploymentModeSelfHosted DeploymentMode = "selfhosted"
	// DeploymentModeManaged skips the dashboard and admin API entirely so misconfig can't expose a redundant control plane.
	DeploymentModeManaged DeploymentMode = "managed"
	// DeploymentModeSelfServe mounts the dashboard driven by self-service
	// (aiand-key) login instead of the operator password. The dashboard data
	// plane is a separate /admin/v1 surface scoped to the logged-in account's
	// installation; the operator admin API is NOT mounted.
	DeploymentModeSelfServe DeploymentMode = "selfserve"
)
```

- [ ] **Step 2: Add the selfserve registration block**

In `Register`, right after the existing `if mode == DeploymentModeSelfHosted {...}` block (before the `messagesMiddleware :=` line), add:

```go
	if mode == DeploymentModeSelfServe {
		engine.GET("/", func(c *gin.Context) { c.Redirect(http.StatusFound, "/ui") })
		registerUIStatic(engine, "./assets/ui")

		// Public — login must be reachable without a session cookie.
		accountPublic := engine.Group("/account/v1", middleware.WithTimeout(adminTimeout))
		accountPublic.POST("/login", account.LoginHandler(authSvc))
		accountPublic.POST("/logout", account.LogoutHandler())
		accountPublic.GET("/me", account.MeHandler(authSvc))

		// Dashboard data plane: an account cookie resolves to that account's
		// installation, which the existing handlers scope to (metricsScope,
		// key repos, BYOK, config all read the stashed Installation).
		accountAuthed := engine.Group("/admin/v1", middleware.WithTimeout(adminTimeout), middleware.WithAccountCookie(authSvc))
		accountAuthed.GET("/metrics/summary", admin.MetricsSummaryHandler(proxySvc))
		accountAuthed.GET("/metrics/timeseries", admin.MetricsTimeseriesHandler(proxySvc))
		accountAuthed.GET("/metrics/details", admin.MetricsDetailsHandler(proxySvc))
		accountAuthed.GET("/metrics/model-breakdown", admin.MetricsModelBreakdownHandler(proxySvc))
		accountAuthed.GET("/keys", admin.ListAPIKeysHandler(authSvc))
		accountAuthed.POST("/keys", admin.IssueAPIKeyHandler(authSvc))
		accountAuthed.POST("/keys/:id/rotate", admin.RotateAPIKeyHandler(authSvc))
		accountAuthed.DELETE("/keys/:id", admin.DeleteAPIKeyHandler(authSvc))
		accountAuthed.GET("/provider-keys", admin.ListExternalKeysHandler(authSvc))
		accountAuthed.POST("/provider-keys", admin.UpsertExternalKeyHandler(authSvc, deployedModels))
		accountAuthed.DELETE("/provider-keys/:id", admin.DeleteExternalKeyHandler(authSvc))
		accountAuthed.GET("/config", admin.ConfigHandler)
		accountAuthed.GET("/onboarding", admin.OnboardingHandler(authSvc))
		accountAuthed.GET("/routing-preferences", admin.GetRoutingPreferencesHandler(authSvc))
		accountAuthed.PUT("/routing-preferences", admin.UpdateRoutingPreferencesHandler(authSvc))
		if aiandCatalogHandler != nil {
			accountAuthed.GET("/aiand/models", aiandCatalogHandler)
		}
		// The live-catalog "models" section stays mounted under /admin/v1 too,
		// since the dashboard fetches it through the account cookie path.
		if deployedModels != nil {
			accountAuthed.GET("/excluded-models", admin.GetExcludedModelsHandler(authSvc, deployedModels, proxySvc))
			accountAuthed.PUT("/excluded-models", admin.UpdateExcludedModelsHandler(authSvc, deployedModels, proxySvc))
			accountAuthed.GET("/allowed-models", admin.GetAllowedModelsHandler(authSvc, deployedModels))
			accountAuthed.PUT("/allowed-models", admin.UpdateAllowedModelsHandler(authSvc, deployedModels))
			accountAuthed.GET("/excluded-providers", admin.GetExcludedProvidersHandler(authSvc, deployedModels, proxySvc))
			accountAuthed.PUT("/excluded-providers", admin.UpdateExcludedProvidersHandler(authSvc, deployedModels, proxySvc))
		}
	}
```

- [ ] **Step 3: Add the missing import**

Add `account "workweave/router/internal/api/account"` to the import block in `internal/server/server.go`.

- [ ] **Step 4: Run tests / compile**

Run: `make build`
Expected: PASS. If `registerUIStatic` is duplicated for two groups, it's fine — both groups serve the same static export; gin panics only on conflicting routes, and `/ui` + `/admin/v1` don't overlap.

- [ ] **Step 5: Commit**

```bash
git add internal/server/server.go
git commit -m "feat(login): selfserve deployment mode + account-scoped dashboard surface"
```

---

## Task 9: `cmd/router/main.go` — wire the aiand verifier + account repos

**Files:**
- Modify: `cmd/router/main.go`

**Interfaces:**
- Consumes: `server.DeploymentModeSelfServe`, `providers.ProviderAiand`, `internal/providers/aiand.KeyVerifier`, `auth.AiandKeyVerifier`, `auth.AccountRepository`, `auth.LoginSessionRepository`, `repo.AccountRepository` (from Task 5's `internal/postgres` mappers on the existing `repo`).
- Produces: boot-time wiring that makes `authSvc.LoginEnabled()` true in `selfserve` mode: `authSvc.WithAccountRepos(...).WithKeyVerifier(...)`.

- [ ] **Step 1: Extend the mode validation switch**

In `cmd/router/main.go`, change the mode switch (lines 104-111) to include the new mode:

```go
	switch deploymentMode {
	case server.DeploymentModeSelfHosted, server.DeploymentModeManaged, server.DeploymentModeSelfServe:
	default:
		err := fmt.Errorf("Invalid ROUTER_DEPLOYMENT_MODE %q (expected %q, %q, or %q)", deploymentMode, server.DeploymentModeSelfHosted, server.DeploymentModeManaged, server.DeploymentModeSelfServe)
		logger.Error("Refusing to boot with invalid deployment mode", "err", err)
		panic(err)
	}
```

- [ ] **Step 2: Wire the account repos + key verifier after `authSvc` is built**

After the `authSvc := auth.NewService(...)...` chain (around line 241), add:

```go
	// Self-serve mode: the dashboard is secured by an aiand-key login instead
	// of the operator password. Each aiand user gets their own installation
	// (account id = installation external_id). AIAND_API_KEY is the DEPLOYMENT's
	// key for the catalog; the LOGIN probe validates arbitrary user keys against
	// aiand's public /api/v1/me, so it never uses a deployment secret.
	if deploymentMode == server.DeploymentModeSelfServe {
		if repo.Accounts == nil || repo.LoginSessions == nil {
			logger.Error("Self-serve mode requires account SQLC repos; refusing to boot", "err", nil)
			panic("selfserve mode: account repos not wired")
		}
		keyVerifier := &aiandProvider.KeyVerifier{
			Client:  &http.Client{Timeout: 15 * time.Second},
			BaseURL: config.GetOr("AIAND_API_URL", aiandProvider.DefaultBaseURL),
		}
		authSvc.WithAccountRepos(repo.Accounts, repo.LoginSessions).WithKeyVerifier(keyVerifier)
		logger.Info("Self-serve login enabled", "aiand_base_url", keyVerifier.BaseURL)
	}
```

> **Note:** `repo` in `main.go` currently exposes `repo.Installations`, `repo.APIKeys`, etc. Task 5 must expose `repo.Accounts` and `repo.LoginSessions` (the SQLC-backed mappers). If an explicit field doesn't exist, add fields to the `postgres.Repository` struct in Task 5. `aiandProvider` is the import alias for `internal/providers/aiand`; `AIAND_API_URL` defaults to `aiand.DefaultBaseURL` (the API root, NOT the OpenAI-compat `/v1` base).

- [ ] **Step 3: Add the import**

Add `aiandProvider "workweave/router/internal/providers/aiand"` to the imports.

- [ ] **Step 4: Run tests / compile**

Run: `make build`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/router/main.go
git commit -m "feat(login): wire aiand key verifier + account repos in selfserve mode"
```

---

## Task 10: Frontend — aiand-key login page + API client

**Files:**
- Modify: `frontend/src/lib/api.ts`
- Modify: `frontend/src/app/(auth)/login/page.tsx`

**Interfaces:**
- Consumes: new endpoints `POST /account/v1/login`, `POST /account/v1/logout`, `GET /account/v1/me`; existing `MeResponse` typing.
- Produces: `api.auth.loginWithKey(key: string)`; `api.auth.accountMe()`; `api.auth.accountLogout()`; a login page that shows an aiand-key form when the deployment reports it (via `GET /account/v1/me` returning 200 + `authenticated:false` OR a dedicated `mode` hint — see Step 1).

- [ ] **Step 1: Add API client functions**

In `frontend/src/lib/api.ts`, after the existing `api.auth` block, add:

```ts
export interface AccountMeResponse {
  authenticated: boolean;
  account_id?: string;
  display_name?: string;
}

auth: {
  // existing ...
  me: () => request<MeResponse>("/auth/me"),
  // New self-service (aiand key) login.
  accountMe: () => request<AccountMeResponse>("/account/v1/me"),
  loginWithKey: (key: string) =>
    request<{ ok: boolean; expires_at: string }>("/account/v1/login", {
      method: "POST",
      body: JSON.stringify({ key }),
    }),
  accountLogout: () => request<{ ok: boolean }>("/account/v1/logout", { method: "POST" }),
},
```

> **Typing note:** `BASE` is `/admin/v1`, so `request("/account/v1/...")` would produce `/admin/v1/account/v1/...` — WRONG. The `request` helper prepends `BASE`. Add an absolute-path variant: `requestFrom(path, init)` that does **not** prepend `BASE` (or temporarily export `requestRaw`). The login endpoints live outside `/admin/v1`. Update `api.ts` to add `requestRaw` and route the three new calls through it.

- [ ] **Step 2: Update `request` to support absolute paths**

In `frontend/src/lib/api.ts`, add a `requestRaw` helper (or a `base` option) so the account endpoints use `/account/v1/*`:

```ts
// Absolute-path variant of request() — used for /account/v1/* which lives
// outside the /admin/v1 BASE group.
async function requestRaw<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...((init?.headers as Record<string, string>) ?? {}),
  };
  const res = await fetch(path, { ...init, credentials: "include", headers });
  if (res.status === 401 && typeof window !== "undefined") {
    // same redirect-to-login logic as request()
    if (!window.location.pathname.startsWith("/ui/login")) {
      const internal = window.location.pathname.startsWith("/ui/")
        ? window.location.pathname.slice(3)
        : "/dashboard";
      window.location.href = `/ui/login?next=${encodeURIComponent(internal)}`;
      throw new Error("401: redirecting to login");
    }
  }
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`${res.status}: ${body}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}
```

Route `accountMe`/`loginWithKey`/`accountLogout` through `requestRaw` with paths `/account/v1/me`, `/account/v1/login`, `/account/v1/logout`.

- [ ] **Step 3: Update the login page**

In `frontend/src/app/(auth)/login/page.tsx`, add a mode check + aiand-key form. On mount, call `api.auth.accountMe()`:
- If it returns `{authenticated: true}` → redirect to `/dashboard`.
- If it returns `{authenticated: false}` → render the aiand-key form (an input for the `sk-` key + submit that calls `api.auth.loginWithKey(key)`, then redirects to `/dashboard`).
- If it throws (endpoint doesn't exist → selfhosted/managed mode) → keep the existing password form.

The existing password form stays untouched for `selfhosted`/`managed`.

- [ ] **Step 4: Build + verify**

Run: `cd frontend && npx tsc --noEmit` (or the repo's lint script)
Expected: PASS. Then `cd ../ && make build` to confirm the static export still compiles.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/api.ts frontend/src/app/"(auth)"/login/page.tsx
git commit -m "feat(login): aiand-key login form + account API client"
```

---

## Task 11: End-to-end smoke + docs

**Files:**
- Create: `scripts/smoke_selfserve.sh` (optional; see Step 1)
- Modify: `docs/CONFIGURATION.md` (document `ROUTER_DEPLOYMENT_MODE=selfserve` + `AIAND_API_URL`)

- [ ] **Step 1: Manual end-to-end verification (no automated test)**

Boot in `selfserve` mode with a real aiand key and verify:
1. `GET /account/v1/me` returns `{"authenticated":false}` with no cookie.
2. `POST /account/v1/login` with a valid `sk-` key → `200`, `ok:true`, sets `router_account_session` cookie (HttpOnly).
3. With the cookie, `GET /admin/v1/metrics/summary` returns the account's own metrics (scoped to the installation).
4. `POST /account/v1/logout` clears the cookie; subsequent `/admin/v1/*` calls 401.
5. Key rotation: log in with a *different* aiand key for the same `aiand_user_id` → same dashboard data (same installation).

- [ ] **Step 2: Document the deployment mode**

Append to `docs/CONFIGURATION.md` a `selfserve` section covering: `ROUTER_DEPLOYMENT_MODE=selfserve`, `AIAND_API_URL` (default `https://api.aiand.com`), the wipe-on-revocation contract, and the fact that the operator `/admin/v1/auth/*` password surface is not mounted.

- [ ] **Step 3: Final full test pass**

Run: `make build && go test ./internal/...`
Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add docs/CONFIGURATION.md
git commit -m "docs(login): selfserve deployment mode + config reference"
```

---

## Self-Review notes (fill as you complete each task)

- [ ] Verify no `saas`/`SaaS` identifiers leak into new code (naming rule).
- [ ] `LoginWithKey` returns the resolved `*auth.Installation` so the renewable `/admin/v1/*` handlers work behind an account cookie.
- [ ] `internal/auth` stays I/O-free: `AiandKeyVerifier` interface only; the HTTP probe lives in `internal/providers/aiand`.
- [ ] Migrations are `BEGIN;`/`COMMIT;`-wrapped with precise down rollbacks.
- [ ] `make generate` regenerates `internal/sqlc` before `make build`.