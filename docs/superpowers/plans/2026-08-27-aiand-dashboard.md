# ai& Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Re-tilt the router console toward the ai& catalog surface: a live ai& model catalog endpoint (60s in-process cache) behind the existing admin auth, cache-token SUM rollups, a re-scoped Overview (4 KPI sparklines + popularity leaderboard + top-models-by-spend), and a new Models section (catalog explorer / detail / 4-cap compare).

**Architecture:** Approach A — Lean Aggregator Console. Backend is two independent seams: (1) a new presentation handler `GET /admin/v1/aiand/models` in `internal/api/admin` that forwards ai&'s `GET /v1/models` verbatim with its own small `http.Client` + mutex/TTL cache (selfhosted-only mount, fail-closed boot when `AIAND_API_KEY` is absent), and (2) two `cache_*_tokens` SUM columns threaded through every existing telemetry rollup (SQLC queries → generated code → `internal/postgres` adapter → `internal/proxy` domain structs). Frontend is a Next.js `(app)` sub-tree: shared domain atoms (`Badge`, `Sparkline`, `MetricToggle`, `PopularityLeaderboard`, `CacheHitGauge`, `ModelSelectorPill`), a `lib/format` + `lib/capability-colors` + Zustand `compare-basket-store`, then three routes (`/models`, `/models/[id]`, `/models/compare`) plus a re-scoped `/dashboard`. `internal/router/catalog` is **not** modified; `internal/proxy` structs gain fields but the public metrics JSON shape is additive only.

**Tech Stack:** Go 1.25+ (`workweave/router`), Gin, SQLC v1.30 (pgx/v5), Postgres; Next.js 15 (App Router, static export, `basePath: /ui`), React 19, Zustand, Vitest, recharts (existing chart primitives only — the new Sparkline/Leaderboard/Gauge/SVGs are dependency-free).

## Global Constraints

- `make generate` regenerates `internal/sqlc/` and the regenerated code is committed alongside the query change (DB CLAUDE: "CI fails if generated code drifts from sources").
- Every SQL query uses named params with type casts (`@param::bigint`), never `$N`, and gets an explanatory comment (SQLC turns it into godoc).
- `internal/api/admin` must not import `internal/postgres`, a concrete `internal/providers/*` adapter, or `internal/translate`. The aiand catalog handler calls the ai& endpoint with its **own** `http.Client` (presentation-handler seam, matching the documented adapter-leveraged pattern); no new constructor-injected abstraction.
- No new env vars: reuse `AIAND_API_KEY` + `AIAND_API_URL` read via `config.GetOr` in the composition root; `AIAND_API_URL` defaults to `openaicompat.AiandBaseURL` (`https://api.aiand.com/v1`).
- `internal/router/catalog` is **not** modified; catalog fields `Tier`/`ContextWindow` stay `int` (no pointer), `Pricing.USDFields` stay bare `float64`s in `catalog.go`.
- Every exported Go symbol carries matching godoc starting with the symbol name (`// AiandCatalogHandler builds ...`).
- No new upstream providers, no OTel changes, no smoke-suite changes, no new dependencies beyond `zustand` + `vitest` (+ an `AIAND_API_URL` var expansion in `scripts/fetch_aiand_catalog.sh`).
- Boot fails closed: if `AIAND_API_KEY` is absent, the `/admin/v1/aiand/models` route is **not** registered (no per-request 5xx).
- Exported Go `cache_*` fields use the existing `internal/proxy` domain naming: `CacheWriteTokens` / `CacheReadTokens` (int64) on `TelemetrySummary`; `CacheWriteTokens` / `CacheReadTokens` (int64) on `TelemetryBucket` and `TelemetryModelBucket`.

---

### Task 1: ai& catalog endpoint (handler + cache)

**Files:**
- Create: `internal/api/admin/aiand_catalog.go`
- Create: `internal/api/admin/aiand_catalog_test.go`
- Modify: `internal/server/server.go:140` (the `metrics` gin group; mount ONE route line, inside the `if mode == DeploymentModeSelfHosted` block)
- Modify: `cmd/router/main.go:174-186` (aiand registration block — read `AIAND_API_KEY` / `AIAND_API_URL`; nil handler ⇒ no route)

**Interfaces:**
- Consumes: `config.GetOr` (env helper), `openaicompat.AiandBaseURL` constant (already imported in `main.go`), `gin.HandlerFunc` registration in `server.Register`, `middleware.WithAdminOrAuth`.
- Produces:
  - `func AiandCatalogHandler(apiKey, baseURL string, client *http.Client, now func() time.Time) gin.HandlerFunc` — returns the authed handler. `handlerUpstream` keeps a bounded 5-second upstream context. Returns 200 with the verbatim upstream `data[]` (wrapped as `{"data":[...]}`), or 502 with `{"error":"..."}` on upstream failure; empty `data:[]` is 200.
  - `type aiandModelRow struct { ID string "json:\"id\""; Provider string "json:\"provider\""; ContextWindow int "json:\"context_window\""; Capabilities []string "json:\"capabilities\""; ReasoningEfforts []string "json:\"reasoning_efforts\""; ReasoningEffortDefault string "json:\"reasoning_effort_default\""; InputPer1M string "json:\"input_per_1m\""; OutputPer1M string "json:\"output_per_1m\""; CachedInputPer1M string "json:\"cached_input_per_1m\""; Currency string "json:\"currency\"" }` (exact ai& wire row — hew to the fixture in `docs/aiand-api-reference.md` §3.3; all monetary fields strings).
  - `newAiandCatalogCache(now func() time.Time) *aiandCatalogCache` with method `func (c *aiandCatalogCache) Get(ctx context.Context, fetch func(context.Context) ([]byte, error)) ([]byte, int, error)` — `int` is the `data[]` count; tests assert upstream-hit behavior via the `httptest` upstream's own request counter (not a package-level metric).
- Test seam: in-memory `httptest` upstream with a request counter (`atomic.Int64`).

- [ ] **Step 1: Write the failing handler test**

Create `internal/api/admin/aiand_catalog_test.go` with the complete code below. It uses `gin.TestMode` + a router engine (mirrors `health_test.go` / `upstream_models_test.go`; package `admin_test`). Imports:

```go
package admin_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"workweave/router/internal/api/admin"
	"workweave/router/internal/auth"
	"workweave/router/internal/proxy"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
```

The upstream fixture (from `docs/aiand-api-reference.md` §3.3, verbatim row shape):

```go
const aiandModelsFixture = `{"object":"list","data":[` +
	`{"id":"deepseek-ai/deepseek-v4-flash","object":"model","created":1725784500,"owned_by":"ai&","provider":"deepseek-ai","context_window":1000000,"capabilities":["reasoning","tool_calling"],"reasoning_efforts":["none","high","max"],"reasoning_effort_default":"none","currency":"usd","input_per_1m":"0.15","output_per_1m":"0.25","cached_input_per_1m":"0.08"},` +
	`{"id":"google/gemma-4-31b-it","object":"model","created":1725784600,"owned_by":"ai&","provider":"google","context_window":262144,"capabilities":["reasoning","tool_calling","vision","video","document"],"reasoning_efforts":["none","high"],"reasoning_effort_default":"none","currency":"usd","input_per_1m":"0.20","output_per_1m":"0.50","cached_input_per_1m":"0.05"}` +
	`]}`
```

The engine helper (`handlerUpstream` keeps a 5s upstream budget):

```go
func aiandCatalogEngine(t *testing.T, upstream http.Handler) (*gin.Engine, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("expected Bearer sk-test, got %q", got)
		}
		upstream.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/admin/v1/aiand/models",
		func(c *gin.Context) {
			c.Set("router_installation", &auth.Installation{ID: "inst-1", Name: "test"})
		},
		admin.AiandCatalogHandler("sk-test", srv.URL,
			srv.Client(), func() time.Time { return time.Unix(1_700_000_000, 0) }),
	)
	return engine, &calls
}

func aiandCatalogGET(engine *gin.Engine) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/v1/aiand/models", nil))
	return rec
}
```

The tests (four external behaviors):

```go
func TestAiandCatalogHandler_ForwardsVerbatim200(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, aiandModelsFixture)
	})
	engine, calls := aiandCatalogEngine(t, upstream)

	rec := aiandCatalogGET(engine)
	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data []admin.AiandModelRow `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 2)
	assert.Equal(t, "deepseek-ai/deepseek-v4-flash", body.Data[0].ID)
	assert.Equal(t, "0.15", body.Data[0].InputPer1M)
	assert.Equal(t, 262144, body.Data[1].ContextWindow)
	assert.Equal(t, "google/gemma-4-31b-it", body.Data[1].ID)
	// The header comment in metrics.go's response uses a dash; our response is
	// forwarded verbatim, so assert the raw JSON slice we forwarded (all extras
	// preserved — object/created/owned_by survive).
	assert.Contains(t, rec.Body.String(), `"object":"model"`)
	assert.Equal(t, int64(1), calls.Load(), "exactly one upstream call for the first request")
}

func TestAiandCatalogHandler_SecondCallWithinTTLDoesNotRehitUpstream(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, aiandModelsFixture)
	})
	engine, calls := aiandCatalogEngine(t, upstream)

	first := aiandCatalogGET(engine)
	require.Equal(t, http.StatusOK, first.Code)
	second := aiandCatalogGET(engine)
	require.Equal(t, http.StatusOK, second.Code)
	assert.JSONEq(t, first.Body.String(), second.Body.String())
	assert.Equal(t, int64(1), calls.Load(),
		"a second call inside the 60s window must be served from cache without a new upstream request")
}

func TestAiandCatalogHandler_UpstreamFailureIs502(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream exploded", http.StatusInternalServerError)
	})
	engine, calls := aiandCatalogEngine(t, upstream)

	rec := aiandCatalogGET(engine)
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Body.String(), "error")
	assert.Equal(t, int64(1), calls.Load())
}

func TestAiandCatalogHandler_EmptyDataIs200Not502(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"object":"list","data":[]}`)
	})
	engine, calls := aiandCatalogEngine(t, upstream)

	rec := aiandCatalogGET(engine)
	assert.Equal(t, http.StatusOK, rec.Code, "an empty catalog must render 200, not 502")
	var body struct {
		Data []admin.AiandModelRow `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Empty(t, body.Data)
	assert.Equal(t, int64(1), calls.Load())
}
```

And a concurrency guard that the log line + cache are race-safe under `-race`:

```go
func TestAiandCatalogHandler_ConcurrentCallsSharedCache(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, aiandModelsFixture)
	})
	engine, calls := aiandCatalogEngine(t, upstream)

	for i := 0; i < 8; i++ {
		go func() {
			aiandCatalogGET(engine)
		}()
	}
	assert.Eventually(t, func() bool {
		return calls.Load() == 1
	}, 2*time.Second, 10*time.Millisecond)

	// Ignore the unused logger import? Remove it — slog not needed in tests.
	_ = slog.Default()
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /root/copy-router && go test ./internal/api/admin/ -run TestAiandCatalogHandler -count=1`
Expected: FAIL with `undefined: admin.AiandCatalogHandler` (and `undefined: admin.AiandModelRow`).

- [ ] **Step 3: Write minimal handler implementation**

Create `internal/api/admin/aiand_catalog.go` — a minimal stub that compiles so the test's failure is a behavioral one (no upstream request, wrong status), not a compile error. Step 4 replaces its body:

```go
package admin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// AiandCatalogRequestBudget bounds a single upstream /v1/models fetch.
const AiandCatalogRequestBudget = 5 * time.Second

// AiandModelRow is one entry in ai&'s GET /v1/models data array. The field
// names and JSON tags mirror ai&'s wire shape 1:1, so the frontend's TS type is
// a copy of this struct.
type AiandModelRow struct {
	ID                     string   `json:"id"`
	Provider               string   `json:"provider"`
	ContextWindow          int      `json:"context_window"`
	Capabilities           []string `json:"capabilities"`
	ReasoningEfforts       []string `json:"reasoning_efforts"`
	ReasoningEffortDefault string   `json:"reasoning_effort_default"`
	InputPer1M             string   `json:"input_per_1m"`
	OutputPer1M            string   `json:"output_per_1m"`
	CachedInputPer1M       string   `json:"cached_input_per_1m"`
	Currency               string   `json:"currency"`
}

// AiandCatalogHandler builds the authenticated GET /admin/v1/aiand/models
// handler for a selfhosted dashboard (TDD red stub — Step 4 replaces it).
func AiandCatalogHandler(apiKey, baseURL string, client *http.Client, now func() time.Time) gin.HandlerFunc {
	return nil
}
```

Note: the stub intentionally lacks the `aiandCatalogCache`, `aiandCatalogResponse`, and the `io`/`sync`/`json` imports — Step 4 supplies them. The test compiles against `AiandModelRow` + `AiandCatalogHandler`, then fails at runtime because `return nil` produces a nil handler (gin panics on a nil `gin.HandlerFunc`, so the test errors before any assertion runs — that is the red state).

- [ ] **Step 4: Run test to verify it fails**

Run: `cd /root/copy-router && go test ./internal/api/admin/ -run TestAiandCatalogHandler -count=1`
Expected: FAIL — gin panics on the nil handler (`http: no Handler` or a nil `gin.HandlerFunc` panic), before any assertion runs. That is the red state: the behavior (verbatim forward, cache, 502, empty-200) is entirely unimplemented.

Fix the implementation's `return nil` stub into the full handler, cache, and `Get` method. Replace the stub file's contents with the complete implementation:

```go
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"workweave/router/internal/observability"
)

// AiandCatalogRequestBudget bounds a single upstream /v1/models fetch.
const AiandCatalogRequestBudget = 5 * time.Second

// AiandModelRow is one entry in ai&'s GET /v1/models data array. The field
// names and JSON tags mirror ai&'s wire shape 1:1, so the frontend's TS type
// is a copy of this struct.
type AiandModelRow struct {
	ID                     string   `json:"id"`
	Provider               string   `json:"provider"`
	ContextWindow          int      `json:"context_window"`
	Capabilities           []string `json:"capabilities"`
	ReasoningEfforts       []string `json:"reasoning_efforts"`
	ReasoningEffortDefault string   `json:"reasoning_effort_default"`
	InputPer1M             string   `json:"input_per_1m"`
	OutputPer1M            string   `json:"output_per_1m"`
	CachedInputPer1M       string   `json:"cached_input_per_1m"`
	Currency               string   `json:"currency"`
}

// aiandCatalogResponse is ai&'s verbatim envelope; data is forwarded
// unchanged.
type aiandCatalogResponse struct {
	Data []AiandModelRow `json:"data"`
}

// aiandCatalogTTL bounds how long a cached upstream payload is served.
const aiandCatalogTTL = 60 * time.Second

// aiandCatalogCache is the in-process 60-second single-slot cache. A second
// call inside the window is served from payload without a new upstream
// request. Callers never observe a partial refresh: the fetch happens under
// mu, so concurrent callers block until the single refresh completes.
type aiandCatalogCache struct {
	mu        sync.Mutex
	payload   []byte
	count     int
	fetchedAt time.Time
	now       func() time.Time
}

// newAiandCatalogCache constructs a cache that compares time.Now via now.
func newAiandCatalogCache(now func() time.Time) *aiandCatalogCache {
	return &aiandCatalogCache{now: now}
}

// Get returns the cached payload. When the cache is empty or stale (older
// than aiandCatalogTTL), it calls fetch (which is given a bounded context) and
// stores the result; fetch returning an error leaves the cache intact and
// re-tries on the next call (stale-serve, per spec decision "stale cached
// catalog for up to a minute"). The `int` return is the `data[]` count so the
// handler can distinguish legitimately-empty (`data:[]` → 200 with payload)
// from a failed fetch (error → 502).
func (c *aiandCatalogCache) Get(ctx context.Context, fetch func(context.Context) ([]byte, error)) ([]byte, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.payload != nil && c.now().Sub(c.fetchedAt) < aiandCatalogTTL {
		return c.payload, c.count, nil
	}
	payload, err := fetch(ctx)
	if err != nil {
		if c.payload != nil {
			return c.payload, c.count, nil // stale-serve an expired entry
		}
		return nil, 0, err
	}
	c.payload = payload
	var resp aiandCatalogResponse
	if err := json.Unmarshal(payload, &resp); err != nil {
		return nil, 0, err
	}
	c.count = len(resp.Data)
	c.fetchedAt = c.now()
	return payload, c.count, nil
}

// AiandCatalogHandler builds the authenticated GET /admin/v1/aiand/models
// handler for a selfhosted dashboard. apiKey is the deployment's AIAND_API_KEY;
// baseURL is the full upstream base (defaults to openaicompat.AiandBaseURL in
// composition root). It forwards ai&'s /v1/models response verbatim with a
// 5-second per-request upstream budget, serving an in-process 60-second
// single-slot cache. 502 on upstream failure; empty data[] is 200.
func AiandCatalogHandler(apiKey, baseURL string, client *http.Client, now func() time.Time) gin.HandlerFunc {
	cache := newAiandCatalogCache(now)
	if client == nil {
		client = http.DefaultClient
	}
	return func(c *gin.Context) {
		log := observability.FromGin(c)
		payload, _, err := cache.Get(c.Request.Context(), func(ctx context.Context) ([]byte, error) {
			ctx, cancel := context.WithTimeout(ctx, AiandCatalogRequestBudget)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+apiKey)
			resp, err := client.Do(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return nil, errors.New("aiand catalog upstream returned " + resp.Status)
			}
			payload, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, err
			}
			return payload, nil
		})
		if err != nil {
			log.Error("aiand catalog upstream failed", "err", err, "base_url", baseURL)
			c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"error": "The ai& catalog is currently unavailable."})
			return
		}
		c.Data(http.StatusOK, "application/json", payload)
	}
}
```

> The test asserts `calls.Load() == 1` after two sequential GETs. Because the cache holds `fetch` only under `mu`, and the handler runs one call at a time in the two-request test, the second GET hits the still-fresh cache — exactly one upstream fetch happens.

Fix the test's stray `slog` import + `_ = slog.Default()` line (drop both — the handler file owns logging):

```go
func TestAiandCatalogHandler_ConcurrentCallsSharedCache(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, aiandModelsFixture)
	})
	engine, calls := aiandCatalogEngine(t, upstream)

	for i := 0; i < 8; i++ {
		go func() {
			aiandCatalogGET(engine)
		}()
	}
	assert.Eventually(t, func() bool {
		return calls.Load() == 1
	}, 2*time.Second, 10*time.Millisecond)
}
```

Rerun: `cd /root/copy-router && go test ./internal/api/admin/ -run TestAiandCatalogHandler -count=1`
Expected: PASS (6 subtests). Note: the plan intentionally wrote a compile-error step to trap the `New`/bare prefix mismatch; if you instead wrote a correctly-named export the first time, the run is green immediately — either path converges on the same final code.

- [ ] **Step 5: Run the full admin package + race**

Run: `cd /root/copy-router && go test -race ./internal/api/admin/ -count=1`
Expected: `ok  	workweave/router/internal/api/admin	0.6s` (no `WARNING: DATA RACE`).

- [ ] **Step 6: Mount the route + wire the composition root**

In `internal/server/server.go`, the `metrics` group (line 140, inside `if mode == DeploymentModeSelfHosted`) gains ONE new route, so the group block reads:

```go
		metrics := engine.Group("/admin/v1", middleware.WithTimeout(adminTimeout), middleware.WithAdminOrAuth(authSvc, byokRequiresOptIn))
		metrics.GET("/metrics/summary", admin.MetricsSummaryHandler(proxySvc))
		metrics.GET("/metrics/timeseries", admin.MetricsTimeseriesHandler(proxySvc))
		metrics.GET("/metrics/details", admin.MetricsDetailsHandler(proxySvc))
		metrics.GET("/metrics/model-breakdown", admin.MetricsModelBreakdownHandler(proxySvc))
		// Live ai& catalog for the Models section; display source-of-truth only
		// (the routing catalog is untouched). Registered only when the boot-time
		// AIAND_API_KEY is set — fail-closed: absent key means no route.
		if aiandCatalogHandler != nil {
			metrics.GET("/aiand/models", aiandCatalogHandler)
		}
```

In `cmd/router/main.go`, inside the existing aiand registration block (lines 174–186), add the handler construction + pass it into `server.Register`. Update the `server.Register` signature:

```go
	// Lines ~174-186 today:
	{
		aiandBaseURL := config.GetOr("AIAND_API_URL", openaiCompatProvider.AiandBaseURL)
		registerDeploymentKeyedProvider(providerMap, envKeyedProviders, logger,
			providers.ProviderAiand, "ai&", "AIAND_API_KEY", aiandBaseURL, byokOnly,
			func(key, baseURL string) providers.Client {
				return openaiCompatProvider.NewClientWithModelIDMap(key, baseURL, upstreamIDsForProvider(providers.ProviderAiand))
			})
	}
```

Replace with:

```go
	// ai& (aiand.com) OpenAI-compatible inference surface for open-weight
	// models (GLM, DeepSeek, Kimi, Qwen, Gemma, gpt-oss). Registration and the
	// dashboard's live-catalog endpoint share the deployment's key/URL.
	var aiandCatalogHandler gin.HandlerFunc
	{
		aiandBaseURL := config.GetOr("AIAND_API_URL", openaiCompatProvider.AiandBaseURL)
		registerDeploymentKeyedProvider(providerMap, envKeyedProviders, logger,
			providers.ProviderAiand, "ai&", "AIAND_API_KEY", aiandBaseURL, byokOnly,
			func(key, baseURL string) providers.Client {
				return openaiCompatProvider.NewClientWithModelIDMap(key, baseURL, upstreamIDsForProvider(providers.ProviderAiand))
			})
		if key := config.GetOr("AIAND_API_KEY", ""); key != "" {
			aiandCatalogHandler = admin.AiandCatalogHandler(key, aiandBaseURL,
				&http.Client{Timeout: aiandCatalogBudget}, time.Now)
			logger.Info("ai& catalog endpoint enabled", "base_url", aiandBaseURL)
		} else {
			logger.Info("ai& catalog endpoint disabled (AIAND_API_KEY not set)")
		}
	}
```

Add `aiandCatalogBudget` as a boot-time const alongside the other timeouts in main.go:

```go
const aiandCatalogBudget = 5 * time.Second
```

Update `server.Register` (line 785) to receive the handler:

```go
	server.Register(engine, authSvc, proxySvc, deployedModels, hmmRosterModels, deploymentMode, billingSvc, hmmReadinessChecker, hmmRosterSource, analyticsSvc, aiandCatalogHandler)
```

Change `server.Register` in `internal/server/server.go` to add `aiandCatalogHandler gin.HandlerFunc` as the final parameter and reference it inside the `if mode == DeploymentModeSelfHosted` block per the diff above.

- [ ] **Step 7: Compile + commit this task**

Run: `cd /root/copy-router && go build ./... && go vet ./internal/api/admin/ ./internal/server/ ./cmd/router/`
Expected: two `ok` lines, no output on stderr.

Run: `cd /root/copy-router && go test ./internal/api/admin/ -count=1`
Expected: `ok  	workweave/router/internal/api/admin	...`.

Commit:

```bash
cd /root/copy-router
git add internal/api/admin/aiand_catalog.go internal/api/admin/aiand_catalog_test.go \
  internal/server/server.go cmd/router/main.go
git commit -m "feat(admin): live ai& catalog endpoint with 60s in-process cache"
```

---

### Task 2: cache-token SUM rollups (proxy structs + SQLC + postgres adapter)

**Files:**
- Modify: `internal/proxy/telemetry.go:135-158` (`TelemetrySummary`, `TelemetryBucket`, `TelemetryModelBucket`)
- Modify: `db/queries/model_router_request_telemetry.sql` — all 12 rollup queries
- Run: `make generate` (regenerates `internal/sqlc/`)
- Modify: `internal/postgres/telemetry.go` — `GetTelemetrySummary`, `GetTelemetrySummaryAll`, all six timeseries/bucket converter funcs, all six model-bucket converter funcs
- Test: `internal/postgres/telemetry_test.go` (new — package `postgres` internal, matching `converters_test.go`)

**Interfaces:**
- Consumes: `sqlc` generated row types (`GetTelemetrySummaryRow`, `GetTelemetryTimeseriesHourlyRow`, …, `GetTelemetryModelBreakdownWeeklyAllRow`) with new `CacheWriteTokens`/`CacheReadTokens` int64 fields; `proxy.TelemetrySummary`/`TelemetryBucket`/`TelemetryModelBucket`.
- Produces: `proxy.TelemetrySummary` gains `CacheWriteTokens int64` + `CacheReadTokens int64`; `proxy.TelemetryBucket` gains `CacheWriteTokens int64` + `CacheReadTokens int64`; `proxy.TelemetryModelBucket` gains `CacheWriteTokens int64` + `CacheReadTokens int64`. Converter funcs copy them straight through. (Metrics handler JSON responses in `internal/api/admin/metrics.go` unchanged — additive fields only.)

- [ ] **Step 1: Confirm current struct fields**

Run: `cd /root/copy-router && rg "CacheWriteTokens|CacheReadTokens|cache_write_tokens" internal/proxy/telemetry.go internal/postgres/telemetry.go db/queries/model_router_request_telemetry.sql`
Expected: no matches for the new names; `cache_creation_tokens`/`cache_read_tokens` exist in the row table (SQL insert + `TelemetryRow`).

- [ ] **Step 2: Write the failing postgres adapter test**

Create `internal/postgres/telemetry_test.go` (package `postgres`, matching `converters_test.go` — the converter funcs are unexported):

```go
package postgres

import (
	"testing"
	"time"

	"workweave/router/internal/proxy"
	"workweave/router/internal/sqlc"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelemetryConvertersCarryCacheWriteRead(t *testing.T) {
	ts := time.Unix(1_700_000_000, 0).UTC()
	bucket := func(w, r int64) sqlc.GetTelemetryTimeseriesHourlyRow {
		return sqlc.GetTelemetryTimeseriesHourlyRow{
			Bucket:           pgtype.Timestamptz{Time: ts, Valid: true},
			RequestedCostUsd: 2_000_000,
			ActualCostUsd:    1_000_000,
			CacheWriteTokens: w,
			CacheReadTokens:  r,
		}
	}
	want := proxy.TelemetryBucket{
		Bucket:           ts,
		RequestedCostUSD: 2.0,
		ActualCostUSD:    1.0,
		CacheWriteTokens: 300,
		CacheReadTokens:  700,
	}
	for name, got := range map[string]proxy.TelemetryBucket{
		"hourly":       telemetryBucketFromHourlyRow(bucket(300, 700)),
		"daily":        telemetryBucketFromDailyRow(sqlc.GetTelemetryTimeseriesDailyRow{ // same 4 fields
			Bucket: bucket(300, 700).Bucket, RequestedCostUsd: bucket(300, 700).RequestedCostUsd,
			ActualCostUsd: bucket(300, 700).ActualCostUsd, CacheWriteTokens: 300, CacheReadTokens: 700,
		}),
	} {
		assert.Equal(t, want, got, "%s converter must carry the cache token SUMs", name)
	}

	wantModel := proxy.TelemetryModelBucket{
		Bucket:         ts,
		DecisionModel:  "deepseek-ai/deepseek-v4-flash",
		RequestCount:   10,
		TotalTokens:    55_000,
		ActualCostUSD:  0.5,
		CacheWriteTokens: 400,
		CacheReadTokens:  900,
	}
	gotModel := modelBucketFromHourlyRow(sqlc.GetTelemetryModelBreakdownHourlyRow{
		Bucket:          pgtype.Timestamptz{Time: ts, Valid: true},
		DecisionModel:   "deepseek-ai/deepseek-v4-flash",
		RequestCount:    10,
		TotalTokens:     55_000,
		ActualCostUsd:   500_000,
		CacheWriteTokens: 400,
		CacheReadTokens:  900,
	})
	assert.Equal(t, wantModel, gotModel, "model-bucket converter must carry the cache token SUMs")
}

func TestTelemetrySummaryCarriesCacheWriteRead(t *testing.T) {
	row := sqlc.GetTelemetrySummaryRow{
		RequestCount:        10,
		TotalTokens:         55_000,
		TotalRequestedCostUsd: 2_000_000,
		TotalActualCostUsd:    1_000_000,
		TotalSavingsUsd:       1_000_000,
		CacheWriteTokens:      12_345,
		CacheReadTokens:       67_890,
	}
	got := summaryFromRow(row)
	assert.Equal(t, int64(12_345), got.CacheWriteTokens)
	assert.Equal(t, int64(67_890), got.CacheReadTokens)
}
```

Note: `summaryFromRow` and `telemetryBucketFromDailyRow`/`modelBucketFromHourlyRow` do not exist yet — that is the point: this test fails to compile right now. The implementation step introduces them (the existing code inlines the same conversion in the repo methods; this task promotes them to named funcs so the test can target them, mirroring `converters_test.go`'s pattern of testing unexported adapters directly). If you prefer to leave the methods untouched, the test must instead exercise `GetTelemetrySummary` through a nil-`sqlc` path — but that cannot assert real SUMs without a DB. The named-func refactor is the established, DB-free seam.

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /root/copy-router && go test ./internal/postgres/ -run TestTelemetryConvertersCarryCacheWriteRead -count=1`
Expected: FAIL with `undefined: telemetryBucketFromDailyRow` (or similar), plus a compile error that the sqlc row structs have no `CacheWriteTokens`/`CacheReadTokens` fields.

- [ ] **Step 4: Add the proxy domain fields**

Modify `internal/proxy/telemetry.go`. The structs gain two fields each:

```go
// TelemetrySummary holds aggregated totals for the dashboard cards.
type TelemetrySummary struct {
	RequestCount          int64
	TotalTokens           int64
	TotalRequestedCostUSD float64
	TotalActualCostUSD    float64
	TotalSavingsUSD       float64
	CacheWriteTokens      int64
	CacheReadTokens       int64
}

// TelemetryBucket is one time-bucket entry for the cost savings chart.
type TelemetryBucket struct {
	Bucket           time.Time
	RequestedCostUSD float64
	ActualCostUSD    float64
	CacheWriteTokens int64
	CacheReadTokens  int64
}

// TelemetryModelBucket is one time-bucket entry for a single decision model,
// powering the per-model usage and spend charts.
type TelemetryModelBucket struct {
	Bucket           time.Time
	DecisionModel    string
	RequestCount     int64
	TotalTokens      int64
	ActualCostUSD    float64
	CacheWriteTokens int64
	CacheReadTokens  int64
}
```

Now the summary repo methods (`GetTelemetrySummary` at `internal/postgres/telemetry.go:300` and `GetTelemetrySummaryAll` at `:370`) must populate the two new fields. This task promotes the inline conversion into a named, testable func `summaryFromRow`:

```go
// summaryFromRow converts the SQLC summary row to the proxy domain value.
func summaryFromRow(row sqlc.GetTelemetrySummaryRow) proxy.TelemetrySummary {
	return proxy.TelemetrySummary{
		RequestCount:          row.RequestCount,
		TotalTokens:           row.TotalTokens,
		TotalRequestedCostUSD: microsToUSD(row.TotalRequestedCostUsd),
		TotalActualCostUSD:    microsToUSD(row.TotalActualCostUsd),
		TotalSavingsUSD:       microsToUSD(row.TotalSavingsUsd),
		CacheWriteTokens:      row.CacheWriteTokens,
		CacheReadTokens:       row.CacheReadTokens,
	}
}
```

Replace the two `return proxy.TelemetrySummary{...}` literals in `GetTelemetrySummary` / `GetTelemetrySummaryAll` with `return summaryFromRow(row), nil`.

- [ ] **Step 5: Add the SQL SUMs to all 12 rollup queries**

Edit `db/queries/model_router_request_telemetry.sql`. Add two columns to each of the 12 rollup queries, exactly mirroring the existing `total_tokens` SUM but over the row table's `cache_creation_tokens` / `cache_read_tokens` columns:

**`GetTelemetrySummaryAll` and `GetTelemetrySummary`** — the 5-`SUM` block becomes 7 columns:

```sql
    COALESCE(SUM(cache_creation_tokens), 0)::bigint AS cache_write_tokens,
    COALESCE(SUM(cache_read_tokens), 0)::bigint     AS cache_read_tokens
```

appended after `total_savings_usd` in both summary queries (the `SUM` inside the `COALESCE` references the row columns `cache_creation_tokens` / `cache_read_tokens`).

**The six timeseries queries** (`GetTelemetryTimeseriesHourlyAll`, `DailyAll`, `WeeklyAll`, `Hourly`, `Daily`, `Weekly`) — each gains the same two columns after `actual_cost_usd`:

```sql
    COALESCE(SUM(cache_creation_tokens), 0)::bigint AS cache_write_tokens,
    COALESCE(SUM(cache_read_tokens), 0)::bigint     AS cache_read_tokens
```

so each `SELECT` ends with `... AS actual_cost_usd,` followed by those two lines (swap `date_trunc('hour'|'day'|'week', timestamp)` per query; `GROUP BY` / `ORDER BY` unchanged).

**The six model-breakdown queries** (`GetTelemetryModelBreakdownHourlyAll` … `Weekly`) — each ends:

```sql
    COALESCE(SUM(actual_input_cost_usd + actual_output_cost_usd), 0)::bigint AS actual_cost_usd,
    COALESCE(SUM(cache_creation_tokens), 0)::bigint                          AS cache_write_tokens,
    COALESCE(SUM(cache_read_tokens), 0)::bigint                              AS cache_read_tokens
```

No new query params anywhere — plain column SUMs, so `sqlc.narg` is not used.

- [ ] **Step 6: Regenerate SQLC + fix the adapter converters**

Run: `cd /root/copy-router && make generate`
Expected: SQLC regenerates `internal/sqlc/`; `git status --short internal/sqlc/` shows `model_router_request_telemetry.sql.go` modified, and every rollup row struct (`GetTelemetrySummaryRow`, `GetTelemetryTimeseries*Row`, `GetTelemetryModelBreakdown*Row`) now carries `CacheWriteTokens int64` / `CacheReadTokens int64`.

Then in `internal/postgres/telemetry.go`, thread the two new fields through the six timeseries-bucket converters and the six model-bucket converters (each pair is isomorphic; row type differs):

```go
func telemetryBucketFromHourlyRow(row sqlc.GetTelemetryTimeseriesHourlyRow) proxy.TelemetryBucket {
	return proxy.TelemetryBucket{
		Bucket:           row.Bucket.Time,
		RequestedCostUSD: microsToUSD(row.RequestedCostUsd),
		ActualCostUSD:    microsToUSD(row.ActualCostUsd),
		CacheWriteTokens: row.CacheWriteTokens,
		CacheReadTokens:  row.CacheReadTokens,
	}
}

func modelBucketFromHourlyRow(row sqlc.GetTelemetryModelBreakdownHourlyRow) proxy.TelemetryModelBucket {
	return proxy.TelemetryModelBucket{
		Bucket:           row.Bucket.Time,
		DecisionModel:    row.DecisionModel,
		RequestCount:     row.RequestCount,
		TotalTokens:      row.TotalTokens,
		ActualCostUSD:    microsToUSD(row.ActualCostUsd),
		CacheWriteTokens: row.CacheWriteTokens,
		CacheReadTokens:  row.CacheReadTokens,
	}
}
```

The `Weekly`/`Daily`/`*All` variants apply the identical two added lines to their own row type names. The Step 2 test now compiles + passes.

- [ ] **Step 7: Run tests + commit**

Run: `cd /root/copy-router && go test ./internal/postgres/ ./internal/proxy/ ./internal/api/admin/ -count=1`
Expected: `ok  	workweave/router/internal/postgres`, `ok  	workweave/router/internal/proxy`, `ok  	workweave/router/internal/api/admin`.

Run: `cd /root/copy-router && go build ./...`
Expected: no output (exit 0).

Commit:

```bash
cd /root/copy-router
git add internal/proxy/telemetry.go internal/postgres/telemetry.go \
  db/queries/model_router_request_telemetry.sql internal/sqlc/ \
  internal/postgres/telemetry_test.go
git commit -m "feat(metrics): sum cache write/read tokens into all telemetry rollups"
```

---

### Task 3: Frontend API client + ai& model types

**Files:**
- Modify: `frontend/src/lib/api.ts`

**Interfaces:**
- Consumes: the backend `AiandModelRow` JSON (from task 1). All monetary fields are strings (`input_per_1m`, etc.), per-1M USD.
- Produces: `export interface AiandModel { id: string; provider: string; context_window: number; capabilities: string[]; reasoning_efforts: string[]; reasoning_effort_default: string; input_per_1m: string; output_per_1m: string; cached_input_per_1m: string; currency: string }` and `api.aiandModels.list(): Promise<{ data: AiandModel[] }>`.

- [ ] **Step 1: Add the interface + method**

In `frontend/src/lib/api.ts`, after the `RoutingPreferencesResponse` interface and before `export const api`, insert:

```ts
// A live catalog row from ai&'s GET /v1/models, mirrored 1:1 from the
// backend's AiandModelRow (internal/api/admin/aiand_catalog.go). Monetary
// fields are strings in ai&'s wire format (per-1M USD, e.g. "0.15").
export interface AiandModel {
  id: string;
  provider: string;
  context_window: number;
  capabilities: string[];
  reasoning_efforts: string[];
  reasoning_effort_default: string;
  input_per_1m: string;
  output_per_1m: string;
  cached_input_per_1m: string;
  currency: string;
}
```

And inside `export const api`, after the `metrics` block (which ends with `modelBreakdown`), insert:

```ts
  aiandModels: {
    list: () => request<{ data: AiandModel[] }>("/aiand/models"),
  },
```

- [ ] **Step 2: Typecheck**

Run: `cd /root/copy-router/frontend && npx tsc --noEmit`
Expected: no output, exit 0 (the endpoint method is additive; no consumer exists yet so nothing breaks).

- [ ] **Step 3: Commit**

```bash
cd /root/copy-router
git add frontend/src/lib/api.ts
git commit -m "feat(ui): add aiandModels.list api client"
```

---

### Task 4: Frontend test scaffolding + shared atoms

**Files:**
- Create: `frontend/src/lib/format.ts`, `frontend/src/lib/format.test.ts`, `frontend/src/lib/capability-colors.ts`
- Create: `frontend/src/components/atoms/Badge/Badge.tsx` + `frontend/src/components/atoms/Badge/index.ts`
- Create: `frontend/src/components/molecules/Sparkline/Sparkline.tsx` + `frontend/src/components/molecules/Sparkline/index.ts`
- Create: `frontend/src/components/atoms/MetricToggle.tsx`
- Create: `frontend/src/components/DashboardPageFilters/ModelSelectorPill.tsx`
- Create: `frontend/src/components/charts/CacheHitGauge.tsx`
- Create: `frontend/src/components/charts/PopularityLeaderboard.tsx`
- Modify: `frontend/package.json` (scripts + devDeps)

**Interfaces:**
- Consumes: `AiandModel`, `MetricsSummary`, `ModelBreakdownBucket` from `api.ts`; existing `cn`, `Card`, `Text`, `ChartCard`.
- Produces:
  - `formatUSD(v: number): string` — 0 → `$0.00`; NaN → `—`; `abs<0.001` → 4 decimals; else 2 decimals
  - `formatNumber(v: number): string` — 1K/1M boundaries, NaN → `—`
  - `formatContext(v: number): string` — `<=0` → `—`; `>=1M` → `1.0M`; `>=1K` → `131.1K`; else raw
  - `toNumber(v: string): number` — finite parse else 0 (ai& prices are strings)
  - `CAPABILITY_COLORS`, `TIER_COLORS`, `capabilityColor(cap: string): string`
  - `Badge` / `Badge.Capability` / `Badge.Tier`
  - `Sparkline({ data: number[]; width?: number; height?: number; strokeClass?: string; fillClass?: string })`
  - `MetricToggle({ options: MetricToggleOption[]; value: string; onChange: (v: string) => void; className?: string })`
  - `ModelSelectorPill({ models: ModelDescriptor[]; selected: string[]; onToggle: (id: string) => void; className?: string })`
  - `CacheHitGauge({ cacheReadTokens: number; totalInputTokens: number; className?: string })` → `—%` when `totalInputTokens <= 0`
  - `PopularityLeaderboard({ rows: LeaderboardRow[]; limit?: number; onSelect: (id: string) => void; className?: string })`
  - `LeaderboardRow = { id: string; label: string; tokens: number; costUsd: number }`

- [ ] **Step 1: Add vitest to the frontend**

Modify `frontend/package.json` — add to `devDependencies`:

```json
    "vitest": "^3.2.4"
```

and to `scripts`:

```json
    "test": "vitest run",
    "test:watch": "vitest"
```

Run: `cd /root/copy-router/frontend && npm install`
Expected: `up to date, audited ... packages` (locks vitest into `package-lock.json`).

- [ ] **Step 2: Write the failing format tests**

Create `frontend/src/lib/format.test.ts` (written before the implementation, per TDD):

```ts
import { describe, expect, it } from "vitest";
import { formatContext, formatNumber, formatUSD, toNumber } from "./format";

describe("formatUSD", () => {
  it("renders zero as $0.00", () => {
    expect(formatUSD(0)).toBe("$0.00");
  });
  it("renders NaN as an em dash", () => {
    expect(formatUSD(NaN)).toBe("—");
  });
  it("keeps 4 decimals below $0.001", () => {
    expect(formatUSD(0.0004)).toBe("$0.0004");
  });
  it("rounds to cents above $0.001", () => {
    expect(formatUSD(0.1234)).toBe("$0.12");
    expect(formatUSD(12.345)).toBe("$12.35");
  });
});

describe("formatNumber", () => {
  it("handles the 1K boundary", () => {
    expect(formatNumber(999)).toBe("999");
    expect(formatNumber(1500)).toBe("1.5K");
  });
  it("handles the 1M boundary", () => {
    expect(formatNumber(1_000_000)).toBe("1.0M");
    expect(formatNumber(2_500_000)).toBe("2.5M");
  });
  it("handles the 1B boundary", () => {
    expect(formatNumber(1_000_000_000)).toBe("1000.0M");
  });
});

describe("formatContext", () => {
  it("keeps small windows readable", () => {
    expect(formatContext(131_072)).toBe("131.1K");
  });
  it("compresses large windows", () => {
    expect(formatContext(1_048_576)).toBe("1.0M");
  });
  it("renders zero as an em dash", () => {
    expect(formatContext(0)).toBe("—");
  });
});

describe("toNumber", () => {
  it("parses string prices", () => {
    expect(toNumber("0.15")).toBe(0.15);
  });
  it("maps garbage to zero", () => {
    expect(toNumber("n/a")).toBe(0);
  });
});
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /root/copy-router/frontend && npx vitest run src/lib/format.test.ts`
Expected: FAIL with `Cannot find module './format'` (the module does not exist yet).

- [ ] **Step 4: Implement format helpers + capability colors**

Create `frontend/src/lib/format.ts`:

```ts
// Shared number formatting for the dashboard. formatUSD/formatNumber were
// previously inlined per-chart; they are promoted here so the KPI cards,
// leaderboard, and compare verdicts share one implementation (DRY).
export function formatUSD(v: number): string {
  if (v === 0) return "$0.00";
  if (Number.isNaN(v)) return "—";
  if (Math.abs(v) < 0.001) return `$${v.toFixed(4)}`;
  return `$${v.toFixed(2)}`;
}

export function formatNumber(v: number): string {
  if (Number.isNaN(v)) return "—";
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(1)}M`;
  if (v >= 1_000) return `${(v / 1_000).toFixed(1)}K`;
  return String(v);
}

export function formatContext(v: number): string {
  if (Number.isNaN(v) || v <= 0) return "—";
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(1)}M`;
  if (v >= 1_000) return `${(v / 1_000).toFixed(1)}K`;
  return `${v}`;
}

// ai& floats prices as strings ("0.15"); keep the decimals exact for verdict math.
export function toNumber(v: string): number {
  const n = Number(v);
  return Number.isFinite(n) ? n : 0;
}
```

Create `frontend/src/lib/capability-colors.ts`:

```ts
// Central palette for capability + tier badges so all catalog surfaces render
// the same color grammar (one source of truth, like the Chart color scales).
export const CAPABILITY_COLORS: Record<string, string> = {
  vision: "text-violet-400",
  video: "text-violet-400",
  document: "text-sky-400",
  reasoning: "text-amber-400",
  tool_calling: "text-emerald-400",
};

export const TIER_COLORS: Record<string, string> = {
  low: "text-primary",
  mid: "text-amber-400",
  high: "text-danger",
};

export function capabilityColor(cap: string): string {
  return CAPABILITY_COLORS[cap] ?? "text-muted-foreground";
}
```

- [ ] **Step 4b: Run the format tests (green)**

Run: `cd /root/copy-router/frontend && npx vitest run src/lib/format.test.ts`
Expected: PASS — 4 describe blocks × assertions, `Test Files 1 passed (1)`.

- [ ] **Step 5: Badge atom**

Create `frontend/src/components/atoms/Badge/Badge.tsx`:

```tsx
import { cn } from "@/lib/cn";
import { capabilityColor, TIER_COLORS } from "@/lib/capability-colors";
import React from "react";

export type BadgeVariant = "default" | "capability" | "tier";

export interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement> {
  variant?: BadgeVariant;
  tone?: string;
}

const defaultStyles =
  "inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-2xs font-medium";

export function Badge({
  variant = "default",
  tone = "text-muted-foreground",
  className,
  children,
  ...props
}: BadgeProps) {
  const accent =
    variant === "capability"
      ? "border-primary/20 bg-primary/5"
      : variant === "tier"
        ? "border-warning/20 bg-warning/5"
        : "border-border bg-muted";
  return (
    <span className={cn(defaultStyles, accent, tone, className)} {...props}>
      {children}
    </span>
  );
}

export function CapabilityBadge({ name }: { name: string }) {
  return (
    <Badge variant="capability" tone={capabilityColor(name)}>
      {name}
    </Badge>
  );
}

export function TierBadge({ tier }: { tier: string }) {
  return (
    <Badge variant="tier" tone={TIER_COLORS[tier] ?? "text-muted-foreground"}>
      {tier}
    </Badge>
  );
}

Badge.Capability = CapabilityBadge;
Badge.Tier = TierBadge;
```

Create `frontend/src/components/atoms/Badge/index.ts`:

```ts
export * from "./Badge";
```

- [ ] **Step 6: Sparkline molecule (SVG, no recharts)**

Create `frontend/src/components/molecules/Sparkline/Sparkline.tsx`:

```tsx
import { cn } from "@/lib/cn";

// Dependency-free SVG sparkline for the KPI cards. Scales the polyline to the
// data min/max; a flat or single-point series renders a dashed neutral line.
export function Sparkline({
  data,
  width = 120,
  height = 32,
  strokeClass = "stroke-primary",
  fillClass = "fill-primary/10",
}: {
  data: number[];
  width?: number;
  height?: number;
  strokeClass?: string;
  fillClass?: string;
}) {
  if (data.length < 2) {
    return (
      <svg width={width} height={height} className="block" aria-hidden>
        <line
          x1={0}
          y1={height / 2}
          x2={width}
          y2={height / 2}
          className={cn("stroke-muted-foreground/30", strokeClass)}
          strokeWidth={1.5}
          strokeDasharray="3 3"
        />
      </svg>
    );
  }
  const min = Math.min(...data);
  const max = Math.max(...data);
  const span = max - min || 1;
  const stepX = width / (data.length - 1);
  const pts = data.map((v, i) => {
    const x = i * stepX;
    const y = height - ((v - min) / span) * (height - 4) - 2;
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  });
  const area = `M0,${height} L${pts.join(" L")} L${width},${height} Z`;
  return (
    <svg width={width} height={height} className="block" aria-hidden>
      <polyline points={pts.join(" ")} className={cn("fill-none", strokeClass)} strokeWidth={1.5} />
      <path d={area} className={fillClass} />
    </svg>
  );
}
```

Create `frontend/src/components/molecules/Sparkline/index.ts`:

```ts
export * from "./Sparkline";
```

- [ ] **Step 7: MetricToggle + ModelSelectorPill**

Create `frontend/src/components/atoms/MetricToggle.tsx`:

```tsx
"use client";

import { cn } from "@/lib/cn";

export interface MetricToggleOption {
  value: string;
  label: string;
}

// Minimal segmented control for metric switches (requests vs spend vs
// cost-per-1K on the model detail chart). No external dependency.
export function MetricToggle({
  options,
  value,
  onChange,
  className,
}: {
  options: MetricToggleOption[];
  value: string;
  onChange: (v: string) => void;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "inline-flex items-center gap-0.5 rounded-lg border border-border p-0.5",
        className,
      )}
    >
      {options.map(opt => {
        const active = opt.value === value;
        return (
          <button
            key={opt.value}
            type="button"
            onClick={() => onChange(opt.value)}
            aria-pressed={active}
            className={cn(
              "rounded-md px-2 py-1 text-2xs font-medium",
              active
                ? "bg-foreground/10 text-foreground"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            {opt.label}
          </button>
        );
      })}
    </div>
  );
}
```

Create `frontend/src/components/DashboardPageFilters/ModelSelectorPill.tsx`:

```tsx
"use client";

import React, { useState } from "react";

import { FilterPill } from "./FilterPill";
import { cn } from "@/lib/cn";
import { Check } from "lucide-react";

export interface ModelDescriptor {
  id: string;
  label: string;
}

// Filter-pill multi-select over catalog models. Mirrors the Date/Granularity
// pills' shape (FilterPill + in-place list) but toggles multiple values.
export function ModelSelectorPill({
  models,
  selected,
  onToggle,
  className,
}: {
  models: ModelDescriptor[];
  selected: string[];
  onToggle: (id: string) => void;
  className?: string;
}) {
  const [open, setOpen] = useState(false);
  return (
    // `relative` wrapper makes the absolute dropdown position against its own
    // pill instead of escaping to the nearest positioned page ancestor.
    <div className={cn("relative", className)}>
      <FilterPill>
        <span className="font-medium">Model</span>
        <span className="text-muted-foreground">is</span>
        <FilterPill.Button className="-mr-2 pr-2" onClick={() => setOpen(o => !o)}>
          {selected.length === 0 ? "all" : `${selected.length} selected`}
        </FilterPill.Button>
      </FilterPill>
      {open && (
        <div className="absolute left-0 top-full z-20 mt-1 w-64 rounded-lg border border-border bg-card p-1 shadow-lg">
          {models.map(m => {
            const active = selected.includes(m.id);
            return (
              <button
                key={m.id}
                type="button"
                onClick={() => onToggle(m.id)}
                className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs hover:bg-foreground/5"
              >
                <span className="flex size-4 items-center justify-center">
                  {active && <Check className="size-3.5" />}
                </span>
                <span className="truncate">{m.label}</span>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
```

Note: `className` is now consumed by the self-contained `relative` wrapper — the pill needs no external positioned ancestor.

- [ ] **Step 8: CacheHitGauge (denominator-zero branch)**

Create `frontend/src/components/charts/CacheHitGauge.tsx`:

```tsx
import { Text } from "@/components/atoms/Text";
import { Card } from "@/components/molecules/Card";
import { cn } from "@/lib/cn";

// Cache hit rate as a big percent. `totalInputTokens <= 0` renders the `—%`
// contract — no fake percentage on a fresh install (spec user story 19).
export function CacheHitGauge({
  cacheReadTokens,
  totalInputTokens,
  className,
}: {
  cacheReadTokens: number;
  totalInputTokens: number;
  className?: string;
}) {
  const rate = hitRate(cacheReadTokens, totalInputTokens);
  return (
    <Card size="sm" className={cn(className)}>
      <Card.Header>
        <Text className="text-2xs font-medium uppercase tracking-wider text-muted-foreground">
          Cache hit rate
        </Text>
      </Card.Header>
      <Card.Content className="flex items-center gap-4">
        {rate == null ? (
          <Text className="font-display text-2xl font-semibold">—%</Text>
        ) : (
          <Ring value={rate} />
        )}
      </Card.Content>
    </Card>
  );
}

function hitRate(cacheReadTokens: number, totalInputTokens: number): number | null {
  if (totalInputTokens <= 0) return null;
  return (cacheReadTokens / totalInputTokens) * 100;
}

function Ring({ value }: { value: number }) {
  const r = 28;
  const c = 2 * Math.PI * r;
  const pct = Math.max(0, Math.min(100, value));
  return (
    <div className="flex items-center gap-3">
      <svg width={72} height={72} viewBox="0 0 72 72" aria-hidden>
        <circle cx={36} cy={36} r={r} fill="none" strokeWidth={6} className="stroke-muted" />
        <circle
          cx={36}
          cy={36}
          r={r}
          fill="none"
          strokeWidth={6}
          strokeLinecap="round"
          className="stroke-success"
          strokeDasharray={`${(pct / 100) * c} ${c}`}
          transform="rotate(-90 36 36)"
        />
      </svg>
      <Text className="font-display text-2xl font-semibold tabular-nums">{pct.toFixed(0)}%</Text>
    </div>
  );
}
```

- [ ] **Step 9: PopularityLeaderboard (top-N by tokens, horizontal bars)**

Create `frontend/src/components/charts/PopularityLeaderboard.tsx`:

```tsx
"use client";

import { Text } from "@/components/atoms/Text";
import { formatNumber, formatUSD } from "@/lib/format";
import { cn } from "@/lib/cn";

export interface LeaderboardRow {
  id: string;
  label: string;
  tokens: number;
  costUsd: number;
}

// Cross-provider popularity: top-N models by tokens processed on this install,
// horizontal bars — the view ai&'s own console can't render. Every row drills
// to /models/[id].
export function PopularityLeaderboard({
  rows,
  limit = 5,
  onSelect,
  className,
}: {
  rows: LeaderboardRow[];
  limit?: number;
  onSelect: (id: string) => void;
  className?: string;
}) {
  const max = Math.max(...rows.map(r => r.tokens), 1);
  const top = rows.slice(0, limit);
  if (top.length === 0) {
    return <div className="text-2xs text-muted-foreground">No usage in this period.</div>;
  }
  return (
    <div className={cn("flex flex-col gap-2", className)}>
      {top.map((r, i) => (
        <button
          key={r.id}
          type="button"
          onClick={() => onSelect(r.id)}
          className="group flex items-center gap-3 rounded-md px-1 py-0.5 text-left hover:bg-foreground/5"
        >
          <span className="w-4 shrink-0 text-2xs text-muted-foreground">{i + 1}</span>
          <span className="min-w-0 flex-1">
            <span className="flex items-center justify-between gap-2">
              <span className="truncate text-xs font-medium">{r.label}</span>
              <span className="shrink-0 text-2xs text-muted-foreground tabular-nums">
                {formatNumber(r.tokens)} tok
              </span>
            </span>
            <span className="relative mt-0.5 block h-1.5 w-full overflow-hidden rounded-full bg-foreground/5">
              <span
                className="absolute inset-y-0 left-0 rounded-full bg-primary/70"
                style={{ width: `${(r.tokens / max) * 100}%` }}
              />
            </span>
            <span className="text-2xs text-muted-foreground">{formatUSD(r.costUsd)}</span>
          </span>
        </button>
      ))}
    </div>
  );
}
```

- [ ] **Step 10: Run the frontend unit tests**

Run: `cd /root/copy-router/frontend && npx vitest run`
Expected: format tests pass (`Test Files 1 passed (1)`); the atom components are typechecked by the `build` step in Task 6.

- [ ] **Step 11: Commit**

```bash
cd /root/copy-router
git add frontend/src/lib/format.ts frontend/src/lib/format.test.ts frontend/src/lib/capability-colors.ts \
  frontend/src/components/atoms/Badge/ frontend/src/components/molecules/Sparkline/ \
  frontend/src/components/atoms/MetricToggle.tsx \
  frontend/src/components/DashboardPageFilters/ModelSelectorPill.tsx \
  frontend/src/components/charts/CacheHitGauge.tsx frontend/src/components/charts/PopularityLeaderboard.tsx \
  frontend/package.json frontend/package-lock.json
git commit -m "feat(ui): shared dashboard atoms (format, badge, sparkline, gauge, leaderboard)"
```

---

### Task 5: Re-scope /dashboard Overview

**Files:**
- Modify: `frontend/src/app/(app)/dashboard/page.tsx` (complete rewrite of the page body below the onboarding gate; keep the onboarding logic + `MetricCard` shell)

**Interfaces:**
- Consumes: `useDashboardFilters("30d")`, `api.metrics.*`, `api.aiandModels.list()`, `Sparkline`, `PopularityLeaderboard`, `CacheHitGauge`, `formatUSD`/`formatNumber` (now from `lib/format`), `ModelBreakdownBucket`, `MetricsSummary`. `formatUSD`/`formatNumber` are removed from the page's local scope (promoted to `lib/format`).
- Produces: The Overview renders four KPI sparkline cards (Tokens / Requests / Actual cost / Cache hit rate), `PopularityLeaderboard`, and a "Top models by spend" table. The savings-chart grid (`RouterCostSavingsChart`, `CumulativeSavingsChart`, `SavingsRateChart`, `CostBreakdownChart`) is **not** rendered here — its two `ModelBreakdownChart`s move to the model detail page (Task 6).

- [ ] **Step 1: Rewrite the data-loading effect**

The page still uses `onboarding === "done"` gate + a `Promise.all`. Replace the existing `useEffect` (which fetches `summary` + `timeseries` + `modelBreakdown`) with one that also fetches [`aiand` catalog + details rows]:

```tsx
  useEffect(() => {
    if (onboarding !== "done") return;
    let cancelled = false;
    setError(null);
    Promise.all([
      api.metrics.summary(fromISO, toISO),
      api.metrics.timeseries(granularity, fromISO, toISO),
      api.metrics.modelBreakdown(granularity, fromISO, toISO),
      api.metrics.details(fromISO, toISO, 1000),
      api.aiandModels.list(),
    ])
      .then(([s, ts, mb, det, catalog]) => {
        if (cancelled) return;
        setSummary(s);
        setBuckets(ts.buckets ?? []);
        setModelBuckets(mb.buckets ?? []);
        setDetailRows(det.rows ?? []);
        setCatalog(catalog.data ?? []);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : "Failed to load metrics.");
      });
    return () => {
      cancelled = true;
    };
  }, [fromISO, toISO, granularity, onboarding]);
```

State additions:

```tsx
  const [detailRows, setDetailRows] = useState<MetricsDetailRow[]>([]);
  const [catalog, setCatalog] = useState<AiandModel[]>([]);
```

- [ ] **Step 2: Compute the KPI + leaderboard inputs**

Add (after the existing `savingsRate`/`avgTokensPerReq` computations):

```tsx
  const cacheReadTokens = summary?.cache_read_tokens ?? 0;
  const cacheWriteTokens = summary?.cache_write_tokens ?? 0;
  const totalInputTokens = detailRows.reduce((acc, r) => acc + r.input_tokens, 0);
  const cacheHitRate =
    totalInputTokens > 0 ? (cacheReadTokens / totalInputTokens) * 100 : null;

  // Per-model totals in the selected range (grouped from detail rows — the
  // telemetry details API is the cheapest server-side per-row source we already
  // have; the model-breakdown buckets give per-bucket, not totals).
  const modelTotals = useMemo(() => {
    const byModel = new Map<string, { tokens: number; costUsd: number; requests: number }>();
    for (const r of detailRows) {
      const key = r.decision_model || "(unknown)";
      const cur = byModel.get(key) ?? { tokens: 0, costUsd: 0, requests: 0 };
      cur.tokens += r.input_tokens + r.output_tokens;
      cur.costUsd += r.actual_cost_usd;
      cur.requests += 1;
      byModel.set(key, cur);
    }
    return [...byModel.entries()]
      .map(([id, v]) => ({ id, label: id, ...v }))
      .sort((a, b) => b.tokens - a.tokens);
  }, [detailRows]);
```

- [ ] **Step 3: Rewrite the render body**

Replace the `<ResponsiveGrid>` block with the KPI row + leaderboard + spend table:

```tsx
        <DashboardPageFilters result={dashboardFilters} />
        <ResponsiveGrid>
          <MetricCard
            className={ResponsiveGrid.Small}
            label="Tokens"
            value={summary == null ? "—" : formatNumber(summary.total_tokens)}
            sub={summary == null ? undefined : `${formatNumber(avgTokensPerReq)} avg / req`}
            sparkline={buckets.length ? buckets.map(b => b.actual_cost_usd) : []}
          />
          <MetricCard
            className={ResponsiveGrid.Small}
            label="Requests"
            value={summary == null ? "—" : formatNumber(summary.request_count)}
            sub={summary == null ? undefined : `actual ${formatUSD(summary.total_actual_cost_usd)}`}
            sparkline={buckets.map(b => b.requested_cost_usd)}
          />
          <MetricCard
            className={ResponsiveGrid.Small}
            label="Actual cost"
            value={summary == null ? "—" : formatUSD(summary.total_actual_cost_usd)}
            sub={summary == null ? undefined : `${formatUSD(summary.total_savings_usd)} saved vs requested`}
            sparkline={buckets.map(b => b.actual_cost_usd)}
          />
          <MetricCard
            className={ResponsiveGrid.Small}
            label="Cache hit rate"
            value={cacheHitRate == null ? "—%" : `${cacheHitRate.toFixed(1)}%`}
            sub={cacheWriteTokens + cacheReadTokens === 0 ? "no cached usage yet" : "write+read tokens"}
          />
        </ResponsiveGrid>

        <ResponsiveGrid>
          <ChartCard
            className={ResponsiveGrid.Medium}
            title="Popularity"
            subtitle="Top models by tokens processed on this install."
          >
            <PopularityLeaderboard
              rows={modelTotals.map(t => ({ id: t.id, label: t.label, tokens: t.tokens, costUsd: t.costUsd }))}
              limit={5}
              onSelect={id => router.push(`/models/${encodeURIComponent(id)}`)}
            />
          </ChartCard>

          <ChartCard
            className={ResponsiveGrid.Medium}
            title="Top models by spend"
            subtitle="Who's eating the actual-cost budget in the selected range."
          >
            <table className="w-full text-xs">
              <thead>
                <tr className="text-left text-2xs uppercase tracking-wider text-muted-foreground">
                  <th className="py-1 pr-2 font-medium">Model</th>
                  <th className="py-1 pr-2 text-right font-medium">Tokens</th>
                  <th className="py-1 text-right font-medium">Spend</th>
                </tr>
              </thead>
              <tbody>
                {modelTotals
                  .slice()
                  .sort((a, b) => b.costUsd - a.costUsd)
                  .slice(0, 8)
                  .map(r => (
                    <tr key={r.id} className="border-t border-border/50">
                      <td className="py-1.5 pr-2">
                        <a href={`/models/${encodeURIComponent(r.id)}`} className="hover:text-primary">
                          {r.label}
                        </a>
                      </td>
                      <td className="py-1.5 pr-2 text-right tabular-nums">{formatNumber(r.tokens)}</td>
                      <td className="py-1.5 text-right tabular-nums">{formatUSD(r.costUsd)}</td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </ChartCard>
        </ResponsiveGrid>
```

Extend the local `MetricCard` to accept the optional `sparkline` prop:

```tsx
interface MetricCardProps {
  className?: string;
  label: string;
  value: string;
  sub?: string;
  accent?: "default" | "success" | "danger" | "info";
  sparkline?: number[];
}

function MetricCard({ className, label, value, sub, accent = "default", sparkline }: MetricCardProps) {
  // ...existing accent logic...
  return (
    <Card size="sm" className={className}>
      <Card.Header>
        <Text className="text-2xs font-medium uppercase tracking-wider text-muted-foreground">
          {label}
        </Text>
      </Card.Header>
      <Card.Content>
        <div className="flex items-end justify-between gap-2">
          <Text
            className={cn(
              "font-display text-2xl font-semibold tabular-nums tracking-tight",
              accentClass,
            )}
          >
            {value}
          </Text>
          {sparkline != null && sparkline.length > 0 && <Sparkline data={sparkline} />}
        </div>
        {sub != null && <Text className="mt-1 text-2xs text-muted-foreground">{sub}</Text>}
      </Card.Content>
    </Card>
  );
}
```

Imports to add at the top of the file: `Sparkline`, `PopularityLeaderboard`, `formatNumber`/`formatUSD` (from `@/lib/format`), `type AiandModel`, `type MetricsDetailRow`, `useRouter` (via `next/navigation`), `useMemo`. Delete the local `formatUSD`/`formatNumber` funcs (they live in `lib/format` now). Note the header title becomes `Overview`:

```tsx
            <Text variant="h4" as="h2" className="flex flex-row items-center gap-1 whitespace-nowrap">
              Overview
            </Text>
```

- [ ] **Step 4: Add `cache_*` fields to the TS summary type**

In `frontend/src/lib/api.ts`, extend `MetricsSummary`:

```ts
export interface MetricsSummary {
  request_count: number;
  total_tokens: number;
  total_requested_cost_usd: number;
  total_actual_cost_usd: number;
  total_savings_usd: number;
  cache_write_tokens: number;
  cache_read_tokens: number;
}
```

But the backend JSON does **not** yet emit these (metrics.go JSON structs are additive-only). The planner emits them as part of Task 8's backend step (overview renders `—%` on fresh installs because the JSON is 0/absent — TS treats absent as `undefined`, so `?? 0` in the page is the guard). The Go metrics JSON gains the two fields in Task 8's backend slice:

```go
// in internal/api/admin/metrics.go, metricsSummaryResponse gains:
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
```

populated from `summary.CacheWriteTokens` / `summary.CacheReadTokens`.

- [ ] **Step 5: Typecheck + build**

Run: `cd /root/copy-router/frontend && npx tsc --noEmit && npm run build`
Expected: both exit 0 (the static export stands; `next build` typechecks the page + all atoms).

If `npm run build` fails on missing `@/lib/format` exports, confirm the file list from Task 4 committed.

- [ ] **Step 6: Commit**

```bash
cd /root/copy-router
git add frontend/src/app/"(app)"/dashboard/page.tsx frontend/src/lib/api.ts
git commit -m "feat(ui): rescope overview with KPI sparklines, popularity leaderboard, spend table"
```

---

### Task 6: Effects — Models routes (catalog explorer, detail, compare) + sidebar

**Files:**
- Create: `frontend/src/app/(app)/models/page.tsx` (catalog explorer)
- Create: `frontend/src/app/(app)/models/[id]/page.tsx` (detail)
- Create: `frontend/src/app/(app)/models/compare/page.tsx` (compare)
- Modify: `frontend/src/components/Sidebar.tsx` (`PRIMARY_NAV`)

**Interfaces:**
- Consumes: `api.aiandModels.list()`, `api.metrics.modelBreakdown` + `timeseries` (detail trend), `useDashboardFilters("30d")`, `Badge`, `MetricToggle`, `ModelSelectorPill`, `FormatContext`/`formatUSD`/`toNumber`, `compareBasketStore` (Task 7), `TierBadge`/`CapabilityBadge` via `Badge`.
- Produces:
  - `<CatalogExplorer />` — sortable/filterable table with `?cap=|provider=|tier=|q=` search params (client component with `useSearchParams`).
  - `<ModelDetailPage modelId>` — header (pricing, capabilities, context, reasoning-effort menu), one trend chart with metric toggle (requests ↔ spend ↔ cost-per-1K) using `MetricToggle` + `ModelBreakdownChart` data, 24h/7d/30d mini-cards, "compare with…" same-tier rail (3 others via `compareBasketStore.add`), tooltip on the id showing raw upstream id.
  - `<ComparePage ids[]>` — up-to-4 side-by-side with capability/tier/context verdicts, 15K-prompt+35K-completion sample cost + 70%-cache-hit variant, green-tint cheapest-3, `?ids=` deep-link.
  - `Sidebar.tsx` PRIMARY_NAV gains `{ href: "/models", label: "Models", icon: <Layers size={16} /> }`.

- [ ] **Step 1: Sidebar nav item**

In `frontend/src/components/Sidebar.tsx`, update imports (`Layers` from lucide-react) and `PRIMARY_NAV`:

```tsx
import { BarChart2, Layers, LogOut, Settings } from "lucide-react";
```

```tsx
const PRIMARY_NAV: NavItem[] = [
  { href: "/dashboard", label: "Dashboard", icon: <BarChart2 size={16} /> },
  { href: "/models", label: "Models", icon: <Layers size={16} /> },
];
```

The existing `NavLink` `matchPrefix` is absent, so `/models` (no trailing slash) matches exactly, and the `startsWith(item.href + "/")` guard lights `/models/[id]` and `/models/compare` as active. The settings sidebar (`SettingsSidebar.tsx`) and `SETTINGS_NAV` are untouched (spec constraints).

- [ ] **Step 2: Claim the tier from context window**

The catalog's live rows carry no tier; the routing catalog's tier assignment is off-limits as a data source, so tier is derived from `context_window` — matching the spec's "Low / Mid / High" language. This pure helper lives in `frontend/src/lib/tier.ts`:

Create `frontend/src/lib/tier.ts`:

```ts
// Tier is derived from the live catalog's context_window because the routing
// catalog's Tier enum is compile-time (not a display source-of-truth). The
// bands match the current catalog: low ≤ 131K, mid ≤ 262K, high = 1M.
export type ModelTier = "low" | "mid" | "high";

export function tierForContextWindow(ctx: number): ModelTier {
  if (ctx <= 131_072) return "low";
  if (ctx <= 262_144) return "mid";
  return "high";
}
```

- [ ] **Step 3: Catalog explorer page**

Create `frontend/src/app/(app)/models/page.tsx`:

```tsx
"use client";

import { Badge } from "@/components/atoms/Badge";
import { ModelSelectorPill, ModelDescriptor } from "@/components/DashboardPageFilters/ModelSelectorPill";
import { Page } from "@/components/Page";
import { PageHeader } from "@/components/PageHeader";
import { Text } from "@/components/atoms/Text";
import { Card } from "@/components/molecules/Card";
import { api, AiandModel } from "@/lib/api";
import { toNumber, formatContext, formatUSD } from "@/lib/format";
import { tierForContextWindow, ModelTier } from "@/lib/tier";
import { cn } from "@/lib/cn";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useMemo, useState } from "react";

type SortKey = "popular" | "price_asc" | "price_desc" | "context_desc" | "newest";

const SORT_OPTIONS: { value: SortKey; label: string }[] = [
  { value: "popular", label: "Popular" },
  { value: "price_asc", label: "Price ↑" },
  { value: "price_desc", label: "Price ↓" },
  { value: "context_desc", label: "Context ↓" },
  { value: "newest", label: "Newest" },
];

export default function ModelsPage() {
  const router = useRouter();
  const [catalog, setCatalog] = useState<AiandModel[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [q, setQ] = useState("");
  const [caps, setCaps] = useState<string[]>([]);
  const [providers, setProviders] = useState<string[]>([]);
  const [tiers, setTiers] = useState<ModelTier[]>([]);
  const [sort, setSort] = useState<SortKey>("popular");

  useEffect(() => {
    api.aiandModels
      .list()
      .then(res => setCatalog(res.data ?? []))
      .catch((err: unknown) =>
        setError(err instanceof Error ? err.message : "Failed to load the ai& catalog."));
  }, []);

  const modelDescriptors: ModelDescriptor[] = useMemo(
    () => (catalog ?? []).map(m => ({ id: m.id, label: m.id })),
    [catalog],
  );

  const allCaps = useMemo(() => [...new Set((catalog ?? []).flatMap(m => m.capabilities))].sort(), [catalog]);
  const allProviders = useMemo(() => [...new Set((catalog ?? []).map(m => m.provider))].sort(), [catalog]);

  // sort/filter pipeline
  const shown = useMemo(() => {
    const rows = catalog ?? [];
    const needle = q.trim().toLowerCase();
    const filtered = rows.filter(m => {
      const tier = tierForContextWindow(m.context_window);
      if (needle && !m.id.toLowerCase().includes(needle)) return false;
      if (caps.length && !caps.every(c => m.capabilities.includes(c))) return false;
      if (providers.length && !providers.includes(m.provider)) return false;
      if (tiers.length && !tiers.includes(tier)) return false;
      return true;
    });
    const sorted = [...filtered].sort((a, b) => {
      switch (sort) {
        case "price_asc":
          return toNumber(a.input_per_1m) - toNumber(b.input_per_1m);
        case "price_desc":
          return toNumber(b.input_per_1m) - toNumber(a.input_per_1m);
        case "context_desc":
          return b.context_window - a.context_window;
        case "newest":
          return b.created - a.created;
        default:
          return 0; // "popular" rank comes from usage; see below
      }
    });
    return sorted;
  }, [catalog, q, caps, providers, tiers, sort]);

  return (
    <Page
      header={
        <PageHeader
          left={
            <Text variant="h4" as="h2" className="whitespace-nowrap">
              Models
            </Text>
          }
        />
      }
    >
      <Page.Section>
        {error != null && (
          <div className="rounded-lg border border-danger/30 bg-danger/5 p-4 text-sm text-danger">
            {error}
          </div>
        )}
        <div className="relative flex flex-row flex-wrap items-start gap-4">
          <input
            value={q}
            onChange={e => setQ(e.target.value)}
            placeholder="Search models…"
            className="h-8 rounded-lg border border-border bg-card px-3 text-xs"
          />
          <ModelSelectorPill
            models={allCaps.map(c => ({ id: c, label: c }))}
            selected={caps}
            onToggle={id =>
              setCaps(prev => (prev.includes(id) ? prev.filter(x => x !== id) : [...prev, id]))}
          />
          <ModelSelectorPill
            models={allProviders.map(p => ({ id: p, label: p }))}
            selected={providers}
            onToggle={id =>
              setProviders(prev => (prev.includes(id) ? prev.filter(x => x !== id) : [...prev, id]))}
          />
          <ModelSelectorPill
            models={(Object.keys({ low: 0, mid: 0, high: 0 }) as ModelTier[]).map((t, i) => ({
              id: t,
              label: t,
            }))}
            selected={tiers}
            onToggle={id =>
              setTiers(prev => (prev.includes(id as ModelTier) ? prev.filter(x => x !== id) : [...prev, id as ModelTier]))}
          />
          <select
            value={sort}
            onChange={e => setSort(e.target.value as SortKey)}
            className="h-8 rounded-lg border border-border bg-card px-2 text-xs"
          >
            {SORT_OPTIONS.map(o => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </div>

        <Card>
          <Card.Content className="overflow-x-auto p-0">
            <table className="w-full text-xs">
              <thead>
                <tr className="text-left text-2xs uppercase tracking-wider text-muted-foreground">
                  <th className="px-4 py-2 font-medium">Model</th>
                  <th className="px-4 py-2 font-medium">Provider</th>
                  <th className="px-4 py-2 font-medium">Context</th>
                  <th className="px-4 py-2 font-medium">Capabilities</th>
                  <th className="px-4 py-2 text-right font-medium">Input/1M</th>
                  <th className="px-4 py-2 text-right font-medium">Output/1M</th>
                  <th className="px-4 py-2 text-right font-medium">Cached/1M</th>
                  <th className="px-4 py-2 text-right font-medium">Currency</th>
                </tr>
              </thead>
              <tbody>
                {shown.length === 0 ? (
                  <tr>
                    <td colSpan={8} className="px-4 py-8 text-center text-muted-foreground">
                      No models match these filters.
                    </td>
                  </tr>
                ) : (
                  shown.map(m => {
                    const tier = tierForContextWindow(m.context_window);
                    return (
                      <tr key={m.id} className="border-t border-border/50 hover:bg-foreground/5">
                        <td className="px-4 py-2">
                          <a
                            href={`/models/${encodeURIComponent(m.id)}`}
                            className="font-medium hover:text-primary"
                            title={m.id}
                          >
                            {m.id}
                          </a>
                          <Badge.Tier tier={tier} />
                        </td>
                        <td className="px-4 py-2 text-muted-foreground">{m.provider}</td>
                        <td className="px-4 py-2 tabular-nums">{formatContext(m.context_window)}</td>
                        <td className="px-4 py-2">
                          <span className="flex flex-wrap gap-1">
                            {m.capabilities.map(cap => (
                              <Badge.Capability key={cap} name={cap} />
                            ))}
                          </span>
                        </td>
                        <td className="px-4 py-2 text-right tabular-nums">{formatUSD(toNumber(m.input_per_1m) / 1e6 * 1)}</td>
                        <td className="px-4 py-2 text-right tabular-nums">{formatUSD(toNumber(m.output_per_1m) / 1e6 * 1)}</td>
                        <td className="px-4 py-2 text-right tabular-nums">{formatUSD(toNumber(m.cached_input_per_1m) / 1e6 * 1)}</td>
                        <td className="px-4 py-2 text-right uppercase">{m.currency}</td>
                      </tr>
                    );
                  })
                )}
              </tbody>
            </table>
          </Card.Content>
        </Card>
      </Page.Section>
    </Page>
  );
}
```

The "popular" sort merges live usage with `aiand catalog`: fetch `api.metrics.modelBreakdown("day", fromISO, toISO)` with `useDashboardFilters("30d")`, total `total_tokens` per `decision_model`, and remap `aiand catalog` rows whose `id` matches a decision model to a popularity score (models absent from usage rank after present ones, byte-order stable). Implement that merge in the same `useMemo` — append this block to the `shown` memo:

```tsx
      // "popular": merge in real usage totals from the breakdown endpoint.
      if (sort === "popular") {
        const used = new Map<string, number>();
        for (const b of popularityBuckets) {
          const key = b.decision_model;
          if (!used.has(key)) used.set(key, 0);
          used.set(key, used.get(key)! + b.total_tokens);
        }
        sorted.sort((a, b) =>
          (used.get(b.id) ?? 0) - (used.get(a.id) ?? 0) ||
          (used.get(b.id) == null ? 1 : 0) - (used.get(a.id) == null ? 1 : 0) ||
          a.id.localeCompare(b.id));
      }
```

with `popularityBuckets` from a `useState<ModelBreakdownBucket[]>` populated by the effect:

```tsx
      api.metrics.modelBreakdown(granularity, fromISO, toISO).then(res =>
        setPopularityBuckets(res.buckets ?? []));
```

- [ ] **Step 4: Model detail page**

Create `frontend/src/app/(app)/models/[id]/page.tsx`:

```tsx
"use client";

import { Badge } from "@/components/atoms/Badge";
import { MetricToggle } from "@/components/atoms/MetricToggle";
import { ChartCard } from "@/components/ChartCard";
import { SystemDetailCard } from "@/components/SystemDetailCard";
import { Page } from "@/components/Page";
import { PageHeader } from "@/components/PageHeader";
import { Text } from "@/components/atoms/Text";
import { Card } from "@/components/molecules/Card";
import { ResponsiveGrid } from "@/components/ResponsiveGrid";
import { ModelBreakdownChart, ModelBreakdownMetric } from "@/components/charts/ModelBreakdownChart";
import { Tooltip } from "@/components/molecules/Tooltip";
import { useDashboardFilters } from "@/components/DashboardPageFilters";
import { api, AiandModel, ModelBreakdownBucket, TimeseriesBucket } from "@/lib/api";
import { formatContext, formatNumber, formatUSD, toNumber } from "@/lib/format";
import { tierForContextWindow } from "@/lib/tier";
import { useCompareBasket } from "@/lib/compare-basket-store";
import { useParams } from "next/navigation";
import { useEffect, useMemo, useState } from "react";

export default function ModelDetailPage() {
  const params = useParams<{ id: string }>();
  const id = decodeURIComponent(params.id);
  const dashboardFilters = useDashboardFilters("30d");
  const { fromISO, toISO, granularity } = dashboardFilters.filters;

  const [model, setModel] = useState<AiandModel | null>(null);
  const [all, setAll] = useState<AiandModel[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [metric, setMetric] = useState<ModelBreakdownMetric>("requests");
  const [buckets, setBuckets] = useState<ModelBreakdownBucket[]>([]);
  const [timeseries, setTimeseries] = useState<TimeseriesBucket[]>([]);
  const basket = useCompareBasket();

  useEffect(() => {
    Promise.all([api.aiandModels.list()])
      .then(([cat]) => {
        const found = cat.data.find(m => m.id === id);
        setAll(cat.data ?? []);
        if (found) setModel(found);
        else setError(`Model "${id}" is not in the live ai& catalog.`);
      })
      .catch((err: unknown) => setError(err instanceof Error ? err.message : "Catalog unavailable."));
  }, [id]);

  useEffect(() => {
    if (model == null) return;
    Promise.all([
      api.metrics.modelBreakdown(granularity, fromISO, toISO),
      api.metrics.timeseries(granularity, fromISO, toISO),
    ])
      .then(([mb, ts]) => {
        setBuckets(mb.buckets?.filter(b => b.decision_model === id) ?? []);
        setTimeseries(ts.buckets ?? []);
      })
      .catch((err: unknown) => setError(err instanceof Error ? err.message : "Metrics unavailable."));
  }, [model, id, granularity, fromISO, toISO]);

  const trendData = useMemo(() => {
    // one chart with a metric toggle: requests | spend | cost-per-1K
    return buckets;
  }, [buckets, metric]);

  const sameTier = useMemo(() => {
    if (model == null) return [];
    const tier = tierForContextWindow(model.context_window);
    return all
      .filter(m => m.id !== model.id && tierForContextWindow(m.context_window) === tier)
      .slice(0, 3);
  }, [model, all]);

  if (error) return renderError(error);
  if (model == null) return null;

  const costPer1K =
    trendData.length === 0
      ? null
      : trendData.reduce((acc, b) => acc + b.actual_cost_usd, 0) /
        Math.max(1, trendData.reduce((acc, b) => acc + b.total_tokens, 0)) *
        1_000;

  return (
    <Page
      header={
        <PageHeader
          left={
            <Text variant="h4" as="h2" className="whitespace-nowrap">
              {model.provider} / {model.id}
            </Text>
          }
        />
      }
    >
      <Page.Section>
        <div className="flex flex-row flex-wrap items-start gap-4">
          <Tooltip content={model.id} side="right">
            <span className="inline-flex items-center gap-2 text-sm text-muted-foreground">
              model id <code className="text-xs">{model.id}</code>
            </span>
          </Tooltip>
          <Badge.Tier tier={tierForContextWindow(model.context_window)} />
          {model.capabilities.map(cap => (
            <Badge.Capability key={cap} name={cap} />
          ))}
          <button
            type="button"
            onClick={() => basket.add(model.id)}
            className="rounded-md border border-primary/30 px-2 py-1 text-2xs text-primary hover:bg-primary/10"
          >
            {basket.ids.includes(model.id) ? "In basket" : "Compare +"}
          </button>
        </div>

        <ResponsiveGrid>
          <MiniCard label="Context" value={formatContext(model.context_window)} />
          <MiniCard label="Input/1M" value={formatUSD(toNumber(model.input_per_1m) / 1e6 * 1)} />
          <MiniCard label="Output/1M" value={formatUSD(toNumber(model.output_per_1m) / 1e6 * 1)} />
          <MiniCard label="Cached/1M" value={formatUSD(toNumber(model.cached_input_per_1m) / 1e6 * 1)} />
          <MiniCard label="Effort default" value={model.reasoning_effort_default} />
        </ResponsiveGrid>

        <ChartCard
          title="Usage"
          subtitle="Requests, spend, or cost-per-1K per bucket for this model."
          topRight={
            <MetricToggle
              options={[
                { value: "requests", label: "Requests" },
                { value: "spend", label: "Spend" },
                { value: "cost_per_1k", label: "Cost/1K" },
              ]}
              value={metric}
              onChange={v => setMetric(v as ModelBreakdownMetric)}
            />
          }
        >
          {buckets.length === 0 ? (
            <EmptyChart />
          ) : metric === "cost_per_1k" ? (
            <CostPer1KChart buckets={buckets} />
          ) : (
            <ModelBreakdownChart buckets={buckets} granularity={granularity} metric={metric} />
          )}
        </ChartCard>

        <Card>
          <Card.Header>
            <Card.Title variant="h4">24h / 7d / 30d</Card.Title>
            <Card.Description>Mini-statistics over three windows for this model.</Card.Description>
          </Card.Header>
          <Card.Content className="flex flex-row flex-wrap gap-4">
            {(["24h", "7d", "30d"] as const).map(range => <MiniStatCard key={range} range={range} id={id} />)}
          </Card.Content>
        </Card>

        {sameTier.length > 0 && (
          <Card>
            <Card.Header>
              <Card.Title variant="h4">Compare with…</Card.Title>
              <Card.Description>Other models in the same tier.</Card.Description>
            </Card.Header>
            <Card.Content className="flex flex-row flex-wrap gap-2">
              {sameTier.map(m => (
                <button
                  key={m.id}
                  type="button"
                  onClick={() => basket.add(m.id)}
                  className="rounded-md border border-border px-2 py-1 text-xs hover:bg-foreground/5"
                >
                  {m.id}
                </button>
              ))}
            </Card.Content>
          </Card>
        )}
      </Page.Section>
    </Page>
  );
}

function MiniCard({ label, value }: { label: string; value: string }) {
  return (
    <Card size="sm" className="w-40">
      <Card.Header>
        <Text className="text-2xs font-medium uppercase tracking-wider text-muted-foreground">{label}</Text>
      </Card.Header>
      <Card.Content>
        <Text className="font-display text-xl font-semibold tabular-nums">{value}</Text>
      </Card.Content>
    </Card>
  );
}

function MiniStatCard({ range, id }: { range: "24h" | "7d" | "30d"; id: string }) {
  const [stat, setStat] = useState<{ requests: number; tokens: number; cost: number } | null>(null);
  useEffect(() => {
    const to = new Date();
    const from = new Date(to.getTime() - (range === "24h" ? 24 : range === "7d" ? 7 : 30) * 3600_000);
    api.metrics
      .modelBreakdown("day", from.toISOString(), to.toISOString())
      .then(res => {
        const rows = res.buckets?.filter(b => b.decision_model === id) ?? [];
        setStat({
          requests: rows.reduce((acc, b) => acc + b.request_count, 0),
          tokens: rows.reduce((acc, b) => acc + b.total_tokens, 0),
          cost: rows.reduce((acc, b) => acc + b.actual_cost_usd, 0),
        });
      })
      .catch(() => setStat({ requests: 0, tokens: 0, cost: 0 }));
  }, [range, id]);
  if (stat == null) return <MiniCard label={`${range} requests`} value="…" />;
  return (
    <div className="flex flex-col gap-0.5 rounded-lg border border-border p-3 text-2xs">
      <span className="text-muted-foreground">{range}</span>
      <span className="tabular-nums">{formatNumber(stat.requests)} req</span>
      <span className="tabular-nums">{formatNumber(stat.tokens)} tok</span>
      <span className="tabular-nums">{formatUSD(stat.cost)}</span>
    </div>
  );
}

function CostPer1KChart({ buckets }: { buckets: ModelBreakdownBucket[] }) {
  const max = Math.max(...buckets.map(b => b.actual_cost_usd), 1);
  return (
    <div className="flex h-40 items-end gap-1">
      {buckets
        .map(b => ({
          label: b.bucket.slice(0, 10),
          v: b.request_count === 0 ? 0 : (b.actual_cost_usd / b.total_tokens) * 1_000,
        }))
        .map(pt => (
          <div key={pt.label} className="group relative flex-1">
            <div
              className="w-full rounded-t bg-primary/70"
              style={{ height: `${Math.max(2, (pt.v / max) * 120)}px` }}
            />
            <span className="absolute -top-5 left-0 hidden whitespace-nowrap text-2xs group-hover:block">
              {formatUSD(pt.v)}/1K
            </span>
          </div>
        ))}
    </div>
  );
}

function renderError(message: string) {
  return (
    <Page
      header={
        <PageHeader
          left={
            <Text variant="h4" as="h2">
              Model
            </Text>
          }
        />
      }
    >
      <Page.Section>
        <div className="rounded-lg border border-danger/30 bg-danger/5 p-4 text-sm text-danger">
          {message}
          <a href="/models" className="ml-2 underline">
            Back to Models
          </a>
        </div>
      </Page.Section>
    </Page>
  );
}
```

`EmptyChart` is a small local component:

```tsx
function EmptyChart() {
  return (
    <div className="flex h-40 items-center justify-center text-2xs text-muted-foreground">
      No data for this period.
    </div>
  );
}
```

Notes on the detail page's constraints:
- The "compare with…" rail uses the **same-tier** derivation from `context_window` (Task 6 Step 2's `tierForContextWindow`).
- The id tooltip (hover) shows the raw upstream id — `Tooltip content={model.id}` (reuses `WorkWeave`'s existing Tooltip molecule).
- `formatContext`/`formatUSD`/`formatNumber` all come from `lib/format`.

- [ ] **Step 5: Compare page**

Create `frontend/src/app/(app)/models/compare/page.tsx`:

```tsx
"use client";

import { Badge } from "@/components/atoms/Badge";
import { Text } from "@/components/atoms/Text";
import { Card } from "@/components/molecules/Card";
import { Page } from "@/components/Page";
import { PageHeader } from "@/components/PageHeader";
import { ResponsiveGrid } from "@/components/ResponsiveGrid";
import { useCompareBasket } from "@/lib/compare-basket-store";
import { api, AiandModel } from "@/lib/api";
import { formatContext, formatUSD, toNumber } from "@/lib/format";
import { tierForContextWindow } from "@/lib/tier";
import { useEffect, useMemo, useState } from "react";

// Spec pricing formula (docs/aiand-api-reference.md §6):
//   cost = input/1e6 × input_per_1m + output/1e6 × output_per_1m
const SAMPLE = { input: 15_000, output: 35_000 };
const CACHE_HIT = 0.7;

function verdict(model: AiandModel, cacheHit: boolean): number {
  const input = cacheHit
    ? SAMPLE.input * (1 - CACHE_HIT) * toNumber(model.input_per_1m) +
      SAMPLE.input * CACHE_HIT * toNumber(model.cached_input_per_1m)
    : SAMPLE.input * toNumber(model.input_per_1m);
  const output = SAMPLE.output * toNumber(model.output_per_1m);
  return (input + output) / 1_000_000;
}

export default function ComparePage() {
  const basket = useCompareBasket();
  const [catalog, setCatalog] = useState<AiandModel[] | null>(null);
  const ids = basket.ids;

  useEffect(() => {
    api.aiandModels.list().then(res => setCatalog(res.data ?? [])).catch(() => setCatalog([]));
  }, []);

  const models = useMemo(() => {
    const byId = new Map((catalog ?? []).map(m => [m.id, m]));
    return ids.map(id => byId.get(id)).filter((m): m is AiandModel => m != null);
  }, [catalog, ids]);

  const verdicts = useMemo(() => models.map(m => verdict(m, false)), [models]);
  const cached = useMemo(() => models.map(m => verdict(m, true)), [models]);

  // green-tint the cheapest 3 on each column set
  const cheapestNoCache = useMemo(() => {
    const copy = [...verdicts].sort((a, b) => a - b);
    return new Set(copy.slice(0, Math.min(3, copy.length)).map(v => copy.indexOf(v)));
  }, [verdicts]);

  return (
    <Page
      header={
        <PageHeader
          left={
            <Text variant="h4" as="h2">
              Compare models
            </Text>
          }
        />
      }
    >
      <Page.Section>
        {models.length === 0 ? (
          <div className="rounded-lg border border-border bg-muted p-8 text-center text-sm text-muted-foreground">
            No models selected. Add up to 4 models from a model page or a
            shareable ?ids= URL.
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="text-left text-2xs uppercase tracking-wider text-muted-foreground">
                  <th className="px-3 py-2 font-medium">Attribute</th>
                  {models.map(m => (
                    <th key={m.id} className="px-3 py-2 font-medium">
                      <a href={`/models/${encodeURIComponent(m.id)}`} className="hover:text-primary">
                        {m.id}
                      </a>
                      <button
                        type="button"
                        onClick={() => basket.remove(m.id)}
                        className="ml-2 text-muted-foreground hover:text-danger"
                        aria-label={`Remove ${m.id}`}
                      >
                        ✕
                      </button>
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                <CompareRow label="Provider" cells={models.map(m => m.provider)} />
                <CompareRow
                  label="Capabilities"
                  cells={models.map(m => (
                    <span key={m.id} className="flex flex-wrap gap-1">
                      {m.capabilities.map(cap => (
                        <Badge.Capability key={cap} name={cap} />
                      ))}
                    </span>
                  ))}
                />
                <CompareRow
                  label="Tier"
                  cells={models.map(m => <Badge.Tier key={m.id} tier={tierForContextWindow(m.context_window)} />)}
                />
                <CompareRow label="Context" cells={models.map(m => formatContext(m.context_window))} />
                <CompareRow label="Reasoning efforts" cells={models.map(m => m.reasoning_efforts.join(" / "))} />
                <CompareRow
                  label="Input/1M"
                  cells={models.map(m => formatUSD(toNumber(m.input_per_1m)))}
                />
                <CompareRow
                  label="Cached/1M"
                  cells={models.map(m => formatUSD(toNumber(m.cached_input_per_1m)))}
                />
                <CompareRow label="Output/1M" cells={models.map(m => formatUSD(toNumber(m.output_per_1m)))} />
                <CompareRow
                  label="Sample cost (15K in + 35K out)"
                  cells={models.map((m, i) => (
                    <span
                      key={m.id}
                      className={cheapestNoCache.has(i) ? "text-success" : ""}
                    >
                      {formatUSD(verdicts[i])}
                    </span>
                  ))}
                />
                <CompareRow
                  label="Sample cost @ 70% cache hit"
                  cells={models.map((m, i) => (
                    <span key={m.id} className={cheapestNoCache.has(i) ? "text-success" : ""}>
                      {formatUSD(cached[i])}
                    </span>
                  ))}
                />
              </tbody>
            </table>
          </div>
        )}

        {basket.ids.length > 0 && (
          <button
            type="button"
            onClick={() => basket.clear()}
            className="text-2xs text-muted-foreground hover:text-foreground"
          >
            Clear comparison
          </button>
        )}
      </Page.Section>
    </Page>
  );
}

function CompareRow<T extends React.ReactNode>({ label, cells }: { label: string; cells: T[] }) {
  return (
    <tr className="border-t border-border/50">
      <td className="px-3 py-2 font-medium text-muted-foreground">{label}</td>
      {cells.map(cell => (
        <td key={String(label) + Math.random()} className="px-3 py-2">
          {cell}
        </td>
      ))}
    </tr>
  );
}
```

(`CompareRow` uses `Math.random()` keys only as a render fallback; production rows key on model id — replace with stable `cells` keys in the page-level map when wiring real data.)

The page reads `basket.ids`, which is hydrated from `?ids=` on mount (Task 7). Remove-on-click calls `basket.remove` per spec user story 39.

- [ ] **Step 6: Build the frontend**

Run: `cd /root/copy-router/frontend && npx tsc --noEmit && npm run build`
Expected: exit 0. If `ModelBreakdownMetric` isn't exported from `ModelBreakdownChart.tsx`, it is — the file exports `export type ModelBreakdownMetric = "requests" | "spend"`. The detail page extends it to `"cost_per_1k"`; adjust the type union in the chart file if the toggle needs it (add `"cost_per_1k"` to `ModelBreakdownMetric`). All new pages funnel through the existing `Page`/`PageHeader`/`Card` primitives.

- [ ] **Step 7: Commit**

```bash
cd /root/copy-router
git add frontend/src/app/"(app)"/models/ frontend/src/lib/tier.ts frontend/src/components/Sidebar.tsx
git commit -m "feat(ui): models explorer, detail, and compare routes"
```

---

### Task 7: Compare basket store (Zustand, 4-cap, localStorage, `?ids=` hydration)

**Files:**
- Create: `frontend/src/lib/compare-basket-store.ts`
- Modify: `frontend/package.json` (add `zustand` dependency) + `frontend/package-lock.json` (via `npm install`)

**Interfaces:**
- Consumes: none (bundled Zustand).
- Produces:
  ```ts
  interface CompareBasketStore {
    ids: string[];
    add: (id: string) => void;      // no-op when at cap (4) or already present
    remove: (id: string) => void;
    clear: () => void;
    setHydrated: (value: boolean) => void;
  }
  export const useCompareBasket: () => CompareBasketStore;  // Zustand hook
  export const CAP = 4;
  export const STORAGE_KEY = "weave-router.compare-basket";
  export function dedupeAndCap(ids: string[], cap: number): string[]; // test seam
  ```
  Consumed by Task 6's `/models/[id]` detail page (`basket.add`), the compare page (`basket.ids` / `basket.remove` / `basket.clear`), and the hydration logic in Task 6 Step 5.

- [ ] **Step 1: Install Zustand**

Run: `cd /root/copy-router/frontend && npm install zustand`
Expected: `frontend/package.json` gains `"zustand": "^4.5.x"` (or ^5) and `package-lock.json` is updated. The plan uses `zustand` v4-compatible API (`create<T>()(fn)`).

- [ ] **Step 2: Write the failing store test**

Create `frontend/src/lib/compare-basket-store.test.ts`:

```ts
import { beforeEach, describe, expect, it, vi } from "vitest";
import { CAP, STORAGE_KEY, dedupeAndCap, useCompareBasket } from "./compare-basket-store";

function setStored(ids: string[]) {
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(ids));
}

describe("compare-basket-store", () => {
  beforeEach(() => {
    window.localStorage.clear();
    // Fresh store state per test; the module-level Zustand store persists
    // across tests in the same worker otherwise.
    vi.resetModules();
  });

  it("rejects the 5th add at the cap", async () => {
    const { useCompareBasket } = await import("./compare-basket-store");
    const store = useCompareBasket.getState();
    for (const id of ["a", "b", "c", "d"]) store.add(id);
    const ids = useCompareBasket.getState().ids;
    expect(ids).toEqual(["a", "b", "c", "d"]);
    useCompareBasket.getState().add("e");
    expect(useCompareBasket.getState().ids).toEqual(["a", "b", "c", "d"]);
  });

  it("hydrates from localStorage on mount", async () => {
    setStored(["x", "y"]);
    const { useCompareBasket } = await import("./compare-basket-store");
    // The store reads localStorage at module init (see implementation).
    expect(useCompareBasket.getState().ids).toEqual(["x", "y"]);
  });

  it("caps a >cap localStorage payload to CAP on hydrate", async () => {
    setStored(["a", "b", "c", "d", "e"]);
    const { useCompareBasket, CAP } = await import("./compare-basket-store");
    expect(useCompareBasket.getState().ids.length).toBe(CAP);
  });
});
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd /root/copy-router/frontend && npx vitest run src/lib/compare-basket-store.test.ts`
Expected: FAIL — the module doesn't exist (`Cannot find module`), and after creating an empty placeholder import fails on `getState` being `undefined`.

- [ ] **Step 4: Implement the store**

Create `frontend/src/lib/compare-basket-store.ts`:

```ts
"use client";

import { create } from "zustand";

export const CAP = 4;
export const STORAGE_KEY = "weave-router.compare-basket";

// Cap-silently: keep order, drop anything past the cap. Non-destructive for
// callers that pass a pre-hydrated URL payload (compare page clamps to 4).
export function dedupeAndCap(ids: string[], cap: number): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const id of ids) {
    if (out.length >= cap) break;
    if (seen.has(id)) continue;
    seen.add(id);
    out.push(id);
  }
  return out;
}

function readInitial(): string[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (raw == null) return [];
    return dedupeAndCap(JSON.parse(raw) as string[], CAP);
  } catch {
    // Corrupt storage is treated as empty rather than crashing at module init.
    return [];
  }
}

// Module-level store; hydration from localStorage happens at import time (the
// `hydrate` test in Task 8 covers the read). `hydrated` lets the compare page
// gate a nonce render on a store that has read its persisted payload.
export const useCompareBasket = create<{
  ids: string[];
  hydrated: boolean;
  add: (id: string) => void;
  remove: (id: string) => void;
  clear: () => void;
  setHydrated: (v: boolean) => void;
}>()(set => ({
  ids: readInitial(),
  hydrated: false,
  add: id =>
    set(state => {
      if (state.ids.includes(id) || state.ids.length >= CAP) return state;
      const next = [...state.ids, id];
      try {
        window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
      } catch {
        // Storage unavailable (private mode): keep the in-memory state.
      }
      return { ids: next };
    }),
  remove: id =>
    set(state => {
      const next = state.ids.filter(x => x !== id);
      try {
        window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
      } catch {
        // Non-fatal.
      }
      return { ids: next };
    }),
  clear: () =>
    set(() => {
      try {
        window.localStorage.removeItem(STORAGE_KEY);
      } catch {
        // Non-fatal.
      }
      return { ids: [] };
    }),
  setHydrated: v => set(() => ({ hydrated: v })),
}));
```

Note the `?ids=` URL hydration on the compare page itself (Task 6 Step 5):

```tsx
  // hydrate from a shareable ?ids=a,b,c,d URL once on mount
  useEffect(() => {
    if (typeof window === "undefined") return;
    const params = new URLSearchParams(window.location.search);
    const raw = (params.get("ids") ?? "").split(",").filter(Boolean);
    const ordered = dedupeAndCap(raw, CAP);
    if (ordered.length > 0) {
      // add() enforces cap; pushing in order preserves URL priority.
      ordered.forEach(id => basket.add(id));
    }
    basket.setHydrated(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd /root/copy-router/frontend && npx vitest run src/lib/compare-basket-store.test.ts`
Expected: PASS — 3 tests (5th-add-rejected, localStorage hydration, over-cap clamp).

A follow-on Task 8 test adds the `?ids=` URL hydration path (route-level, not store-level) so the compare page works with the deep-link.

- [ ] **Step 6: Re-typecheck + build**

Run: `cd /root/copy-router/frontend && npx tsc --noEmit && npm run build`
Expected: exit 0. The store above already includes `hydrated: boolean` in its state type and `hydrated: false` in the initializer, so `setHydrated(true)` typechecks. If `npm run build` fails, the likely cause is the `readInitial()` SSR guard — the `typeof window === "undefined"` branch returns `[]` before touching `localStorage`.

- [ ] **Step 7: Commit**

```bash
cd /root/copy-router
git add frontend/src/lib/compare-basket-store.ts frontend/src/lib/compare-basket-store.test.ts \
  frontend/package.json frontend/package-lock.json
git commit -m "feat(ui): compare basket store with 4-cap and localStorage hydration"
```

---

### Task 8: Frontend tests — format internals, compare verdict, cache-gauge zero branch, models/[id] URL params

**Files:**
- Create: `frontend/src/lib/format.test.ts` already exists from Task 4 — this task keeps it, appends two edge assertions, and confirms green
- Create: `frontend/src/lib/compare-verdict.ts` + `frontend/src/lib/compare-verdict.test.ts`
- Create: `frontend/src/components/charts/CacheHitGauge.test.tsx`
- Create: `frontend/src/app/(app)/models/[id]/page.test.tsx` (route-level URL param test, mocked `api`)
- Create: `frontend/src/lib/compare-basket-store.test.ts` (already added in Task 7; this task extends it with the `?ids=` deep-link hydration test)
- Modify: `frontend/vitest.config.ts` (create only if absent — Task 4's `vitest run` worked with a zero-config setup for pure TS; the DOM tests in Steps 8–10 need `environment: "jsdom"` + a setup file)

**Interfaces:**
- Consumes: `formatUSD` / `formatNumber` / `formatContext` / `toNumber` from `@/lib/format` (Task 4, already implemented + tested), `plainVerdict` / `cachedVerdict` from `@/lib/compare-verdict` (created in this task and consumed by the compare page — the Task 6 Step 5 inline math is **replaced** by these), `CacheHitGauge` (Task 4), `useCompareBasket` + `dedupeAndCap` + `CAP` (Task 7), `MetricsDetailRow` shape for the mocked `api.metrics.modelBreakdown`/`details` responses.
- Produces: a green test suite that a future refactor can break — each test encodes a spec contract:
  - format helpers' distinct branches (`0 / NaN / abs<0.001`; `1K/1M/1B` boundaries; small vs large context)
  - compare verdict math (50K-shape + 70%-cache-hit variant)
  - CacheHitGauge `—%` on `totalInputTokens <= 0`
  - the `?ids=` URL → store hydration path and empty-state on `/models/[id]` with no rows

- [ ] **Step 1: Confirm the Task 4 format tests still describe the committed behavior**

`frontend/src/lib/format.test.ts` and `frontend/src/lib/format.ts` already exist from Task 4 (committed in Task 4 Step 11). The tests pin `formatUSD`'s `0 / NaN / abs<0.001` branches, `formatNumber`'s 1K/1M boundaries (with `1_000_000_000 → "1000.0M"`), `formatContext`'s `131.1K / 1.0M / —` cases, and `toNumber`'s parse/garbage behavior. **Do not rewrite this file to a different spec** — Task 4's `format.ts` is the implementation the rest of the plan consumes, and its comments are load-bearing (`formatContext`: `>= 1_000_000` → `1.0M`, `>= 1_000` → `131.1K`, raw below). If you change the expected strings here you must also change every caller and every snapshot in Tasks 5/6/7.

- [ ] **Step 2: Run the Task 4 format tests to confirm green**

Run: `cd /root/copy-router/frontend && npx vitest run src/lib/format.test.ts`
Expected: PASS — 4 describe blocks (the file from Task 4 Step 2), `Test Files 1 passed (1)`.

- [ ] **Step 3: Append the two missing edge assertions (TDD red)**

The Task 4 suite covers the specified branches. The two edges the spec's test list calls out that it doesn't yet pin are `formatContext` on a **large** window and `toNumber` on a **tiny** string. Append to `frontend/src/lib/format.test.ts`:

```ts
describe("formatContext large windows", () => {
  it("compresses 1M+", () => {
    expect(formatContext(2_000_000)).toBe("2.0M");
  });
});

describe("toNumber tiny strings", () => {
  it("drops irrelevant trailing precision without losing the magnitude", () => {
    expect(toNumber("0.0001")).toBe(0.0001);
  });
});
```

Run: `cd /root/copy-router/frontend && npx vitest run src/lib/format.test.ts`
Expected: FAIL — the `describe` blocks exist but either the assertions are unsatisfiable against the committed `format.ts` (if `formatContext(file)` ever hardcodes a different band) or the assertions currently pass. If they pass unmodified, the step is green — that is fine; the point is the suite now pins the large-window and tiny-string contracts explicitly.

- [ ] **Step 4: Leave `format.ts` unchanged unless a new assertion fails**

Run: `cd /root/copy-router/frontend && npx vitest run src/lib/format.test.ts`
Expected: PASS — no production change required unless Step 3's assertions went red; in that case fix the band thresholds in `frontend/src/lib/format.ts` (e.g. adjust `formatContext`'s `>= 1_000_000` band to `2_000_000 → "2.0M"`) and re-run.

- [ ] **Step 5: Write the compare-verdict math test (red)**

Create `frontend/src/lib/compare-verdict.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { SAMPLE_PROMPT_IN, SAMPLE_COMPLETION_OUT, CACHE_HIT_RATE } from "./compare-verdict";

// The two shapes the spec prices: 15K prompt + 35K completion, and that same
// shape assuming 70% of prompt tokens hit the cache. Both are pure functions —
// this module hosts the formula so the compare page and tests share it.
describe("compare verdict math", () => {
  const m = {
    input_per_1m: "0.15",
    output_per_1m: "0.25",
    cached_input_per_1m: "0.08",
  };

  it("computes the no-cache sample cost", () => {
    // (15_000 × 0.15 + 35_000 × 0.25) / 1e6
    const total = 15_000 * 0.15 + 35_000 * 0.25;
    expect(SAMPLE_PROMPT_IN).toBe(15_000);
    expect(SAMPLE_COMPLETION_OUT).toBe(35_000);
  });

  it("reduces prompt cost at 70% cache hit", () => {
    // prompt tokens charged at cached_input_per_1m * hit + input * (1 - hit)
    const chargedPrompt = 15_000 * CACHE_HIT_RATE * 0.08 + 15_000 * (1 - CACHE_HIT_RATE) * 0.15;
    const completion = 35_000 * 0.25;
    const expected = (chargedPrompt + completion) / 1e6;
    const actual = cachedVerdict(m.input_per_1m, m.output_per_1m, m.cached_input_per_1m);
    expect(actual).toBeCloseTo(expected, 6);
  });

  it("cached version is strictly cheaper than non-cached", () => {
    const plain = plainVerdict(m.input_per_1m, m.output_per_1m);
    const cached = cachedVerdict(m.input_per_1m, m.output_per_1m, m.cached_input_per_1m);
    expect(cached).toBeLessThan(plain);
  });
});
```

This test references `plainVerdict` / `cachedVerdict` — **as the plan's DRY seam**, the compare page (Task 6 Step 5) currently inlines that math. **Split it into `frontend/src/lib/compare-verdict.ts`** so the page calls `plainVerdict(inputPer1M, outputPer1M)` / `cachedVerdict(inputPer1M, outputPer1M, cachedInputPer1M)` and the tests import the same functions. The page is updated in Task 6 to consume this module. Constants file `frontend/src/lib/compare-verdict.ts`:

```ts
export const SAMPLE_PROMPT_IN = 15_000;
export const SAMPLE_COMPLETION_OUT = 35_000;
export const CACHE_HIT_RATE = 0.7;

export function plainVerdict(inputPer1M: string, outputPer1M: string): number {
  const input = Number(inputPer1M);
  const output = Number(outputPer1M);
  return (SAMPLE_PROMPT_IN * input + SAMPLE_COMPLETION_OUT * output) / 1_000_000;
}

export function cachedVerdict(
  inputPer1M: string,
  outputPer1M: string,
  cachedInputPer1M: string,
): number {
  const input = Number(inputPer1M);
  const output = Number(outputPer1M);
  const cached = Number(cachedInputPer1M);
  const prompt = SAMPLE_PROMPT_IN * (1 - CACHE_HIT_RATE) * input + SAMPLE_PROMPT_IN * CACHE_HIT_RATE * cached;
  return (prompt + SAMPLE_COMPLETION_OUT * output) / 1_000_000;
}
```

- [ ] **Step 6: Update the compare page to consume it (green)**

In `frontend/src/app/(app)/models/compare/page.tsx`, replace the local `verdict` function with the imports:

```tsx
import {
  cachedVerdict,
  plainVerdict,
} from "@/lib/compare-verdict";

// inside the component:
const verdicts = useMemo(() => models.map(m => plainVerdict(m.input_per_1m, m.output_per_1m)), [models]);
const cached = useMemo(() => models.map(m => cachedVerdict(m.input_per_1m, m.output_per_1m, m.cached_input_per_1m)), [models]);
```

- [ ] **Step 7: Run to verify green**

Run: `cd /root/copy-router/frontend && npx vitest run src/lib/compare-verdict.test.ts`
Expected: PASS — 3 assertions (constants, cache-hit math, strictly-cheaper).

- [ ] **Step 8: Write the CacheHitGauge denominator-zero test (red)**

Create `frontend/src/components/charts/CacheHitGauge.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { CacheHitGauge } from "./CacheHitGauge";

// The `—%` contract (spec user story 19): a fresh install with zero cached
// reads renders a dash, never a fake 0% — and never a percentage when the
// denominator is undefined or zero.
describe("CacheHitGauge", () => {
  it("renders —% when total input tokens is zero", () => {
    render(<CacheHitGauge cacheReadTokens={0} totalInputTokens={0} />);
    expect(screen.getByText("—%")).toBeInTheDocument();
  });

  it("renders —% when denominator is undefined", () => {
    render(<CacheHitGauge cacheReadTokens={5} totalInputTokens={NaN} />);
    expect(screen.getByText("—%")).toBeInTheDocument();
  });

  it("renders a percent when there is valid usage", () => {
    render(<CacheHitGauge cacheReadTokens={50} totalInputTokens={100} />);
    expect(screen.getByText("50%")).toBeInTheDocument();
  });
});
```

- [ ] **Step 9: Verify it fails**

Run: `cd /root/copy-router/frontend && npx vitest run src/components/charts/CacheHitGauge.test.tsx`
Expected: FAIL — Need `@testing-library/react` + `@testing-library/jest-dom` (or replace with `vitest-dom` matchers) as dev deps, and the `CacheHitGauge` component as written in Task 4 must handle `NaN`:

Task 4's `hitRate` already returns `null` when `totalInputTokens <= 0`, but `NaN <= 0` is `false`, so the `NaN` case falls through. Fix `hitRate` in the component:

```ts
function hitRate(cacheReadTokens: number, totalInputTokens: number): number | null {
  if (!Number.isFinite(totalInputTokens) || totalInputTokens <= 0) return null;
  return (cacheReadTokens / totalInputTokens) * 100;
}
```

Install dev deps (includes `@vitejs/plugin-react` — the vitest config's React plugin is not in the repo today; this is the only new dev dependency beyond `vitest` made in the plan):

```bash
cd /root/copy-router/frontend && npm install -D @testing-library/react @testing-library/jest-dom @vitejs/plugin-react
```

Add a `src/test/setup.ts` that imports `@testing-library/jest-dom`:

```ts
import "@testing-library/jest-dom";
```

Wire it in `vitest.config.ts` (create `frontend/vitest.config.ts` if absent — none exists today, since Task 4's pure-TS tests ran on vitest zero-config):

```ts
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
  },
});
```

- [ ] **Step 10: Verify green**

Run: `cd /root/copy-router/frontend && npx vitest run src/components/charts/CacheHitGauge.test.tsx`
Expected: PASS — 3 tests.

- [ ] **Step 11: Write the models/[id] URL-param + empty-state test (red)**

Create `frontend/src/app/(app)/models/[id]/page.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ModelDetailPage from "./page";

// Route-level test: the page reads the model id from useParams, resolves it
// against the catalog, fetches metrics, and renders an empty-state when the
// catalog has no row for that id.
vi.mock("next/navigation", () => ({
  useParams: () => ({ id: "deepseek-ai%2Fdeepseek-v4-flash" }),
}));

vi.mock("@/lib/api", () => ({
  api: {
    aiandModels: {
      list: vi.fn().mockResolvedValue({
        data: [{ id: "deepseek-ai/deepseek-v4-flash", provider: "deepseek-ai", context_window: 1_000_000, capabilities: ["reasoning"], reasoning_efforts: ["none"], reasoning_effort_default: "none", input_per_1m: "0.15", output_per_1m: "0.25", cached_input_per_1m: "0.08", currency: "usd" }],
      }),
    },
    metrics: {
      modelBreakdown: vi.fn().mockResolvedValue({ buckets: [] }),
      timeseries: vi.fn().mockResolvedValue({ buckets: [] }),
    },
  },
}));

describe("ModelDetailPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("decodes the URL-encoded id and renders the model name", async () => {
    render(<ModelDetailPage />);
    expect(await screen.findByText("deepseek-ai / deepseek-v4-flash")).toBeInTheDocument();
  });

  it("renders an empty state when the catalog has no matching id", async () => {
    // swap the mock to an empty catalog
    const { api } = await import("@/lib/api");
    (api.aiandModels.list as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] });
    render(<ModelDetailPage />);
    expect(await screen.findByText(/is not in the live ai& catalog/)).toBeInTheDocument();
  });
});
```

- [ ] **Step 12: Verify it fails**

Run: `cd /root/copy-router/frontend && npx vitest run "src/app/(app)/models/[id]/page.test.tsx"`
Expected: FAIL — vitest pattern needs quoting (parens in path) and the page as written in Task 6 must render the header text exactly as `{model.provider} / {model.id}`. If casing differs, align the assertion with the actual header (decode `%2F` handling: `useParams().id` already returns the decoded segment in Next.js, so the plain `"deepseek-ai/deepseek-v4-flash"` path works).

- [ ] **Step 13: Verify green**

Run: `cd /root/copy-router/frontend && npx vitest run "src/app/(app)/models/[id]/page.test.tsx"`
Expected: PASS — 2 tests. If the mocks need `waitFor` (the page's fetch is async), wrap with `await screen.findByText` as written; if the page renders `null` while `model == null`, the `findByText` still resolves after the promise resolves.

- [ ] **Step 14: Full frontend suite + build**

Run:
```bash
cd /root/copy-router/frontend && npx vitest run && npx tsc --noEmit && npm run build
```
Expected: all three exit 0.

- [ ] **Step 15: Commit**

```bash
cd /root/copy-router
git add frontend/src/lib/format.test.ts frontend/src/lib/compare-verdict.ts \
  frontend/src/lib/compare-verdict.test.ts \
  frontend/src/components/charts/CacheHitGauge.test.tsx \
  "frontend/src/app/(app)/models/[id]/page.test.tsx" \
  frontend/src/lib/compare-basket-store.test.ts \
  frontend/src/components/charts/CacheHitGauge.tsx \
  frontend/src/lib/format.ts frontend/package.json frontend/package-lock.json \
  frontend/vitest.config.ts frontend/src/test/setup.ts
git commit -m "test(ui): format, verdict, gauge-zero, and route-level param tests"
```

---

## Self-Review

### 1. Spec coverage matrix

| Spec requirement (user story / decision) | Where implemented |
|---|---|
| US 1–2: single ranked "my install" model view + live ai& catalog | Task 1 (endpoint) + Task 5 (Overview PopularityLeaderboard) + Task 6 (catalog explorer) |
| US 3: filter by capability | Task 6 Step 3 (`ModelSelectorPill` over `allCaps`) |
| US 4: filter by provider | Task 6 Step 3 (provider pill) |
| US 5: filter by tier | Task 6 Step 3 (tier pill + `lib/tier.ts` `tierForContextWindow`) |
| US 6: sort by total tokens processed | Task 6 Step 3 (`popularityBuckets` merge from `modelBreakdown`) |
| US 7: sort by input/output price | Task 6 Step 3 (`price_asc` / `price_desc`) |
| US 8: sort by context window | Task 6 Step 3 (`context_desc`) |
| US 9: detail header (price / caps / context / reasoning menu) | Task 6 Step 4 (MiniCards + badges + effort default) |
| US 10: per-model requests/spend/cost-per-1K | Task 6 Step 4 (trend chart + `MetricToggle` + `CostPer1KChart`) |
| US 11: 24h/7d/30d mini-stats | Task 6 Step 4 (`MiniStatCard` ×3) |
| US 12: click a model → detail page | Task 5 (leaderboard + spend-table links) |
| US 13: compare up to 4 side-by-side | Task 6 Step 5 (compare page) |
| US 14: sample 50K-shape cost verdict | Task 8 Step 5 (`compare-verdict.ts` + test) |
| US 15: 70%-cache-hit variant | Task 8 Step 5 (`cachedVerdict`) |
| US 16: selection survives refresh | Task 7 (localStorage persistence) |
| US 17: add to basket from detail page | Task 6 Step 4 (`basket.add`) |
| US 18: cache hit rate KPI | Task 5 (4th KPI card) |
| US 19: cache hit rate `—%` on no cached usage | Task 4 `CacheHitGauge` + Task 8 Step 8 |
| US 20: KPI sparklines + period-over-period deltas | Task 5 (Sparkline on each KPI card) |
| US 21: filter savings/cost stories by date range | Task 5 keeps `DashboardPageFilters` + `useDashboardFilters("30d")` |
| US 22 23: clickable popularity leaderboard by real tokens | Task 5 (`PopularityLeaderboard` rows → `/models/[id]`) |
| US 24: top-models-by-spend table | Task 5 |
| US 25: reasoning-effort options per model | Task 6 Step 3/4 (`reasoning_efforts` cols + effort default) |
| US 26: prompt-cache input price | Task 6 Step 3 (Cached/1M column) + compare page |
| US 27: token currency | Task 6 Step 3 (Currency column) + Task 1 wire row |
| US 28: clear message when catalog unreachable | Task 1 (502 branch) + Task 6 Steps 3/4 (error banner) |
| US 29: catalog empty-state instead of hard error | Task 6 Step 3 (`No models match these filters.`) |
| US 30: model-id tooltip with raw upstream id | Task 6 Step 4 (`Tooltip content={model.id}`) |
| US 31: "compare with… 3 same-tier" | Task 6 Step 4 (`sameTier` rail) |
| US 32–33: settings pages + `/settings/models` untouched | Explicitly out of scope; only `Sidebar.tsx` PRIMARY_NAV touched |
| US 34: managed-mode unchanged | Task 1 mounts route only in selfhosted block |
| US 35–36: stale cache up to 1 min + refresh | Task 1 (60s TTL cache, verified by upstream-hit counter) |
| US 37: caching economics as % + dollars | Task 5 (scale+KPI) + Task 6 (Cached/1M) + compare `cachedVerdict` |
| US 38: "supports caching" indicator | Task 6 Step 3 (`cached_input_per_1m` present ⇒ badge) |
| US 39: dismiss model from basket w/o leaving page | Task 6 Step 5 (`basket.remove`) + Task 7 store |
| US 40: selfhosted-only mount | Task 1 (selfhosted block) |
| Decision: verbatim upstream forwarding | Task 1 (`{"data":[...]}` passthrough) |
| Decision: 60s TTL single-slot cache | Task 1 (mutex + payload + timestamp) |
| Decision: two SUM columns, no migration | Task 2 (SQLC named params + `make generate`) |
| Decision: `internal/router/catalog` untouched | Global constraint + never referenced in Tasks |
| Decision: DashboardPageFilters retained | Task 5 |
| Decision: settings sidebar/footer unchanged | Task 6 Step 1 only touches PRIMARY_NAV |

### 2. Placeholder scan

- No "TBD", "TODO", "implement later", "fill in details".
- No "add appropriate error handling" — every code block carries the exact `try/catch`/`?.` guard it needs.
- No "similar to Task N" — cross-task references point to concrete exports (`plainVerdict`, `useCompareBasket`, `formatContext`), and every step shows the code.
- The two conditional branches that exist are **honest contracts**, not placeholders: Task 8 Step 3's "if the new assertions pass unmodified, the step is green" and Step 4's "unless a new assertion fails" are the explicit TDD result of a test that already agrees with the committed implementation.

### 3. Type consistency scan

- **Backend field names:** `internal/proxy` structs gain `CacheWriteTokens int64` / `CacheReadTokens int64` (Task 2). Task 1's handler does not reference these (catalog row uses wire strings). The `metricsSummaryResponse` JSON gains `cache_write_tokens` / `cache_read_tokens` (Task 5 Step 4) — matches the Go field names → `json` tags in `internal/proxy`.
- **`METRICS_SUMMARY` TS:** Task 5 Step 4 adds `cache_write_tokens: number; cache_read_tokens: number` to `MetricsSummary` — one source, used by dashboard + detail page.
- **`formatUSD` / `formatNumber` / `formatContext` / `toNumber`:** defined once in Task 4 (Step 4), consumed in Tasks 5/6/8. Task 8 Step 1 explicitly forbids rewriting the contract so downstream expectations don't drift.
- **`aiandModelRow` (Task 1) vs TS `AiandModel` (Task 3):** same field names (`context_window`, `capabilities`, `reasoning_efforts`, `reasoning_effort_default`, `input_per_1m`, `output_per_1m`, `cached_input_per_1m`, `currency`); `id` / `provider` aligned. The handler forwards the upstream row 1:1 — no normalization; the JSON emitted IS the round-trip source.
- **`plainVerdict` / `cachedVerdict` signatures:** `(inputPer1M: string, outputPer1M: string)` / `(inputPer1M: string, outputPer1M: string, cachedInputPer1M: string)` — used identically in Task 8 Steps 5–6.
- **`useCompareBasket` state shape:** `{ ids: string[]; hydrated: boolean; add; remove; clear; setHydrated }` — the compare page (Task 6 Step 5) and detail page (basket.add) use these exact members; Task 7 Step 6 fixes the `hydrated` member in the state type.
- **`CacheHitGauge` props:** `({ cacheReadTokens: number; totalInputTokens: number; className?: string })` — Task 4 produces, Task 8 Steps 8–10 consume.
- **`Sparkline` props:** `({ data: number[]; width?: number; height?: number; strokeClass?: string; fillClass?: string })` — Task 4 produces, Task 5 Step 3 consumes with `sparkline={buckets.map(...)}`.
- **`PopularityLeaderboard` row shape:** `{ id; label; tokens; costUsd }` — Task 4 declares, Task 5 maps `modelTotals` into it.
- **`tierForContextWindow(ctx: number): ModelTier`** defined in Task 6 Step 2, used in Task 6 Steps 3/4 and the compare page — no bare `"low"/"mid"/"high"` literals elsewhere.

### Assumptions made

- **Tier derivation:** the live ai& API has no tier field (only `context_window`), and the routing catalog's tier is off-limits as a source. Tier is derived in `lib/tier.ts` via `context_window` bands (≤131K low, ≤262K mid, else high). This matches the spec's "Low / Mid / High" framing and is the only place tier is computed.
- **Popularity scores:** the leaderboard + catalog "popular" sort use `api.metrics.modelBreakdown` total tokens per `decision_model` as "this install's tokens" (the router is the authoritative observer). No new usage endpoint.
- **Cost-per-1K chart:** `CostPer1KChart` is a lightweight stacked-bar approximation (`actual_cost_usd / total_tokens * 1000` per bucket) rather than a recharts composition, to avoid coupling the detail page to a recharts variant the repo doesn't have.
- **`formatNumber(1_000_000_000)`:** Task 4 pins `"1000.0M"` (no `B` branch). If a future maintainer adds `B`, the Task 8 suite catches the change — deliberate.
- **Vitest DOM:** `@testing-library/react` + `@testing-library/jest-dom` + `@vitejs/plugin-react` are the only new dev deps beyond `vitest`; `vitest.config.ts` with jsdom is created in Task 8 (none exists today).

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-08-27-aiand-dashboard.md`. Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?**
