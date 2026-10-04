#!/usr/bin/env python3
"""Build artifacts/v0.77: AIand roster re-anchored on RESEARCHED benchmarks.

Same frozen-geometry overlay as v0.76 (centroids byte-identical to v0.75;
runtime blend regenerated with the same parity gate), with two upgrades:

1. Quality columns for the 13 AIand models are no longer scale-guess clones.
   Each cluster gets a least-squares regression from AA Intelligence Index
   v4.3.2 to the frozen q_norm cell, fit over the 18 v0.75 roster models
   (AA span 9-51: interpolation, not extrapolation). Every AIand model's
   researched score (AA index, blended 50/50 with SWE-bench Verified where
   independently measured) maps through its cluster's regression, clamped
   to the cluster's existing [min,max]. Slope is floored at 0, so a higher
   researched score can never map below a lower one.

2. model_axes TTFT/TPS carry the AA-published per-model medians (v0.76
   cloned the source model's axes; AA numbers differ by up to 5x).

Research data: .context/bench-scores.md (13 AIand models) and
.context/roster-aa-indexes.md (18 roster anchors), both retrieved
2026-10-04 from artificialanalysis.ai (markdown scrape).

Usage: python scripts/build_v077_aiand_researched.py
Requires: PyYAML. Deterministic.
"""

import hashlib
import json
import shutil
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
ART = ROOT / "internal" / "router" / "cluster" / "artifacts"
SRC = ART / "v0.76"
DST = ART / "v0.77"

# AIand model -> (researched quality, live-probed prices, AA-published
# latency where available). aa_idx: AA Intelligence Index v4.3.2
# (max/reasoning variant where published; motif-3 is AA's own estimate).
# swebench_v: SWE-bench Verified % (independent runs preferred; None when
# no independent measurement exists).
AIAND_RESEARCHED = [
    # model                              aa_idx  swebench_v  in/1M  out/1M  ttft_s  tps    note
    ("zai-org/glm-5.2",                 34, 82.8, 1.00, 4.00, 2.99,  93.7,
     "Twin of z-ai/glm-5.2 (same weights); AA 34; SWE-V 82.8 INDEP (Vals)."),
    ("zai-org/glm-5.3",                 45, 95.4, 1.00, 4.00, 2.84,  71.4,
     "AA 45; SWE-V 95.4 INDEP (Vals rank 6); strongest AIand coder by index."),
    ("zai-org/glm-5.3-flash",           42, 92.0, 0.15, 0.50, 3.31,  53.2,
     "AA 42; SWE-V 92.0 INDEP; near-frontier quality at flash pricing."),
    ("deepseek-ai/deepseek-v4-flash",   34, 88.8, 0.15, 0.25, 1.07, 214.6,
     "Twin of deepseek/deepseek-v4-flash; AA 34; SWE-V 88.8 INDEP; fastest lane."),
    ("deepseek-ai/deepseek-v4.1-flash", 39, None, 0.30, 0.60, 1.09, 213.3,
     "AA 39; SWE-V not independently measured (released after Vals board archive)."),
    ("deepseek-ai/deepseek-v4-pro",     36, 96.4, 1.00, 2.50, 1.72, 115.8,
     "Twin of deepseek/deepseek-v4-pro; AA 36; SWE-V 96.4 INDEP (Vals rank 2)."),
    ("moonshotai/kimi-k2.7-code",       26, 78.2, 0.75, 3.50, 2.94,  96.5,
     "AA 26; SWE-V 78.2 INDEP; weakest coder of the kimi pair."),
    ("moonshotai/kimi-k3",              44, 93.4, 3.00, 12.50, 4.06,  40.2,
     "AA 44; SWE-V 93.4 INDEP (rank 8); premium lane, slow (40 TPS, 4.1s TTFT)."),
    ("qwen/qwen3.6-27b",                21, 70.0, 0.32, 3.20, 3.60,  56.5,
     "AA 21; SWE-V 70.0 INDEP; low-capability 27B lane."),
    ("qwen/qwen3.8-27b",                34, 86.0, 0.40, 3.00, 3.79,  45.8,
     "AA 34 (xhigh variant); SWE-V 86.0 INDEP (Vals rank 15)."),
    ("motif-technologies/motif-3",      34, 76.2, 0.50, 2.00, None, None,
     "AA 34 is AA's own estimate (no measured run); SWE-V 76.2 SELF only."),
    ("google/gemma-4-31b-it",           15, None, 0.20, 0.50, 1.12,  35.6,
     "AA 15; only subset SWE numbers published; weak coding signal."),
    ("openai/gpt-oss-120b",             12, None, 0.15, 0.60, 0.83, 181.5,
     "AA 12; SWE-V harness-limited (62.4 SELF high); very fast (181 TPS)."),
]

# AA Intelligence Index v4.3.2 for the 18 v0.75 roster models (their frozen
# per-cluster quality cells are the regression's y values). Sources:
# .context/roster-aa-indexes.md (retrieved 2026-10-04, artificialanalysis.ai
# model pages, max/reasoning variant per roster id).
ROSTER_AA = {
    "claude-haiku-4-5": 17,
    "claude-opus-5": 51,
    "claude-sonnet-5": 38,
    "claude-fable-5": 50,
    "gpt-5.5": 38,
    "gpt-5.4-mini": 24,
    "gemini-3.5-flash": 33,
    "gemini-3.1-pro-preview": 30,
    "gemini-3.1-flash-lite-preview": 16,
    "deepseek/deepseek-v4-flash": 34,
    "deepseek/deepseek-v4-pro": 36,
    "z-ai/glm-5.2": 34,
    "moonshotai/kimi-k2.7": 26,
    "qwen/qwen3.7-plus": 25,
    "qwen/qwen3-coder-next": 9,
    "qwen/qwen3-next-80b-a3b-instruct": 10,
    "minimax/minimax-m3": 29,
    "xiaomi/mimo-v2.5-pro": 26,
}

# Weight of the SWE-bench Verified signal when present, vs the AA index.
SWE_WEIGHT = 0.5

RANKINGS_ALPHA = 0.96


def load(name):
    with open(SRC / name, encoding="utf-8") as f:
        return json.load(f)


def dump(name, data):
    DST.mkdir(exist_ok=True)
    with open(DST / name, "w", encoding="utf-8", newline="\n") as f:
        json.dump(data, f, indent=2, ensure_ascii=False)
        f.write("\n")


def blend_row(qm_row, models, costs, alpha):
    """Exact runtime blend (Scorer.blendScoresV2) for one cluster at
    speed_weight=0 / output_cost_ratio=0: cost axis = input_per_1k only."""
    qmin, qmax = min(qm_row[m] for m in models), max(qm_row[m] for m in models)
    qr = qmax - qmin
    cmin, cmax = min(costs.values()), max(costs.values())
    cr = cmax - cmin
    out = {}
    for m in models:
        qn = (qm_row[m] - qmin) / qr if qr > 0 else 0.0
        cn = (costs[m] - cmin) / cr if cr > 0 else 0.0
        out[m] = alpha * qn + (1.0 - alpha) * (1.0 - cn)
    return out


def anchor_score(aa_idx, swebench_v):
    """Composite researched score in AA-index units. When a SWE-V number
    exists, blend it at SWE_WEIGHT, rescaling SWE-V onto the AA scale via a
    4-point fit (v4-flash 88.8/34, v4-pro 96.4/36, glm-5.3 95.4/45,
    glm-5.2 82.8/34)."""
    if swebench_v is None:
        return float(aa_idx)
    swe = [88.8, 96.4, 95.4, 82.8]
    aas = [34.0, 36.0, 45.0, 34.0]
    n = len(swe)
    sx = sum(swe); sy = sum(aas)
    sxx = sum(x * x for x in swe); sxy = sum(x * y for x, y in zip(swe, aas))
    slope = (n * sxy - sx * sy) / (n * sxx - sx * sx)
    intercept = (sy - slope * sx) / n
    swe_as_aa = slope * swebench_v + intercept
    return (1.0 - SWE_WEIGHT) * aa_idx + SWE_WEIGHT * swe_as_aa


def cluster_regression(row):
    """Per-cluster least-squares AA->q_norm regression over the roster
    models present in this cluster's row. Returns (slope, intercept) with
    the slope floored at 0 (higher researched score never maps lower)."""
    xs = [ROSTER_AA[m] for m in ROSTER_AA if m in row]
    ys = [row[m] for m in ROSTER_AA if m in row]
    n = len(xs)
    if n < 3:
        return None, None
    sx, sy = sum(xs), sum(ys)
    sxx = sum(x * x for x in xs)
    sxy = sum(x * y for x, y in zip(xs, ys))
    denom = n * sxx - sx * sx
    if abs(denom) < 1e-9:
        return None, None
    slope = (n * sxy - sx * sy) / denom
    if slope < 0:
        slope = 0.0
    intercept = (sy - slope * sx) / n
    return slope, intercept


def researched_cell(row, model_score, slope, intercept):
    """Affine map from researched score (AA units) onto the cluster's
    q_norm scale, clamped to the cluster's existing [min,max]."""
    lo, hi = min(row.values()), max(row.values())
    y = slope * model_score + intercept
    return min(hi, max(lo, y))


def main():
    qm = load("quality_means.json")
    axes = load("model_axes.json")
    reg = load("model_registry.json")
    rk = load("rankings.json")

    old_models = sorted(qm["quality_means"]["0"].keys())
    assert len(old_models) == 31, "expected the v0.76 31-model roster"
    aiand_names = [r[0] for r in AIAND_RESEARCHED]
    assert set(aiand_names) <= set(old_models), "v0.76 must already carry the AIand roster"

    # --- parity gate: the runtime blend must reproduce v0.76's committed
    # rankings bit-for-bit on the unchanged roster BEFORE we touch anything.
    costs_all = {m: (axes["axes"][m]["input_per_1k_usd"] or 0.0) for m in old_models}
    for k in range(16):
        got = blend_row(qm["quality_means"][str(k)], old_models, costs_all, RANKINGS_ALPHA)
        for m in old_models:
            assert abs(got[m] - rk["rankings"][str(k)][m]) < 1e-12, (
                f"v0.76 rankings parity failed at cluster {k} model {m}")

    # --- quality_means: researched anchors replace the v0.76 clone/scale cells.
    for k in range(16):
        row = qm["quality_means"][str(k)]
        slope, intercept = cluster_regression(row)
        if slope is None:
            continue  # degenerate cluster: keep v0.76 cells
        for name, aa_idx, swe_v, _in, _out, _ttft, _tps, _note in AIAND_RESEARCHED:
            s = anchor_score(aa_idx, swe_v)
            row[name] = researched_cell(row, s, slope, intercept)

    # --- model_axes: researched TTFT/TPS where published.
    for name, _aa, _swe, in_1m, out_1m, ttft_s, tps, _note in AIAND_RESEARCHED:
        ax = axes["axes"][name]
        ax["input_per_1k_usd"] = in_1m / 1000.0
        ax["output_per_1k_usd"] = out_1m / 1000.0
        if ttft_s is not None:
            ax["ttft_s"] = ttft_s
        if tps is not None:
            ax["tps"] = tps

    # --- rankings: regenerate over the 31-model roster with new cells.
    costs = {m: (axes["axes"][m]["input_per_1k_usd"] or 0.0) for m in old_models}
    rk["rankings"] = {
        str(k): blend_row(qm["quality_means"][str(k)], old_models, costs, RANKINGS_ALPHA)
        for k in range(16)
    }
    rk["meta"]["n_models"] = len(old_models)

    # --- model_registry: flip the 13 entries from proxy to researched.
    by_model = {e["model"]: e for e in reg["deployed_models"]}
    for name, _aa, _swe, _in, _out, _ttft, _tps, note in AIAND_RESEARCHED:
        e = by_model[name]
        e["bench_column"] = "aa_intelligence_index_v4.3.2"
        e["proxy"] = False
        e["research_note"] = note
        e["research_date"] = "2026-10-04"
    reg["meta"]["comment"] = (
        "v0.77: v0.76 roster with AIand columns re-anchored on researched benchmarks "
        "(AA Intelligence Index v4.3.2 + SWE-bench Verified where independently measured; "
        "scripts/build_v077_aiand_researched.py, sources in .context/bench-scores.md and "
        ".context/roster-aa-indexes.md). Per-cluster least-squares regression over the "
        "18-model roster anchors (AA 9-51).")
    reg["meta"]["last_refreshed"] = "2026-10-04"
    reg["meta"]["parent"] = "v0.76"

    DST.mkdir(exist_ok=True)
    shutil.copyfile(SRC / "centroids.bin", DST / "centroids.bin")
    features = {
        "meta": {
            "comment": "v0.77: v0.75 geometry + AIand research-anchored columns (build_v077_aiand_researched.py).",
            "k": 16,
            "n_models": len(old_models),
            "roster_version": "v0.77",
            "source": "quality_means.json + model_axes.json",
        },
        "models": {
            m: {
                "psi_probe": [qm["quality_means"][str(k)][m] for k in range(16)],
                "operational": axes["axes"][m],
            } for m in old_models
        },
    }

    # --- metadata.yaml
    with open(SRC / "metadata.yaml", encoding="utf-8") as f:
        meta = yaml.safe_load(f)
    meta["version"] = "v0.77"
    meta["parent"] = "v0.76"
    meta["deployed_providers"] = sorted(set(meta["deployed_providers"]) | {"aiand"})
    meta["changelog"] = (
        "v0.77 = v0.76 with the 13 AIand quality columns RE-ANCHORED on researched benchmarks "
        "(additive overlay; no model retired; centroids.bin BYTE-IDENTICAL to v0.75/v0.76). "
        "Calibration: per-cluster least-squares regression from AA Intelligence Index v4.3.2 "
        "(2026-10-04, max/reasoning variant) to the frozen q_norm cell, fit over the 18 v0.75 "
        "roster anchors (AA 9-51, interpolation range), blended 50/50 with SWE-bench Verified "
        "(independent Vals runs) where both exist, slope floored at 0, clamped to the cluster's "
        "[min,max]. Key corrections vs v0.76 guesses: glm-5.3-flash 0.85x glm-5.2 -> AA 42 "
        "vs 34, SWE-V 92.0 vs 82.8 (near-frontier at flash price); kimi-k3 1.05x k2.7 -> AA 44 vs 26, "
        "SWE-V 93.4 vs 78.2; glm-5.3 clone glm-5.2 -> AA 45 vs 34, SWE-V 95.4 vs 82.8; gemma-4/gpt-oss "
        "scaled 0.85/0.88 -> AA 15/12 (weak coders, cheap+fast lanes); qwen3.6 0.92x -> AA 21. "
        "model_axes TTFT/TPS replaced with AA-published per-model medians where available "
        "(v0.76 cloned the source model's axes). rankings.json regenerated by the exact runtime "
        "blend (parity-gated on v0.76 first). Generated by scripts/build_v077_aiand_researched.py "
        "from .context/bench-scores.md + .context/roster-aa-indexes.md sources. Candidate; "
        "artifacts/latest NOT bumped.\n\n" + meta["changelog"])
    with open(DST / "metadata.yaml", "w", encoding="utf-8", newline="\n") as f:
        yaml.safe_dump(meta, f, sort_keys=False, allow_unicode=True, width=100)

    # --- centroids: byte copy + hash verify
    sha256_of = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()  # noqa: E731
    assert sha256_of(SRC / "centroids.bin") == sha256_of(DST / "centroids.bin"), "centroids drift"

    dump("quality_means.json", qm)
    dump("model_axes.json", axes)
    dump("rankings.json", rk)
    dump("model_registry.json", reg)
    dump("model_features.json", features)

    print("wrote", DST)
    print("roster:", len(old_models), "models (AIand columns re-anchored on research)")


if __name__ == "__main__":
    main()
