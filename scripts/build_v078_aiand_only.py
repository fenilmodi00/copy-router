#!/usr/bin/env python3
"""Build artifacts/v0.78: the aiand-only 8-model registry bundle.

Narrows v0.77's 31-model roster to the curated 8-model AIand product roster
(user decision 2026-10-04). The registry IS the enforcement surface: boot
registers all providers regardless of which keys are wired, so an 8-model
model_registry.json is what actually restricts routing to the 8 — no env pin
or catalog change can widen it.

- centroids.bin copied BYTE-IDENTICAL from v0.77 (geometry frozen since v0.75).
- quality_means/model_axes/model_features keep all v0.77 columns (NewScorer
  validates only registry candidates; extra columns are harmless and keep
  cross-deploy compatibility).
- model_registry.json deployed_models = ONLY the 8, provider aiand,
  bench_column aa_intelligence_index_v4.3.2, research cells inherited from
  v0.77's researched anchoring (measured labels land in v0.79).
- rankings.json regenerated over the 8 only (same runtime blend, parity-gated
  against v0.77 first).
- metadata.yaml documents the roster decision; artifacts/latest NOT touched
  here (promotion is a separate deliberate step, done with the family-gate
  exemption in the same ticket series).

Usage: python scripts/build_v078_aiand_only.py
Requires: PyYAML. Deterministic.
"""

import hashlib
import json
import shutil
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
ART = ROOT / "internal" / "router" / "cluster" / "artifacts"
SRC = ART / "v0.77"
DST = ART / "v0.78"

# The curated 8-model AIand product roster (2026-10-04 user decision).
# Dropped from v0.77's 13: glm-5.2, kimi-k2.7-code, qwen3.6-27b, gemma-4,
# gpt-oss-120b (older/less capable lanes; see parent issue #64).
AIAND_ONLY = [
    "deepseek-ai/deepseek-v4-pro",
    "deepseek-ai/deepseek-v4-flash",
    "deepseek-ai/deepseek-v4.1-flash",
    "zai-org/glm-5.3",
    "zai-org/glm-5.3-flash",
    "qwen/qwen3.8-27b",
    "moonshotai/kimi-k3",
    "motif-technologies/motif-3",
]

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


def main():
    qm = load("quality_means.json")
    axes = load("model_axes.json")
    reg = load("model_registry.json")
    rk = load("rankings.json")

    all_models = sorted(qm["quality_means"]["0"].keys())
    assert len(all_models) == 31, "expected v0.77's 31-model roster"
    assert set(AIAND_ONLY) <= set(all_models), "v0.77 must carry the 8"

    # --- parity gate: the runtime blend over v0.77's full roster must
    # reproduce v0.77's committed rankings bit-for-bit before we narrow.
    costs_all = {m: (axes["axes"][m]["input_per_1k_usd"] or 0.0) for m in all_models}
    for k in range(16):
        got = blend_row(qm["quality_means"][str(k)], all_models, costs_all, RANKINGS_ALPHA)
        for m in all_models:
            assert abs(got[m] - rk["rankings"][str(k)][m]) < 1e-12, (
                f"v0.77 rankings parity failed at cluster {k} model {m}")

    # --- rankings: regenerate over the 8 only.
    costs8 = {m: costs_all[m] for m in AIAND_ONLY}
    rk["rankings"] = {
        str(k): blend_row(qm["quality_means"][str(k)], AIAND_ONLY, costs8, RANKINGS_ALPHA)
        for k in range(16)
    }
    rk["meta"]["n_models"] = len(AIAND_ONLY)

    # --- model_registry: deployed_models = ONLY the 8.
    by_model = {e["model"]: e for e in reg["deployed_models"]}
    reg["deployed_models"] = [by_model[m] for m in AIAND_ONLY]
    for e in reg["deployed_models"]:
        e["provider"] = "aiand"
        e["bench_column"] = "aa_intelligence_index_v4.3.2"
        e["proxy"] = False
    reg["meta"]["comment"] = (
        "v0.78: aiand-only product bundle — 8-model registry (2026-10-04 roster "
        "decision, parent issue #64). The registry is the enforcement surface: "
        "boot registers all providers regardless of keys, so narrowing lives here. "
        "Cells inherited from v0.77's researched anchoring; measured labels land "
        "in v0.79. Family note: deepseek-v4-flash + v4.1-flash are distinct "
        "capability variants (text-only vs multimodal), not a supersession.")
    reg["meta"]["last_refreshed"] = "2026-10-04"
    reg["meta"]["parent"] = "v0.77"

    DST.mkdir(exist_ok=True)
    shutil.copyfile(SRC / "centroids.bin", DST / "centroids.bin")

    # quality_means / model_axes / model_features: copy through unchanged
    # (extra columns are harmless; NewScorer validates registry candidates only).
    for name in ("quality_means.json", "model_axes.json", "model_features.json"):
        shutil.copyfile(SRC / name, DST / name)

    # --- metadata.yaml
    with open(SRC / "metadata.yaml", encoding="utf-8") as f:
        meta = yaml.safe_load(f)
    meta["version"] = "v0.78"
    meta["parent"] = "v0.77"
    meta["deployed_models"] = list(AIAND_ONLY)
    meta["deployed_providers"] = ["aiand"]
    meta["cost_per_1k_input_usd"] = {
        m: v for m, v in meta["cost_per_1k_input_usd"].items() if m in AIAND_ONLY
    }
    meta["changelog"] = (
        "v0.78 = v0.77 narrowed to the 8-model curated AIand roster (user decision "
        "2026-10-04; dropped glm-5.2, kimi-k2.7-code, qwen3.6-27b, gemma-4, "
        "gpt-oss-120b). The REGISTRY is the enforcement surface — boot registers all "
        "providers regardless of wired keys, so an 8-model model_registry.json is what "
        "restricts routing; no env pin or BYOK header can widen it. quality_means/"
        "model_axes columns keep all 31 v0.77 entries (loader validates registry "
        "candidates only); rankings regenerated over the 8 by the exact runtime blend "
        "(parity-gated on v0.77's committed rankings first). centroids.bin "
        "BYTE-IDENTICAL to v0.75/v0.76/v0.77. Family note: deepseek-v4-flash + "
        "v4.1-flash share a family key but are distinct capability variants "
        "(text-only vs multimodal) — exempted, not a supersession. Cells inherit "
        "v0.77's researched anchoring; v0.79 replaces them with measured labels "
        "(RouterArena full-split campaign). Candidate; promotion handled with the "
        "family-gate exemption in the same ticket series.\n\n" + meta["changelog"])
    with open(DST / "metadata.yaml", "w", encoding="utf-8", newline="\n") as f:
        yaml.safe_dump(meta, f, sort_keys=False, allow_unicode=True, width=100)

    # --- centroids: byte copy + hash verify
    sha256_of = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()  # noqa: E731
    assert sha256_of(SRC / "centroids.bin") == sha256_of(DST / "centroids.bin"), "centroids drift"

    dump("rankings.json", rk)
    dump("model_registry.json", reg)

    print("wrote", DST)
    print("roster:", len(AIAND_ONLY), "models (registry-enforced aiand-only)")


if __name__ == "__main__":
    main()
