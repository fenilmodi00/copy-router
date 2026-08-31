# Scorer retraining pipeline — how v0.75–v0.77 were built and what v0.78 needs

*How the cluster-scorer bundles in `internal/router/cluster/artifacts/` were actually produced (v0.75 → v0.76 → v0.77), where centroid training lives (spoiler: not this repo), how per-model quality was measured on a probe budget, and exactly what is missing to produce v0.78 with a six-model roster.*

---

## TL;DR

1. **v0.75–v0.77 were not retrains — they are "frozen-geometry roster overlays."** Every new-model bundle copied `centroids.bin` byte-identical from its parent and only edited the four JSON files + `metadata.yaml` via overlay scripts (`build_v076_aiand_overlay.py`, `build_v077_aiand_overlay.py`). The last actual K=16 clustering happened upstream in the sibling training repo before v0.75 landed here (`internal/router/cluster/AGENTS.md:38` names `train_cluster_router.py`; `internal/router/cluster/artifacts/README.md:67` places it in `router-internal/eval/`).
2. **Per-cluster quality for new models is a multiplier on a probed pool, not a measured cell.** Each probe (30–100 prompts × a few models, pass/fail on a deterministic token) produces a `recommended_*` factor; the overlay multiplies the parent's per-cluster pool max by that factor and clamps below tier leaders.
3. **The label + prior inputs the trainer consumes (`routerarena_labels_combined.jsonl`, `aa_quality_priors_v0.64.json`, the benchmark-corpus loaders) are NOT in this repo** — only `metadata.yaml` records their filenames (`v0.76/metadata.yaml:26,35`). Weights, scores, and effective quality tables are visible; the raw training corpus is not.
4. **Bundles are load-enforced**: the Go loader pins embedder ID+dim (`scorer.go:120–127`), the CRT1 centroid magic (`artifacts.go:25–28`), `MaxPromptChars=1024` / 1500 ms embed timeout (`scorer.go:39–46`), and fails closed with `ErrClusterUnavailable`→503 rather than any fallback (`internal/router/cluster/AGENTS.md:78`).
5. **Promotion = one-line edit to `artifacts/latest`** (contents: `v0.77`), plus a redeploy; there is no automated gate (`internal/router/cluster/AGENTS.md:26–27`). Git history confirms both v0.76 (commit `061cfe1a`) and v0.77 (commit `e13b1d52`) were promoted this way after first shipping as candidates.
6. **v0.78 with the six-model roster is entirely reachable in-repo** on frozen geometry: probe the models that changed, run a v0.78 overlay script against v0.77's artifacts, self-check the tiers, ship as candidate, promote via `latest`. A true retrain (new K, new clusters) is NOT reachable from this repo — see "Where training actually happens".

---

## Bundle anatomy

Every committed bundle is one frozen directory under `internal/router/cluster/artifacts/v<X.Y>/` (legacy v1 format under `legacy/`) loaded via `//go:embed all:artifacts` at boot (`internal/router/cluster/artifacts.go:38–41`, `internal/router/cluster/artifacts/README.md:9–23`).

| File | Role (v2 format, in use since v0.53) | Cited contract |
|---|---|---|
| `centroids.bin` | K=16 × 768-dim L2-normalized centroids; binary format `magic "CRT1"` + `version 1` + k + dim + `[k][dim]float32` | `internal/router/cluster/artifacts.go:18–28`; loader refuses mismatched sizes (`artifacts.go:508–512`) |
| `quality_means.json` | Per-(cluster, model) raw quality means `Q̄[k][m]` — the pre-blend table the runtime re-blends live. `meta` block carries the full training recipe (k, alpha, shrinkage_k0, score_normalization, n_prompts, benchmark loaders, AA-prior settings, residuals) plus pinned catalog costs | `internal/router/cluster/artifacts/README.md:32–34`; verified meta keys of `v0.75/quality_means.json` (`aa_evidence_scale`, `aa_label_tier_weights`, `source_fractions`, `measured_speed_source`, …) |
| `model_axes.json` | Per-model raw operational axes: `input_per_1k_usd`, `output_per_1k_usd`, `tps`, `ttft_s`, `verbosity_tokens` | `internal/router/cluster/artifacts/README.md:34–37` |
| `model_features.json` | Per-model `operational` block + `psi_probe` (the model's per-cluster quality vector, copied from `quality_means.json` by the overlay scripts) | `scripts/build_v076_aiand_overlay.py:185–216`; `scripts/build_v077_aiand_overlay.py` features block verified |
| `model_registry.json` | Deployed entries: `model`, `provider`, `bench_column`, `proxy`, `proxy_note`. Direct columns are 1:1; proxy entries reuse another model's bench scores | `internal/router/cluster/artifacts.go:77–85` |
| `metadata.yaml` | Runtime-informational provenance: embedder block, full `training` block (knobs incl. per-cluster `alpha`/`alpha_floor` vectors, `recommended_ui_defaults`, `roster_edit`), deployed providers/models, `cost_per_1k_*`, changelog. "Does not affect routing decisions" | `internal/router/cluster/AGENTS.md:40–42`; `internal/router/cluster/artifacts.go:106–118` |
| `rankings.json` | v1-only baked score table. **Intentionally absent in v0.76/v0.77** (stale 18-model data; v2 loader ignores it) | `internal/router/cluster/artifacts/v0.76/metadata.yaml:138`; `internal/router/cluster/artifacts/README.md:106–110` |

Runtime knobs: v2 means α, speed_weight, output_cost_ratio, expected_output_tokens, per_model_verbosity are **reconstructable at request time** (overridable via `x-weave-routing-*` headers) instead of baked into rankings — that is precisely why overlays can change rosters without retraining (`internal/router/cluster/artifacts/README.md:25–38`).

Per-cluster winners in the current geometry (16 clusters): hard `{0,13}` → kimi-k3, medium `{1,3,4,5,6,7,8,12,14}` → motif-3, conversational `{2,9,10,11,15}` → deepseek-v4-flash (`internal/router/cluster/artifacts/README.md:108–110`; `v0.77/metadata.yaml:137–140`).

---

## How v0.75/v0.76/v0.77 were produced

### v0.75 — roster swap on frozen geometry (parent v0.73)

- `parent: v0.73`, `status: candidate` (`internal/router/cluster/artifacts/v0.75/metadata.yaml:1–4`).
- **Delta**: claude-opus-4-8 retired, claude-opus-5 added in its place. `centroids.bin BYTE-IDENTICAL to v0.73 (no retrain, no re-cluster)`. claude-opus-5's quality column was a **manual clone of claude-fable-5's per-cluster cells** ("make opus 5 similar to fable"), priced at opus-5's real catalog rate. `rankings.json` was regenerated by the runtime blend over the new 18-model roster, **parity-gated**: the same blend had to reproduce v0.73 bit-for-bit on the unchanged roster first. No probe run, no bake-off — explicit direction ("Candidate; latest bump is a separate step") (`v0.75/metadata.yaml:157–163`).
- Roster: 18 models across 6 providers, incl. `z-ai/glm-5.2`, `moonshotai/kimi-k2.7`, `deepseek/deepseek-v4-flash` (`v0.75/metadata.yaml:112–137`).
- Its own changelog also carries the inherited story of the first frozen-geometry overlay (v0.70): the training corpus was **no longer reproducible** ("OpenRouterBench source returns 0 rows, routerarena label file drifted"), and retraining churned geometry in ways that regressed hard-agentic solve ~13pt — hence the standing decision to freeze geometry and calibrate new models onto the existing `q_norm` scale via anchors (`v0.75/metadata.yaml:166–179`).

### v0.76 — five-model managed-upstream-only tier bundle (parent v0.75)

Produced entirely by `scripts/build_v076_aiand_overlay.py`:

1. **Inputs**: v0.75's `model_registry.json`, `quality_means.json`, `model_axes.json`, `model_features.json` + `scripts/motif_probe_results.json` (the probe gate input) (`scripts/build_v076_aiand_overlay.py:2–14,112–127`).
2. **Gate before build**: `read_m3_win()` exits with "do not build. Report to human" unless the motif probe result has `go_no_go == "PASS"` (`scripts/build_v076_aiand_overlay.py:82–90`).
3. **Geometry**: `centroids.bin` copied byte-identical (`scripts/build_v076_aiand_overlay.py:121–122`).
4. **Roster**: filter v0.75 to 3 renamed survivors (kimi-k2.7 → kimi-k2.7-code exact upstream ID), add `moonshotai/kimi-k3` + `motif-technologies/motif-3`; all entries forced to `provider: "aiand"` (`scripts/build_v076_aiand_overlay.py:21–28,129–141`).
5. **Quality**: for each of the 16 clusters, `pool_max = max(existing models' q)`, then `kimi-k3 = pool_max × 1.03` on hard clusters `{0,13}` (× 0.97 elsewhere), `motif-3 = pool_max × Q_M3_WIN` (probe-derived, landed at 1.1) on medium `{1,3,…,14}` (× 0.92 elsewhere) (`scripts/build_v076_aiand_overlay.py:31–37,93–98,143–154`).
6. **Registry proxy notes**: kimi-k3 borrows bench column `routerarena_moonshotai/kimi-k2.6`; motif-3 borrows `routerarena_z-ai/glm-5` — both flagged `proxy: true` with a note saying why (`scripts/build_v076_aiand_overlay.py:49–56`).
7. **Costs**: pinned from the managed-upstream catalog for all 5 models into `model_axes.json`, `model_features.json`, `quality_means.meta`, and `metadata.yaml` (`scripts/build_v076_aiand_overlay.py:40–46,156–158,166–216`).
8. **Self-check before write**: each tier leader must be *strictly higher* than every other model on its clusters, else the script aborts (`scripts/build_v076_aiand_overlay.py:227–240`).
9. **metadata.yaml**: version/parent/status updated, `deployed_providers: ["aiand"]`, changelog written (`scripts/build_v076_aiand_overlay.py:249–272`).

Result metadata: five deployed models, `roster_edit.added = [kimi-k3, motif-3]`, changelog stating "Three-tier per-cluster quality: kimi-k3 wins HARD_K3 {0,13} at 1.03x maxpool; motif-3 wins MID_M3 … at 1.1x maxpool (30-prompt probe); flash wins FLASH_5 {2,9,10,11,15} … NOT promoted to latest." (`internal/router/cluster/artifacts/v0.76/metadata.yaml:119–145`).

### v0.77 — six-model bundle: glm-5.3 replaces glm-5.2, qwen3.8-27b joins (parent v0.76)

Produced by `scripts/build_v077_aiand_overlay.py`, same mechanical shape:

1. **Inputs**: v0.76 artifacts + two probe result files — `scripts/glm53_probe_results.json` (mandatory; `go_no_go == "PASS"` or "do not build") and `scripts/motif_probe_results.json` (re-read for Q_M3_WIN so the motif tier stays bit-identical) (`scripts/build_v077_aiand_overlay.py:19–22,92–107,120–147`).
2. **Factors from probes**: `q53_on = recommended_q_glm53_on` (capped at 1.12 by the probe script, floored at 1.02; the probe's raw pass-rate ratio saturated at 1.0 so the shipped factor 1.12 came from the AA Index ratio 60/53 = 1.132, per the changelog), `q53_off = 0.97`, `q_qwen38_off = 0.92` clamped to `[0.40, 0.92]` (`scripts/build_v077_aiand_overlay.py:54–55,131–147`; `scripts/probe_glm53_medium.py:283–298`; `v0.77/metadata.yaml:141–142`).
3. **Quality math per cluster**: `glm-5.3 = glm-5.2's own value × (on-factor if hard cluster or glm-5.2 was the cluster max, else off-factor)`; `qwen3.8-27b = pool_max × q_qwen38_off`; both **clamped to 0.99× the tier leader** if the scaled value would overtake it, so tier winners never flip (`scripts/build_v077_aiand_overlay.py:111–117,207–239`).
4. **Registry**: glm-5.3 proxies `routerarena_z-ai/glm-5.2`; qwen3.8-27b proxies `routerarena_qwen/qwen3.6-27b`; kept models re-noted "v0.77 overlay: kept from v0.76" (`scripts/build_v077_aiand_overlay.py:28–48,176–205`; verified values in `internal/router/cluster/artifacts/v0.77/model_registry.json`).
5. **Axes honesty rule**: models with no measurement ship `tps/ttft_s/verbosity_tokens: null` — "nulls are honest"; only deepseek-v4-flash carries measured `tps: 98.429, ttft_s: 0.797` inherited from older bundles (`scripts/build_v077_aiand_overlay.py:255–269`; verified in `v0.77/model_axes.json`).
6. **Same strict tier-leader self-check** + same byte-identical `centroids.bin` copy (`scripts/build_v077_aiand_overlay.py:168–169`, self-check verified in the tail of `main()`).
7. **Changelog**: "glm-5.3 quality = glm-5.2's per-cluster values x probe factor (1.12 on hard clusters {0,13}, 0.97 elsewhere; AA Index ratio 60/53 = 1.132 capped to 1.12). qwen3.8-27b quality = pool max x 0.92 everywhere (TierLow pool model, wins no clusters) … NOT promoted to latest." (`internal/router/cluster/artifacts/v0.77/metadata.yaml:137–146`).

Full runbook for the v0.77 change: `docs/adding-glm-5-3.md:126–158` (bundle steps + the four test commands validating catalog, bundle load, scorer argmax, prices).

### Named failed/alternative candidate: `candidate-k12`

`internal/router/cluster/artifacts/candidate-k12/metadata.yaml` shows what a *real* retrain candidate looks like: "K=12 re-cluster on real prod turns (embed_input geometry) with quality from our OWN in-harness execution grades (harness-faithful mine_bakeoff), NOT AA/DeepSWE imputed priors" — 146 faithful cells + 94 faithful-shrunk + 12 shrunk (lines 1–5, 97–101). "Bake-off candidate for a 3-way live comparison vs v0.72. NOT promoted; artifacts/latest is unchanged" (lines 111–112). It demonstrates the孪生 promotion mechanics that never fired for it: "Servable on a staging router (`ROUTER_CLUSTER_BUILD_ALL_VERSIONS=true`) via `x-weave-cluster-version: candidate-k12`" (line 112).

---

## Where training actually happens

**Not in this repo.** Evidence chain:

- The trainer is `train_cluster_router.py`; the layout rules say it "always writes to `artifacts/v<X.Y>/` and never overwrites a previous version (auto-bumps from `latest` when `--version` is omitted)" and "Never edit `centroids.bin` / `rankings.json` by hand" — a noqa-style contract aimed at a script this repo does not contain (`internal/router/cluster/AGENTS.md:38`).
- Its location is stated explicitly: "`train_cluster_router.py` (sibling `router-internal/eval/`) must embed training prompts identically to how the Go runtime embeds requests, or the bundle silently misroutes" (`internal/router/cluster/artifacts/README.md:65–70`; agnostic org name per the no-org-names rule at `AGENTS.md:217`).
- The eval harness is likewise external: "sibling Poetry package, **not in this repo** — lives at `router-internal/eval/` … and runs as a Modal app" (`AGENTS.md:236–238`). The 1000-prompt v2-diff corpus itself was generated externally: "Regenerate via `poetry run python regen_diff_corpus.py --n 1000 --seed 42 --routerarena-only` (from router-internal/scripts)" (`internal/router/cluster/diff_v2_test.go:14–18`).
- Grep across `scripts/`, `docs/`, `internal/` for `train_cluster_router` / kmeans / trainer finds no training implementation — only references (loader error text `internal/router/cluster/artifacts.go:509`, AGENTS/README rules, prices dict note at `AGENTS.md:70`). A glob for `**/train_cluster_router.py` and `**/regen_diff_corpus.py` returns nothing. There is no `docs/plans/` scorer-training/tunable-knobs doc in this checkout (`artifacts/README.md:38` references `docs/plans/ROUTER_RUNTIME_TUNABLE_KNOBS.md`, which does not resolve here; the referenced `CLUSTER_ROUTING_PLAN.md` lives under `docs/plans/archive/` in the wider monorepo per `internal/router/cluster/CLAUDE.md:5`).

### What must be handed over to retrain (v0.78 with true re-cluster)

The trainer, as invoked by past versions, consumed (from `training:` blocks, identical across v0.75–v0.77 at `v0.76/metadata.yaml:9–59` / `v0.77/metadata.yaml:9–59`):

| Input | Filename as recorded | In this repo? | Known format |
|---|---|---|---|
| RouterArena per-prompt labels | `routerarena_labels_combined.jsonl` (`v0.76/metadata.yaml:26`) | **No** — the file does not exist here; only historic metadata mentions it (older long-form path `eval/results/_unsorted/routerarena_labels_full.jsonl` at `artifacts/legacy/v0.22/metadata.yaml:22`, produced by `routerarena_label.py` per `artifacts/legacy/v0.21/model_registry.json:3`) | Not directly visible; legacy registries describe it as "per-prompt RouterArena coverage" with one column per labeled model, named `routerarena_<model>` (the bench_column convention, `internal/router/cluster/artifacts.go:77–85`) |
| AA quality priors | `aa_quality_priors_v0.64.json` (`v0.76/metadata.yaml:35`) | **No** — glob finds no `aa_quality_priors*` anywhere; only `metadata.yaml` records the name. Older bundles used `aa_quality_priors.json` (`artifacts/legacy/v0.54/metadata.yaml:31`, burned-in per version at `v0.56`) | Not directly visible. Its *effect* is recorded: per-source tier weights (`aa_label_tier_weights`, e.g. `SWE_BENCH_VERIFIED: 1.0`, `BFCL_V4_SIMPLE: 0.3`), `aa_evidence_scale: 3.0`, and residual stats `n_cells: 178, mean: 0.0567, p90: 0.153` (`v0.77/metadata.yaml:36–59`; also baked into `quality_means.json:meta`) |
| Benchmark prompt corpus | loaders `aider-polyglot-go`, `aider-polyglot-rust-cpp-java`, `livecodebench-ts-and-aider-polyglot-ts`, `bfcl-v4-simple`, `bfcl-v4-parallel-multi`; `source_fractions: {routerarena: 0.1}`; `n_prompts: 1775`; `training_data_mix: {d1: 1.0}` (`v0.76/metadata.yaml:30–33,17–20`) | **No** — loaders are trainer-side code | Loader names + mix fractions only |
| Geometry corpus + hyperparams | k=16, top_p=4, alpha=0.96, shrinkage_k0=10.0, `score_normalization: per_prompt_zscore_across_bench_columns`, seed=42 (`v0.76/metadata.yaml:10–17`) | N/A | Hyperparams visible; corpus is the missing piece (and per the v0.70 changelog, the historical corpus already broke reproducibility once, `v0.75/metadata.yaml:167–170`) |

Note the corpus drift risk is a **documented precedent**: "v0.68's training corpus is no longer reproducible … a rejected retrain regressed hard-agentic solve ~13pt purely from cluster movement" (`v0.75/metadata.yaml:167–172`) — this is why frozen-geometry overlays became the default path, and why v0.78 should stay on that path unless the full corpus can be handed over.

---

## Measuring new per-model scores on a budget

### The probe pattern (in-repo, fully reproducible)

All probes share one shape — small fixed prompt set, sent through the managed upstream's OpenAI-compatible `/v1/chat/completions` at `temperature: 0.0` / `max_tokens: 2000`, scored pass/fail on a deterministic substring (function name or computed answer token), then converted to a **multiplier on the pool max** rather than an absolute score:

| Script | Corpus | Models | Pass criterion | Budget (docstring) | Output factor |
|---|---|---|---|---|---|
| `scripts/probe_hard_tier.py:1–16` | 30 HARD agentic/multi-step problems; model must end with `@@P<NN>=<answer>` — "code-unfriendly token that cannot appear in valid Python" so pass/fail is deterministic and measure correctness, not name-guessing (`probe_hard_tier.py:20–24`) | glm-5.2 vs kimi-k3 vs kimi-k2.7-code | exact answer token | ~$2–4 | `ratio_glm_vs_k3 = pass_glm/pass_k3`; ≥0.97 → GLM_HARD_TIER, <0.97 → K3_HARD_TIER (`probe_hard_tier.py:8–12`). Result committed: kimi-k3 25/30 vs glm 17/30 → `go_no_go: K3_HARD_TIER` (`scripts/hard_tier_probe_results.json`) |
| `scripts/probe_kimi_k3_quality.py:2–9` | ~100 coding prompts (40 Python / 25 Go / 20 JS / 15 more), pass if `test` name appears in response (`probe_kimi_k3_quality.py:255`) | kimi-k3 + 3 incumbents | substring | ~$10 (100 × 4 × ~2k tok) | `ratio = kimi_rate / pool_max`; recommended = `min(1.03, 1+0.5(ratio−1))` if ≥1 else `max(0.98, ratio)` (`probe_kimi_k3_quality.py:287–302`). Committed: kimi 100/100, pool max 0.99 → multiplier 1.005 → tier table landed at 1.03/0.97 (`scripts/probe_results.json` pass rates) |
| `scripts/probe_motif3_medium.py:2–9` | 30 medium coding prompts | motif-3 vs glm-5.2 vs kimi-k2.7-code | substring | ~$1–3 | `recommended_m3_win`. Committed: motif 30/30 vs glm 27/27 → ratio 1.111 → `recommended_m3_win: 1.1`, `go_no_go: PASS` (`scripts/motif_probe_results.json:19–21,534–538`) |
| `scripts/probe_glm53_medium.py:2–18` | the **same 30 medium prompts** reused from the motif probe, now 5 models; errored calls retried ≤3× and excluded from denominators (`docs/adding-glm-5-3.md:117–122`; error-aware pass rates at `probe_glm53_medium.py:246–267`) | glm-5.3, kimi-k3, motif-3, flash, qwen3.8-27b | substring | ~$3–6 | `go_no_go` PASS iff glm-5.3 ≥ kimi-k3 (`:283`); `recommended_q_glm53_on = clamp(raw_ratio, 1.02, 1.12)`, `qwen38_off = clamp(raw, 0.40, 0.92)` (`:283–298`). Committed: all leaders 100%, ratios 1.0 → `on: 1.02, off: 0.97, qwen38_off: 0.92`, PASS, `n_prompts: 30`, `test_date: 2026-08-29` (`scripts/glm53_probe_results.json`) |

Probe results file → overlay factor is the whole handoff: `build_v076_aiand_overlay.py` refuses to run unless `motif_probe_results.json` says PASS (`scripts/build_v076_aiand_overlay.py:82–90`); `build_v077_aiand_overlay.py` does the same for both probe files and carries `n_prompts`/`test_date` into the registry proxy notes (`scripts/build_v077_aiand_overlay.py:131–156`).

### Token-arithmetic cost estimate for re-measuring 6 models over the RouterArena 1000

The candidate corpus exists in-repo: `internal/router/cluster/testdata/diff_v2_prompts.jsonl` — 1000 routerarena prompts, schema `{"prompt_id", "source", "text"}` (verified; SHA pinned in CI at `internal/router/cluster/diff_v2_test.go:14–22`). Scoring them needs *answers*, which RouterArena provides for labeled models only — so a probe over the 1000 means **generating** 1000 × 6 = 6000 completions and grading. Cost = Σ over models of `(input_tok/1M × $in + output_tok/1M × $out)` at v0.77/pinned catalog prices (`v0.77/metadata.yaml:130–153`; `docs/adding-glm-5-3.md:12–19`):

- Per model, worst case ~0.5k input token + 2k output per prompt (probe scripts set `max_tokens: 2000`): glm-5.3 ≈ $0.0085, kimi-k2.7-code ≈ $0.0074, kimi-k3 ≈ $0.0265, motif-3 ≈ $0.0043, qwen3.8-27b ≈ $0.0062, flash ≈ $0.0006 → **≈ $0.053/prompt × 1000 ≈ $53 upper bound**; realistically 2–5× less (typical probe responses ran far below the 2k cap; committed probes show ~$0.5–2k-token responses at ~$1–10 total for similar call counts and this is the same passage pattern as the $10 / 400-call kimi-k3 probe — `[INFERENCE]` on realized rate, arithmetic above is exact).
- **Targeted-probe alternative (recommended, matches precedent)**: don't re-measure the incumbents. Measure only what changed — for v0.78 with the six-model roster already in v0.77, nothing needs probing; a roster *delta* needs one 30-prompt medium probe + one 30-prompt hard probe per newcomer (~$2–6 per script run, per the budgets above), exactly the pattern that produced v0.76/v0.77 quality factors for ≤ $10 total per release.

---

## Constraints & promotion path

### Loader contract (what makes a bundle legal to serve)

- **Embedder pinning**: `NewScorer` refuses a bundle whose declared `embedder.model`/`embed_dim` differs from the runtime embedder — "dim alone doesn't guard this since dims can collide" (`internal/router/cluster/scorer.go:120–127`; bundle declares `jina-v2-base-code-int8`, 768, max_tokens 256 at `v0.77/metadata.yaml:5–8`). Trainer-side embedding (model, pooling, L2, no instruction prefix, tail truncation) must match runtime exactly (`internal/router/cluster/artifacts/README.md:65–89`).
- **`MaxPromptChars = 1024`** default (`internal/router/cluster/scorer.go:39–46`), tail-truncate with UTF-8 boundary snapping, mirroring the Python embedding path byte-for-byte (`scripts/embed_register_probes.py:55–71`). Loosening the cap requires re-running the latency test — "BERT inference is O(n²) attention; the cap is load-bearing" (`internal/router/cluster/AGENTS.md:75`).
- **Latency gate / fail-closed**: embeds are raced against `EmbedTimeout: 1500ms` (`scorer.go:39–46,450–456`); every failure path returns `ErrClusterUnavailable` → HTTP 503 — "no fail-open fallbacks; the previous heuristic fallback was removed because it silently degraded routing" (`internal/router/cluster/AGENTS.md:78`). Promoting any bundle with a *new embedder* requires a p95 embed measurement against the gate first (`internal/router/cluster/AGENTS.md:76`; `artifacts/README.md:86–89`) — irrelevant for v0.78 on the same Jina embedder.
- **Candidate promotion (multi-version serving)**: prod builds only the default version; `ROUTER_CLUSTER_BUILD_ALL_VERSIONS=true` builds one Scorer per committed bundle, enabling per-request pinning via `x-weave-cluster-version: v0.X` (middleware `WithClusterVersionOverride`) — the bake-off mechanism used for staging/eval (`internal/router/cluster/AGENTS.md:30–34`; `AGENTS.md:240–243`; `cmd/router/main.go:961–964`). `ROUTER_CLUSTER_VERSION` env selects the default at boot (`cmd/router/main.go:336,955`).

### Promotion conventions

- The pointer file is a single line: `internal/router/cluster/artifacts/latest` currently contains `v0.77`. "Promotion = one-line edit to `latest` + redeploy. Don't bypass the version pointer" (`internal/router/cluster/AGENTS.md:26–27,81`).
- The two quoted changelog promotion lines the ticket asks for:
  - v0.76: "…5-model ai&-only registry. **NOT promoted to latest.**" (`internal/router/cluster/artifacts/v0.76/metadata.yaml:139`) — later flipped by the dedicated commit `feat(cluster): promote v0.76 ai&-only bundle to latest` (`061cfe1a`), documented in `internal/router/cluster/artifacts/README.md:106–108` ("the `latest` pointer resolves to it: this deployment is ai&-only, so v0.76 is the default").
  - candidate-k12: "**NOT promoted**; artifacts/latest is unchanged. Servable on a staging router (`ROUTER_CLUSTER_BUILD_ALL_VERSIONS=true`) via `x-weave-cluster-version: candidate-k12`" (`internal/router/cluster/artifacts/candidate-k12/metadata.yaml:5,111–112`).
- v0.77 followed the identical recipe: shipped as candidate with changelog "…6-model ai&-only registry. NOT promoted to latest." (`v0.77/metadata.yaml:146`), then promoted via `latest` edit in commit `e13b1d52` ("Sync AIand dashboard metrics cluster artifacts to v0.77").
- Bundle immutability: never overwrite a committed version directory; new version = new dir (`internal/router/cluster/AGENTS.md:80`). `metadata.yaml` is informational; only `model_registry.json` is hand-editable (`internal/router/cluster/AGENTS.md:38,40–42`).
- Offline pre-promotion review exists in-repo: `cmd/routing-report` routes the 180-prompt × 6-register labeled corpus (`testdata/register_probes.jsonl`, registers `conversational/trivial_nl/knowledge_qa/easy_code/hard_code/agentic_tool`) through the real Scorer using precomputed embeddings (`cmd/routing-report/main.go:1–43`); the v2-vs-v1 release gate routes the 1000-prompt fixture through both blend paths asserting ≥99% top-1 agreement (`internal/router/cluster/diff_v2_integration_test.go:23–45`).

---

## Open questions

1. **`aa_quality_priors_v0.64.json` format is unverifiable in-repo.** Its schema (per-model × per-source cells? raw pass rates?) must be recovered from the training repo before any real retrain; only its *fnarp name*, tier weights, evidence scale, and residual stats are recorded (`v0.77/metadata.yaml:35–59`).
2. **`routerarena_labels_combined.jsonl` location in the training repo** is unknown here; also note the documented precedent of the label file "drifting" and breaking corpus reproducibility (`v0.75/metadata.yaml:167–170`). Any v0.78 retrain must re-hash/verify it.
3. **`d1/d2/d3` training_data_mix semantics** are not documented in this repo (values `d1: 1.0` at `v0.76/metadata.yaml:18–20`, consumed only as opaque floats by the v1 rankings meta parser at `internal/router/cluster/artifacts.go:68–72`). Definition lives with the trainer.
4. **v0.78 roster is already served**: v0.77's `deployed_models` is exactly the requested six (`v0.77/metadata.yaml:123–129`). If v0.78 means "same roster, better numbers," the missing inputs are fresh probe runs, not training data; if it means re-clustering, the trainer + the four table-row inputs above must be handed over.
5. **Metadata promotion inconsistency**: v0.77's `metadata.yaml` still says `status: candidate` and "NOT promoted to latest" even though `artifacts/latest` contains `v0.77` (commit `e13b1d52`) — worth normalizing in a v0.78 run (pattern: add `status: latest`/`promoted_date`, which the parser already supports at `internal/router/cluster/artifacts.go:112–113`).
6. **`docs/plans/` referenced docs are missing from this checkout** (`artifacts/README.md:38` → `docs/plans/ROUTER_RUNTIME_TUNABLE_KNOBS.md`; `internal/router/cluster/CLAUDE.md:5` → `docs/plans/archive/CLUSTER_ROUTING_PLAN.md`) — recoverable only from the wider monorepo.
7. **Latency/latency-proxy axes are mostly `null`** in v0.77 (only flash carries measured `tps`/`ttft_s`; verified in `v0.77/model_axes.json`) — speed-axis features for the other five models are inherited-null, so any speed-sensitive knob assessment would need fresh measurement or stays inert at `speed_weight: 0` (`v0.77/metadata.yaml:23,95`).
