# Adding GLM-5.3 + Qwen3.8-27B: Six-Model Router Roster

> **Status: Implemented 2026-08-29** on branch `feat/aiand-dashboard-metrics`. ai&
> availability confirmed live (GET /v1/models). The router now serves exactly
> six models; every other catalog row was removed per the roster cut.

## What changed

The goal: better routing at lower cost. Auto-mode routes only within these six
models (live-verified against ai& on 2026-08-29):

| # | Model | Tier | Context | Vision | Efforts | In/Out $/1M | Cached $/1M |
|---|---|---|---|---|---|---|---|
| 1 | zai-org/glm-5.3 | High | 1,048,576 | no | none/low/xhigh/max | 1.00 / 4.00 | 0.30 |
| 2 | qwen/qwen3.8-27b | Low | 262,144 | **yes** | none/low/medium/xhigh | 0.40 / 3.00 | 0.20 |
| 3 | moonshotai/kimi-k3 | High | 1,048,576 | yes | low/high/max | 3.00 / 12.50 | 0.50 |
| 4 | motif-technologies/motif-3 | Mid | 262,144 | no | low/medium/high | 0.50 / 2.00 | 0.20 |
| 5 | moonshotai/kimi-k2.7 | High | 262,144 | no | high | 0.75 / 3.50 | 0.20 |
| 6 | deepseek-ai/deepseek-v4-flash | Low | 1,048,576 | no | none/high/max | 0.15 / 0.25 | 0.08 |

Removed catalog rows: `zai-org/glm-5.2`, `deepseek-ai/deepseek-v4-pro`,
`openai/gpt-oss-120b`, `qwen/qwen3.6-27b`, `google/gemma-4-31b-it`.

| Artifact | Change |
|---|---|
| `internal/router/catalog/catalog.go` | Six-model roster; new `EffortXHigh` constant |
| `internal/router/catalog/alias_test.go`, `catalog_test.go` | Successor-alias contract tests |
| `scripts/probe_glm53_medium.py` | 5-model probe (adds qwen3.8-27b, error retry, error-aware pass rates) |
| `scripts/glm53_probe_results.json` | Probe output: go/no-go + quality factors |
| `scripts/build_v077_aiand_overlay.py` | Six-model overlay script (glm-5.3 replaces glm-5.2, qwen3.8-27b joins) |
| `internal/router/cluster/artifacts/v0.77/` | New six-model bundle |
| `install/{install.sh,cc-statusline.sh}`, `install/pi-router/src/pricing.generated.ts` | Regenerated via `go run ./cmd/genprices` |

## Live facts that differed from the original planning assumptions

The planning doc assumed `$1.40/$4.40/$0.26` pricing and `low/high/max`
mandatory reasoning. Live ai& (2026-08-29) actually serves:

- **id**: `zai-org/glm-5.3`, ctx 1,048,576, text-only (capabilities:
  reasoning, tool_calling — no vision)
- **efforts**: `none/low/xhigh/max` — the live menu includes `none`, and has
  **`xhigh` instead of `high`**. The doc's "mandatory reasoning (no none)"
  claim was wrong. A new `EffortXHigh = "xhigh"` constant was added; glm-5.3
  does not accept `high` or `medium`.
- **pricing**: $1.00 in / $4.00 out / $0.30 cached per 1M. Cache multiplier
  `0.30 / 1.000` (30% of base input, same shape as glm-5.2's).

qwen/qwen3.8-27b joined the roster after the initial plan (user request):
TierLow pool model, ctx 262,144, live vision/document/video capabilities (so
no `ImageInputUnsupported` — it stays eligible for image-bearing turns),
efforts `none/low/medium/xhigh` (no high, no max), $0.40/$3.00/$0.20.

## Catalog changes

```go
// new effort vocabulary constant
EffortXHigh = "xhigh"

{ID: "zai-org/glm-5.3", Tier: TierHigh, ContextWindow: 1_048_576, ImageInput: ImageInputUnsupported,
	ReasoningEfforts: []string{EffortNone, EffortLow, EffortXHigh, EffortMax},
	Providers: []ProviderBinding{
		{Provider: providers.ProviderAiand, UpstreamID: "zai-org/glm-5.3",
			Price: Pricing{InputUSDPer1M: 1.000, OutputUSDPer1M: 4.000, CacheReadMultiplier: 0.30 / 1.000}},
	}},

{ID: "qwen/qwen3.8-27b", Tier: TierLow, ContextWindow: 262_144,
	ReasoningEfforts: []string{EffortNone, EffortLow, EffortMedium, EffortXHigh},
	Providers: []ProviderBinding{
		{Provider: providers.ProviderAiand, UpstreamID: "qwen/qwen3.8-27b",
			Price: Pricing{InputUSDPer1M: 0.400, OutputUSDPer1M: 3.000, CacheReadMultiplier: 0.20 / 0.400}},
	}},
```

Cache math: glm-5.3 `0.30/1.000 = 0.30` (same 70% discount as glm-5.2);
qwen3.8 `0.20/0.400 = 0.50` (50% discount).

### Aliases

The user instruction to remove every non-roster row supersedes this doc's
earlier "keep GLM-5.2 in catalog" guidance. The alias table is the designed
mechanism for retired rows — frozen artifacts and stored session pins keep
resolving:

```go
var aliases = map[string]string{
	"z-ai/glm-5.2":    "zai-org/glm-5.3",
	"zai-org/glm-5.2": "zai-org/glm-5.3",
	"z-ai/glm-5.3":    "zai-org/glm-5.3",
	"qwen/qwen3.6-27b": "qwen/qwen3.8-27b",
}
```

`deepseek-v4-pro`, `gpt-oss-120b`, and `gemma-4-31b-it` deliberately have NO
aliases — they have no successor; pins referencing them miss and re-route via
the scorer.

## AA Index benchmark basis

Collected from the internet 2026-08-29 (artificialanalysis.ai primary; full
research in `local/aa-benchmarks.md`). All six roster models have direct AA
Intelligence Index entries:

| Model | AA Index | Terminal-Bench | DeepSWE v1.1 | Confidence |
|---|---|---|---|---|
| zai-org/glm-5.3 | 60 | 28.3 (TB 3.0) / 88.2 (TB 2.1) | 66.9 | Direct AA; TB/DeepSWE vendor |
| moonshotai/kimi-k3 | 60 | 88.3 (TB 2.1) | 67.5 | Direct AA |
| qwen/qwen3.8-27b | 52 | 73.0 (TB 2.1) | 42.2 | Direct AA |
| deepseek-ai/deepseek-v4-flash | 52 | 82.7 (TB 2.1) | 54.4 | Direct AA |
| motif-technologies/motif-3 | 47 | not published | not published | Direct AA |
| moonshotai/kimi-k2.7-code | 43 | no completed runs | not published | Direct AA |

GLM-5.2 (replaced): AA 53, TB 2.1 ~78–81, DeepSWE 46.2. The replacement delta:
AA +7, Terminal-Bench 3.0 4.6→28.3, DeepSWE 46.2→66.9.

## Probe

`scripts/probe_glm53_medium.py` sends the same 30 medium-difficulty coding
prompts used by `probe_motif3_medium.py` to 5 models (glm-5.3 pinned at
`max` effort, qwen3.8-27b pinned at `medium`, leaders at their defaults),
scores pass/fail, and derives the v0.77 quality factors. Errored calls are
retried up to 3 times and excluded from pass-rate denominators so transient
network failures don't poison the go/no-go. Output:
`scripts/glm53_probe_results.json` — `go_no_go` (PASS iff glm-5.3 pass rate ≥
kimi-k3's), `recommended_q_glm53_on/off`, `recommended_q_qwen38_off`.

## v0.77 bundle

`scripts/build_v077_aiand_overlay.py` reads v0.76 artifacts and produces the
six-model bundle at `internal/router/cluster/artifacts/v0.77/`:

- **centroids.bin**: byte-identical copy from v0.76 (frozen geometry).
- **Roster edit**: glm-5.2 → glm-5.3 rename; qwen3.8-27b appended; removed
  models were never registry members (v0.76 registry held only 5).
- **Registry**: glm-5.3 proxies the GLM-5.2 Arena column
  (`routerarena_z-ai/glm-5.2`); qwen3.8-27b proxies its predecessor's
  (`routerarena_qwen/qwen3.6-27b`), proxy_note records it's a TierLow pool
  model that wins no clusters.
- **Tier structure unchanged**: kimi-k3 wins HARD_K3 {0,13}; motif-3 wins
  MID_M3 {1,3,4,5,6,7,8,12,14} at 1.1x maxpool (motif probe, PASS); flash
  wins FLASH_5 {2,9,10,11,15}.
- **glm-5.3 quality** = glm-5.2's per-cluster values × probe factor (on-factor
  on hard clusters and wherever glm-5.2 was cluster max, off-factor
  elsewhere), clamped below the tier leader if it would overtake.
- **qwen3.8-27b quality** = pool max × `recommended_q_qwen38_off`, clamped
  below every tier leader so it never wins a cluster.
- **Self-check**: tier leaders must stay strictly-highest on their clusters.
- **Status**: candidate — NOT promoted to latest. Promotion is a separate
  human decision (staging first, then `latest` edit + redeploy).

## Testing and validation

```bash
go test ./internal/router/catalog/...   # roster + alias contract
go test ./internal/router/cluster/...   # bundle loads, scorer argmax
go run ./cmd/genprices                   # regenerate install price tables
go build ./... && go test ./internal/... ./cmd/...
go vet -tags smoke ./smoke/...          # smoke package compile check
```

## What NOT to do

- **Do NOT add a modelIDMap entry** — glm-5.3's upstream ID equals its catalog
  ID; no rewrite needed.
- **Do NOT add a new provider constant** — both new models use
  `providers.ProviderAiand`.
- **Do NOT change the centroid geometry** — centroids.bin is byte-identical
  across versions.
- **Do NOT add a new tier** — three-tier system is Low/Mid/High.
- **Do NOT re-add removed models' aliases** for deepseek-v4-pro /
  gpt-oss-120b / gemma-4-31b-it — no successor exists; pins must miss and
  re-route.
