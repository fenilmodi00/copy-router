# v0.79 promotion runbook

Runbook for promoting `artifacts/v0.79` (RouterArena measured-label
bundle, built by `scripts/build_v079_aiand_measured.py`, parent `v0.78`,
same 8-model AIand roster, centroids byte-identical to v0.78) once the
bundle is committed. Promotion mechanics per
`internal/router/cluster/CLAUDE.md` ("Promotion = one-line edit to
`latest` + redeploy") and `internal/router/cluster/artifacts/README.md`
("Promote a candidate by editing `latest` to its name and redeploying").

Current state: `internal/router/cluster/artifacts/latest` contains
`v0.78`. The v0.79 builder writes `artifacts/v0.79/` and deliberately
does **not** touch `latest` — the pointer edit below is the promotion.

The 8-model roster (must appear verbatim in every verification below):

```
deepseek-ai/deepseek-v4-pro        zai-org/glm-5.3
deepseek-ai/deepseek-v4-flash      zai-org/glm-5.3-flash
deepseek-ai/deepseek-v4.1-flash    qwen/qwen3.8-27b
                                   moonshotai/kimi-k3
                                   motif-technologies/motif-3
```

## 1. Pre-flight gates

Run all of these **before** touching `latest`:

1. **Full cluster package suite:**

   ```bash
   go test ./internal/router/cluster/...
   ```

   (Contributors without `libonnxruntime` add `-tags no_onnx`; the
   production Dockerfile builds with `-tags ORT` — do not drop that tag
   from any production-bound build.)

2. **`TestV079BundleLoads` unskipped and green**
   (`internal/router/cluster/artifacts_v079_test.go`). It skips while
   `artifacts/v0.79` is absent from the embedded tree, so confirm it
   actually ran:

   ```bash
   go test ./internal/router/cluster/ -run TestV079BundleLoads -v
   ```

   Must print `--- PASS: TestV079BundleLoads`, **not** `--- SKIP`. It
   pins: v2 bundle parses, `version: v0.79`, `parent: v0.78`, K=16,
   exactly 8 registry entries all on provider `aiand`, bench columns
   carry `routerarena_measured`, exactly 8 candidates under both the
   aiand-only boot and the full-provider boot, centroids byte-identical
   to v0.78, and the dropped models (`glm-5.2`, `kimi-k2.7-code`,
   `qwen3.6-27b`, `gemma-4-31b-it`, `gpt-oss-120b`) absent.

3. **`cmd/routing-report` on the candidate** (`cmd/routing-report/main.go`,
   runs under `-tags no_onnx` with the precomputed probe embeddings, so
   the blend/normalization/eligibility path is exact production code):

   ```bash
   go run -tags no_onnx ./cmd/routing-report --target v0.79 --baseline v0.78
   ```

   Review the per-register routing diff before promoting. The probe
   embedding cache stays valid: v0.79 keeps the
   `jina-v2-base-code-int8` embedder, and `routeCorpus` hard-fails on
   an embedder ID/dim mismatch with the cache.

4. **Offline label comparison** (optional but cheap):

   ```bash
   python scripts/v079_comparison.py
   ```

   Reports per-cluster within-AIand winner changes and per-model
   average-rank deltas vs v0.78.

## 2. Promote: the `latest`-pointer edit

1. Edit `internal/router/cluster/artifacts/latest` — a single-line file —
   from `v0.78` to:

   ```
   v0.79
   ```

   Do not edit `centroids.bin`, `rankings.json`, `quality_means.json`,
   or `model_axes.json` by hand; `model_registry.json` is the only
   hand-editable file inside a bundle. Never overwrite a previously
   committed version directory.

2. Re-run the pointer-gated tests before committing the edit:

   - `TestResolveVersion_Latest` (`internal/router/cluster/artifacts_test.go`)
     — catches a typo'd pointer: it resolves `latest` and asserts the
     result names a committed version directory in `ListVersions()`.
   - `TestEmbeddedArtifacts_AllVersionsLoadable`
     (`internal/router/cluster/artifacts_test.go`) — loads **every**
     committed bundle end-to-end, v0.79 included.

3. Commit the pointer edit and redeploy (Dockerfile builds with
   `-tags ORT`).

## 3. Post-promotion verification

1. **Boot logs** (`cmd/router/main.go`) must show v0.79 with the 8
   roster models:

   - `Cluster scorer version built` with `cluster_version=v0.79`,
     `embedder=jina-v2-base-code-int8`, and `models=` listing exactly
     the 8 roster models above (`main.go:1764`).
   - `Cluster multiversion router ready` with `default_version=v0.79`
     (`main.go:1777`).
   - `Routing via cluster scorer` (`main.go:539`).
   - Absence of `Cluster scorer failed to build; refusing to boot`
     (`main.go:536`) — a failed build panics rather than degrading.

2. **Live routed completion:** send a real request through
   `POST /v1/chat/completions` (or `/v1/messages`) and confirm the
   response `model` is one of the 8 roster models.

3. **BYOK-wall check:** an installation that holds a vendor BYOK key
   (e.g. an Anthropic key) must still route only to the 8 roster
   models — the bundle's model registry gates the candidate pool before
   provider eligibility is consulted, so a vendor BYOK header cannot
   widen it (`docs/CONFIGURATION.md`, "AIand-only deployment"; pinned by
   `TestV079BundleLoads`' full-provider boot, which asserts exactly 8
   candidates with every provider registered). To block per-request
   vendor BYOK widening outright, set `ROUTER_EXCLUDED_PROVIDERS` to the
   comma list of the 16 non-aiand providers (`.env.example`).

## 4. Rollback

1. Repoint `internal/router/cluster/artifacts/latest` back to `v0.78`
   and redeploy.
2. Without a redeploy, `ROUTER_CLUSTER_VERSION=v0.78` pins the served
   version per-deployment (`docs/CONFIGURATION.md`; `cmd/router/main.go`
   `buildClusterScorer` reads it, defaulting to `latest`).
3. Keep `artifacts/v0.79` committed — versions are frozen for
   comparison; deleting it would also break `TestV079BundleLoads` and
   the eval harness's per-request `x-weave-cluster-version` override.

## Appendix: version-enumerating tests (cross-check)

Does any test need updating when a new version directory appears?
**No** — every version-enumerating test in the cluster package iterates
`cluster.ListVersions()` or resolves `latest` dynamically, so a committed
`artifacts/v0.79` is picked up automatically:

| Test (all in `internal/router/cluster/`) | What it does | Update needed for v0.79? |
| --- | --- | --- |
| `TestEmbeddedArtifacts_AllVersionsLoadable` (`artifacts_test.go`) | Iterates `ListVersions()`, loads every committed bundle end-to-end | No — v0.79 joins the loop automatically |
| `TestResolveVersion_Latest` (`artifacts_test.go`) | Resolves `latest`, asserts it names a directory in `ListVersions()` | No — no hardcoded version; this is the typo-catcher for the pointer edit itself |
| `TestListVersions_FlattensLegacyAndOmitsPseudoName` (`artifacts_test.go`) | Enumerates all versions; asserts no `legacy` pseudo-name leaks and `v0.21` stays reachable | No |
| `TestLatestBundle_OneDeployedModelPerFamily` (`artifacts_test.go`) | Loads whatever `latest` points at; one model per family | No — its accepted-variant exemption map already covers the `deepseek-v4-flash` / `deepseek-v4.1-flash` pair v0.79 keeps |
| `TestFastestModel_RealLatestBundle_LowTierPrefersFastFlash` (`artifacts_test.go`) | Loads `latest`; asserts the low-tier clamp picks `gemini-3.1-flash-lite-preview` (google) over `deepseek/deepseek-v4-flash` | No — **skips** while the bundle carries no `tok_per_s` annotations in `metadata.yaml`; neither v0.78 nor the v0.79 builder (`scripts/build_v079_aiand_measured.py`) writes `tok_per_s`, so it stays skipped. Re-pin its hardcoded model names if a future bundle ever adds `tok_per_s` |
| `TestV079BundleLoads` (`artifacts_v079_test.go`) | v0.79-specific guard: skips until `artifacts/v0.79` exists, then pins the 8-model roster, geometry, and measured bench columns | Already written; flips from SKIP to PASS when the bundle lands |

Not version-enumerating: `TestV2MatchesV1`
(`internal/router/cluster/diff_v2_integration_test.go`) is gated on
`DIFF_V2_BUNDLE_DIR` and only runs under the `diff_v2_vs_v1.py`
driver. `cmd/router/main.go` `buildClusterScorer` enumerates versions
only when `ROUTER_CLUSTER_BUILD_ALL_VERSIONS=true` (staging/eval
A/B); prod loads the served default only.

Housekeeping in the promotion PR: `docs/CONFIGURATION.md`'s
"AIand-only deployment" section names `artifacts/v0.78` as where the
roster enforcement lives — re-point that reference to `artifacts/v0.79`.
