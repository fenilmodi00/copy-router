## Summary

Promotes the cluster scorer from **researched anchors** (v0.78) to **measured labels** (v0.79): per-(cluster, model) quality cells computed from the RouterArena label campaign — real generations of the roster graded against RouterArena ground truth. **Roster unchanged**: same 8 models (glm-5.3, kimi-k3, glm-5.3-flash, v4.1-flash, v4-pro, qwen3.8-27b, motif-3, v4-flash — AA index 45/44/42/39/36/34/34/34 per [#64](https://github.com/fenilmodi00/copy-router/issues/64)), same registry enforcement, same frozen centroids; only the quality cells change. Contract per [#71](https://github.com/fenilmodi00/copy-router/issues/71) (T7); [#72](https://github.com/fenilmodi00/copy-router/issues/72) (T8) carries the live smoke and closes #64.

```text
aiand-only
├── internal/router/cluster/                 # v0.79 bundle: measured cells; centroids byte-identical to v0.78
├── scripts/build_v079_aiand_measured.py   # measured-label trainer (T7)
├── scripts/v079_comparison.py             # offline v0.78 -> v0.79 report (winners, rank deltas, anchor flips)
└── artifacts/latest -> v0.79              # promotion flips routing; no deploy
```

```text
build_v079_aiand_measured.py
  embed prompts      jina-v2 mean-pool, L2, tail-truncate 1024 chars (runtime contract)
  assign clusters    argmax on frozen v0.78 centroids (geometry frozen)
  z-score            per prompt, across the 8 model columns
  shrink             per-(cluster, model) means, shrinkage_k0=10
  parity gate        then prune to the 8-model roster
```

Label integrity: the LiveCodeBench coding slice was re-run at max_tokens=4096 (at 1024 it truncated, putting our weakest cluster near floor); the evaluator skips truncated/failed generations — zero silent nulls — and the builder treats a missing score as absent, not zero.

## Evidence

**Campaign state (T5, #69):** 4,630/6,472 rows (72%), $5.75 spent; 5/8 models complete at 809/809 — v4-pro (740 ok), v4-flash (700), v4.1-flash (695), glm-5.3 (689), glm-5.3-flash (715); qwen3.8-27b 459/809, kimi-k3 63/809, motif-3 63/809 still in flight ([checkpoint 2026-10-05T00:38:45Z](https://github.com/fenilmodi00/copy-router/issues/69#issuecomment-5986147926)). Fleet mechanics: 2 processes × 6 in-flight workers, ~6.6s/row, zero 429s (AIand serializes at the org tier); outputs capped at max_tokens=1024 with a single 2048 retry ([T5 start, 2026-10-04T15:32:37Z](https://github.com/fenilmodi00/copy-router/issues/69#issuecomment-5981636210)).

**Before — v0.78 researched anchors** (cells are AA-index priors, `aa_evidence_scale` 3.0; table computed from the committed bundle `internal/router/cluster/artifacts/v0.78/` with the winner/avg-rank code path of `scripts/v079_comparison.py`; AA indexes from [#64](https://github.com/fenilmodi00/copy-router/issues/64)):

| model | AA idx | v0.78 avg rank (16 clusters) | quality winner in |
|---|---|---|---|
| zai-org/glm-5.3 | 45 | 1.00 | 12/16 (c00–c08, c12–c14) |
| moonshotai/kimi-k3 | 44 | 1.75 | — |
| zai-org/glm-5.3-flash | 42 | 2.50 | — |
| deepseek-ai/deepseek-v4.1-flash | 39 | 3.25 | — |
| deepseek-ai/deepseek-v4-pro | 36 | 4.00 | — |
| deepseek-ai/deepseek-v4-flash | 34 | 4.75 | 4/16 (c09–c11, c15) |
| qwen/qwen3.8-27b | 34 | 5.50 | — |
| motif-technologies/motif-3 | 34 | 6.25 | — |

Blend (`rankings`) winner split is identical: glm-5.3 ×12, v4-flash ×4. The researched ordering tracks the AA index exactly — measurement is what can break that tie.

**After — v0.79 measured cells** (sub_10: 809 prompts × 8 models, real ground truth):

| model | measured accuracy (full run) | v0.79 avg rank | Δ rank vs v0.78 | clusters won |
|---|---|---|---|---|
| deepseek-ai/deepseek-v4-pro | **TODO(v0.79-build)** | **TODO(v0.79-build)** | **TODO(v0.79-build)** | **TODO(v0.79-build)** |
| deepseek-ai/deepseek-v4-flash | **TODO(v0.79-build)** | **TODO(v0.79-build)** | **TODO(v0.79-build)** | **TODO(v0.79-build)** |
| deepseek-ai/deepseek-v4.1-flash | **TODO(v0.79-build)** | **TODO(v0.79-build)** | **TODO(v0.79-build)** | **TODO(v0.79-build)** |
| zai-org/glm-5.3 | **TODO(v0.79-build)** | **TODO(v0.79-build)** | **TODO(v0.79-build)** | **TODO(v0.79-build)** |
| zai-org/glm-5.3-flash | **TODO(v0.79-build)** | **TODO(v0.79-build)** | **TODO(v0.79-build)** | **TODO(v0.79-build)** |
| qwen/qwen3.8-27b | **TODO(v0.79-build)** | **TODO(v0.79-build)** | **TODO(v0.79-build)** | **TODO(v0.79-build)** |
| moonshotai/kimi-k3 | **TODO(v0.79-build)** | **TODO(v0.79-build)** | **TODO(v0.79-build)** | **TODO(v0.79-build)** |
| motif-technologies/motif-3 | **TODO(v0.79-build)** | **TODO(v0.79-build)** | **TODO(v0.79-build)** | **TODO(v0.79-build)** |

(Final per-model accuracy = the completion watcher's full `aiand_grade.py` pass at 809×8; ranks/winners = `python scripts/v079_comparison.py` output.)

Early signal (partial grading pass, real ground truth, n varies — noisy on small n; glm-5.3/glm-5.3-flash still truncate at 4096 on long prompts and are graded with those rows skipped) ([first measured accuracies, 2026-10-04T18:25:58Z](https://github.com/fenilmodi00/copy-router/issues/69#issuecomment-5983042663)):

| model | n | accuracy |
|---|---|---|
| deepseek-ai/deepseek-v4.1-flash | 42 | 78.6% |
| qwen/qwen3.8-27b | 25 | 76.0% |
| motif-technologies/motif-3 | 25 | 76.0% |
| deepseek-ai/deepseek-v4-flash | 252 | 72.2% |
| moonshotai/kimi-k3 | 25 | 68.0% |
| deepseek-ai/deepseek-v4-pro | 722 | 64.9% |
| zai-org/glm-5.3 | 40 | 60.0% |
| zai-org/glm-5.3-flash | 357 | 55.2% |

**Cluster winner changes v0.78 → v0.79:** **TODO(v0.79-build)** — post-build run of `scripts/v079_comparison.py`: per-cluster quality + blend winner changes (count + cluster list), per-model average-rank deltas, and researched-anchor flips annotated with/against the AA Intelligence Index.

**LCB rerun (coding-slice rescue):** 38 coding prompts × 8 models redone at max_tokens=4096 — usable answers: motif-3 38/38 (was 0 at 1024), kimi-k3 36/38 (0), v4-pro 35/38 (23), v4.1-flash 32/38 (4), v4-flash 30/38 (15), glm-5.3 23/38 (3), glm-5.3-flash 22/38 (9), qwen3.8-27b 21/38 (0); rerun spend $2.44, cumulative $3.77 at that point ([LCB rerun complete, 2026-10-04T20:36:36Z](https://github.com/fenilmodi00/copy-router/issues/69#issuecomment-5984143273)).

**Suites:** **TODO(v0.79-build)** — `artifacts_v079_test.go` (bundle loads as v2, version/parent v0.79/v0.78, exactly the 8-model AIand roster under aiand-only AND all-provider boots, every candidate on provider aiand, centroids byte-identical, dropped models absent), `go test ./internal/router/cluster/...`, `cmd/routing-report` on `latest`, and the T8 live smoke (docker compose with only `AIAND_API_KEY`: boot shows cluster v0.79 with the 8-model roster; `model:auto` → HTTP 200, `decision_provider=aiand`, `decision_model` within the 8).

v0.78 smoke evidence already in this PR stands ([PR #73](https://github.com/fenilmodi00/copy-router/pull/73)): boot `cluster_version=v0.78` with exactly the 8 models; `model:auto` → HTTP 200, `decision_model=zai-org/glm-5.3`, `decision_provider=aiand`; a BYOK-widening request still routed inside the AIand roster.

## Merge Danger

**Door:** two-way

`artifacts/v0.78` and `artifacts/v0.79` are immutable directories; `artifacts/latest` is a pointer that can be repointed back to v0.78 with no data migration — the only client-visible change is routing decisions.

**Blast Radius:** routing

`latest` flips v0.78 → v0.79, so every auto-routed turn's per-cluster winner can change wherever measured cells disagree with the researched anchors — strictly within the 8-model roster (registry unchanged, centroids byte-identical, no deploy required). Ramifications: a cluster whose measured winner has a lower AA index than its researched anchor routes to a model that research ranks weaker — the comparison tool flags every flip with/against research so each is reviewable before merge; and glm-5.3-flash's partial-pass measured accuracy (55.2%, n=357) sits far below its researched anchor, so the current cheap-lane workhorse may lose clusters it wins today — the main watch item once the final numbers land.
