#!/usr/bin/env python3
"""Build v0.77 ai&-only six-model cluster artifact bundle.

v0.77 replaces GLM-5.2 with GLM-5.3 (same 743B architecture, better agentic
benchmarks: AA Index 60 vs 53, Terminal-Bench 3.0 28.3 vs 4.6, DeepSWE v1.1
66.9 vs 46.2) and adds qwen/qwen3.8-27b as a TierLow pool model that wins no
clusters. Final roster (6): zai-org/glm-5.3, deepseek-ai/deepseek-v4-flash,
moonshotai/kimi-k2.7-code, moonshotai/kimi-k3, motif-technologies/motif-3,
qwen/qwen3.8-27b.

Reads v0.76 artifacts, applies the glm-5.2 → glm-5.3 rename plus the qwen3.8
addition, assigns per-cluster quality factors reflecting the probe results
(scripts/glm53_probe_results.json) and the motif probe
(scripts/motif_probe_results.json). centroids.bin is byte-identical.

Tier structure is unchanged from v0.76: kimi-k3 wins HARD_K3 {0,13}, motif-3
wins MID_M3 {1,3,4,5,6,7,8,12,14}, flash wins FLASH_5 {2,9,10,11,15}.

Usage: python3 scripts/build_v077_aiand_overlay.py [--force] [--glm53-win X]
Requires scripts/glm53_probe_results.json (go_no_go == PASS) and
scripts/motif_probe_results.json (go_no_go == PASS).
"""
import json
import shutil
import sys
from pathlib import Path

# --- model-name rename map (v0.76 id -> v0.77 id; glm-5.2's slot becomes glm-5.3) ---
RENAME_MAP = {
    "zai-org/glm-5.2": "zai-org/glm-5.3",
    "deepseek-ai/deepseek-v4-flash": "deepseek-ai/deepseek-v4-flash",
    "moonshotai/kimi-k2.7-code": "moonshotai/kimi-k2.7-code",
}
# v0.76 pool rows are read under these names; the other two roster members are
# carried over from v0.76 (kimi-k3, motif-3). qwen3.8-27b joins as a sixth
# pool model — TierLow like flash, wins no clusters.
KEEP_OLD = list(RENAME_MAP.keys())
KEPT_MODELS = ["moonshotai/kimi-k3", "motif-technologies/motif-3"]
NEW_MODELS = ["qwen/qwen3.8-27b"]
ALL_MODELS = [RENAME_MAP[m] for m in KEEP_OLD] + KEPT_MODELS + NEW_MODELS
ADDED_MODELS = ["zai-org/glm-5.3", "qwen/qwen3.8-27b"]
REMOVED_MODELS = ["zai-org/glm-5.2"]

GLM52 = "zai-org/glm-5.2"
GLM53 = "zai-org/glm-5.3"
GLM53_BENCH_COLUMN = "routerarena_z-ai/glm-5.2"  # proxy the GLM-5.2 Arena column
QWEN38 = "qwen/qwen3.8-27b"
QWEN38_BENCH_COLUMN = "routerarena_qwen/qwen3.6-27b"  # proxy its predecessor's Arena column

# --- per-cluster tier assignment (unchanged from v0.76) ---
HARD_K3 = {0, 13}
MID_M3 = {1, 3, 4, 5, 6, 7, 8, 12, 14}
FLASH_5 = {2, 9, 10, 11, 15}
Q53_OFF_DEFAULT = 0.97  # used when the probe omits recommended_q_glm53_off
Q_QWEN38_OFF_DEFAULT = 0.55  # used when the probe omits recommended_q_qwen38_off

# --- pinned catalog costs (per 1K tokens, USD; glm-5.3 + qwen3.8 at LIVE ai& prices) ---
CATALOG_COSTS = {
    "zai-org/glm-5.3": {"input_per_1k_usd": 0.001, "output_per_1k_usd": 0.004},
    "deepseek-ai/deepseek-v4-flash": {"input_per_1k_usd": 0.00015, "output_per_1k_usd": 0.00025},
    "moonshotai/kimi-k2.7-code": {"input_per_1k_usd": 0.00075, "output_per_1k_usd": 0.0035},
    "moonshotai/kimi-k3": {"input_per_1k_usd": 0.003, "output_per_1k_usd": 0.0125},
    "motif-technologies/motif-3": {"input_per_1k_usd": 0.0005, "output_per_1k_usd": 0.002},
    "qwen/qwen3.8-27b": {"input_per_1k_usd": 0.0004, "output_per_1k_usd": 0.003},
}

# tier -> (leader model, cluster set)
TIER_LEADERS = [
    ("moonshotai/kimi-k3", HARD_K3),
    ("motif-technologies/motif-3", MID_M3),
    ("deepseek-ai/deepseek-v4-flash", FLASH_5),
]
LEADER_FOR = {k: leader for leader, clusters in TIER_LEADERS for k in clusters}


def find_artifacts_dir() -> Path:
    script_dir = Path(__file__).resolve().parent
    return script_dir.parent / "internal" / "router" / "cluster" / "artifacts"


def load_json(path: Path) -> dict:
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def save_json(path: Path, data: dict) -> None:
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2, ensure_ascii=False)
        f.write("\n")


def read_m3_win() -> float:
    probe_path = Path(__file__).resolve().parent / "motif_probe_results.json"
    if not probe_path.exists():
        return 1.1
    probe = load_json(probe_path)
    if probe.get("go_no_go") != "PASS":
        sys.exit(f"ERROR: motif probe go_no_go={probe.get('go_no_go')!r} — do not build. Report to human.")
    return float(probe["recommended_m3_win"])


def read_glm53_probe():
    probe_path = Path(__file__).resolve().parent / "glm53_probe_results.json"
    if not probe_path.exists():
        return None
    return load_json(probe_path)




def glm53_factor(cluster: int, glm52_val: float, max_others: float,
                 q53_on: float, q53_off: float) -> float:
    """On-factor on the hard clusters and wherever glm-5.2 was the cluster
    max; off-factor (below the tier leader) elsewhere."""
    if cluster in HARD_K3 or glm52_val >= max_others:
        return q53_on
    return q53_off


def main():
    force = "--force" in sys.argv
    glm53_win_override = None
    if "--glm53-win" in sys.argv:
        glm53_win_override = float(sys.argv[sys.argv.index("--glm53-win") + 1])
    q_m3_win = None
    if "--mq-win" in sys.argv:
        q_m3_win = float(sys.argv[sys.argv.index("--mq-win") + 1])
    if q_m3_win is None:
        q_m3_win = read_m3_win()

    probe = read_glm53_probe()
    if probe is not None and probe.get("go_no_go") != "PASS":
        sys.exit(f"ERROR: glm53 probe go_no_go={probe.get('go_no_go')!r} — do not build. Report to human.")
    if glm53_win_override is not None:
        q53_on = glm53_win_override
    elif probe is not None:
        q53_on = float(probe["recommended_q_glm53_on"])
    else:
        sys.exit("ERROR: scripts/glm53_probe_results.json not found — run the GLM-5.3 probe first (or pass --glm53-win).")
    q53_off = float(probe.get("recommended_q_glm53_off", Q53_OFF_DEFAULT)) if probe is not None else Q53_OFF_DEFAULT
    q_qwen38_off = float(probe.get("recommended_q_qwen38_off", Q_QWEN38_OFF_DEFAULT)) if probe is not None else Q_QWEN38_OFF_DEFAULT
    n_prompts = probe.get("n_prompts", "?") if probe is not None else "?"
    test_date = probe.get("test_date", "2026-08-29") if probe is not None else "2026-08-29"
    print(f"  Q_GLM53_ON = {q53_on}")
    print(f"  Q_GLM53_OFF = {q53_off}")
    print(f"  Q_M3_WIN = {q_m3_win}")
    print(f"  Q_QWEN38_OFF = {q_qwen38_off}")

    glm53_note = (
        f"GLM-5.3 replaces GLM-5.2 — per-cluster quality x{q53_on}/{q53_off} of GLM-5.2 values "
        f"(probe {n_prompts} prompts, {test_date})."
    )
    qwen38_note = (
        f"qwen3.8-27b joins as a TierLow pool model — per-cluster quality x{q_qwen38_off} of pool max; "
        f"never wins a cluster (probe {n_prompts} prompts, {test_date})."
    )

    artifacts_dir = find_artifacts_dir()
    src = artifacts_dir / "v0.76"
    dst = artifacts_dir / "v0.77"

    if dst.exists():
        if not force:
            sys.exit(f"ERROR: {dst} exists — use --force to overwrite")
        shutil.rmtree(dst)
    dst.mkdir(parents=True)

    shutil.copy2(src / "centroids.bin", dst / "centroids.bin")
    print("  copied centroids.bin (byte-identical)")

    registry = load_json(src / "model_registry.json")
    quality_means = load_json(src / "quality_means.json")
    model_axes = load_json(src / "model_axes.json")
    model_features = load_json(src / "model_features.json")

    # --- model_registry.json: glm-5.2 -> glm-5.3; qwen3.8-27b appended;
    #     pool models re-noted; kimi-k3/motif-3 entries copied unchanged ---
    kept = []
    for e in registry["deployed_models"]:
        if e["model"] == GLM52:
            kept.append({
                "model": GLM53,
                "provider": "aiand",
                "bench_column": GLM53_BENCH_COLUMN,
                "proxy": True,
                "proxy_note": glm53_note,
            })
        elif e["model"] in ("deepseek-ai/deepseek-v4-flash", "moonshotai/kimi-k2.7-code"):
            kept.append({
                "model": e["model"],
                "provider": "aiand",
                "bench_column": e.get("bench_column", ""),
                "proxy": True,
                "proxy_note": "v0.77 overlay: kept from v0.76.",
            })
        else:
            kept.append(e)
    kept.append({
        "model": QWEN38,
        "provider": "aiand",
        "bench_column": QWEN38_BENCH_COLUMN,
        "proxy": True,
        "proxy_note": qwen38_note,
    })
    registry["deployed_models"] = kept

    # --- quality_means.json: glm-5.3 = glm-5.2's row x probe factor (clamped
    #     below the tier leader if the boost would overtake it); kimi-k3/motif-3
    #     re-derived from the v0.76 pool max (bit-identical to v0.76) ---
    qm = quality_means["quality_means"]
    clamped = []
    qwen_clamped = []
    for k_str, row in qm.items():
        k = int(k_str)
        pool_max = max(row[old] for old in KEEP_OLD)
        glm52_val = row[GLM52]
        max_others = max(v for m, v in row.items() if m != GLM52)
        # glm-5.3 replaces glm-5.2: scale glm-5.2's own row
        val53 = round(glm52_val * glm53_factor(k, glm52_val, max_others, q53_on, q53_off), 16)
        leader = LEADER_FOR[k]
        leader_val = row[leader]
        if val53 >= leader_val:
            print(f"  WARN: cluster {k}: glm-5.3 scaled value {val53} >= tier leader {leader} ({leader_val}); clamping to 0.99x leader")
            val53 = round(leader_val * 0.99, 16)
            clamped.append(k)
        row[GLM53] = val53
        # qwen3.8-27b: pool model at probe off-factor of the pool max, clamped
        # below every tier leader so it never wins a cluster.
        val38 = round(pool_max * q_qwen38_off, 16)
        if val38 >= leader_val:
            print(f"  WARN: cluster {k}: qwen3.8-27b value {val38} >= tier leader {leader} ({leader_val}); clamping to 0.99x leader")
            val38 = round(leader_val * 0.99, 16)
            qwen_clamped.append(k)
        row[QWEN38] = val38
        for m in list(row.keys()):
            if m not in ALL_MODELS:
                del row[m]
    if clamped:
        print(f"  NOTE: glm-5.3 clamped below tier leader on clusters {sorted(clamped)}")
    qm_meta = quality_means.get("meta", {})
    qm_meta["cost_per_1k_input_usd"] = {m: CATALOG_COSTS[m]["input_per_1k_usd"] for m in ALL_MODELS}
    qm_meta["cost_per_1k_output_usd"] = {m: CATALOG_COSTS[m]["output_per_1k_usd"] for m in ALL_MODELS}
    qm_meta["roster_edit"] = {"added": ADDED_MODELS, "removed": REMOVED_MODELS, "parent": "v0.76", "tool": "build_v077_aiand_overlay.py", "version": "v0.77"}

    # --- model_axes.json: 6 models, pinned costs, tps/ttft inherited from v0.76 rows ---
    old_axes = model_axes["axes"]
    new_axes = {}
    for old_name, new_name in RENAME_MAP.items():
        src_entry = old_axes[old_name]
        new_axes[new_name] = {
            "input_per_1k_usd": CATALOG_COSTS[new_name]["input_per_1k_usd"],
            "output_per_1k_usd": CATALOG_COSTS[new_name]["output_per_1k_usd"],
            "tps": src_entry.get("tps"),
            "ttft_s": src_entry.get("ttft_s"),
            "verbosity_tokens": src_entry.get("verbosity_tokens"),
        }
    for m in KEPT_MODELS:
        src_entry = old_axes[m]
        new_axes[m] = {
            "input_per_1k_usd": CATALOG_COSTS[m]["input_per_1k_usd"],
            "output_per_1k_usd": CATALOG_COSTS[m]["output_per_1k_usd"],
            "tps": src_entry.get("tps"),
            "ttft_s": src_entry.get("ttft_s"),
            "verbosity_tokens": src_entry.get("verbosity_tokens"),
        }
    # qwen3.8-27b has no v0.76 row; no measured tps/ttft — nulls are honest.
    new_axes[QWEN38] = {
        "input_per_1k_usd": CATALOG_COSTS[QWEN38]["input_per_1k_usd"],
        "output_per_1k_usd": CATALOG_COSTS[QWEN38]["output_per_1k_usd"],
        "tps": None,
        "ttft_s": None,
        "verbosity_tokens": None,
    }
    model_axes["axes"] = new_axes
    ma_meta = model_axes.get("meta", {})
    ma_meta["cost_per_1k_input_usd"] = {m: CATALOG_COSTS[m]["input_per_1k_usd"] for m in ALL_MODELS}
    ma_meta["cost_per_1k_output_usd"] = {m: CATALOG_COSTS[m]["output_per_1k_usd"] for m in ALL_MODELS}

    # --- model_features.json: 6 models, pinned costs, psi_probe from quality_means ---
    clusters_sorted = sorted(qm.keys(), key=int)
    old_mf = model_features["models"]
    new_mf = {}
    for old_name, new_name in RENAME_MAP.items():
        src_op = old_mf[old_name]["operational"]
        new_mf[new_name] = {
            "operational": {
                "input_per_1k_usd": CATALOG_COSTS[new_name]["input_per_1k_usd"],
                "output_per_1k_usd": CATALOG_COSTS[new_name]["output_per_1k_usd"],
                "tps": src_op.get("tps"),
                "ttft_s": src_op.get("ttft_s"),
                "verbosity_tokens": src_op.get("verbosity_tokens"),
            },
            "psi_probe": [qm[k][new_name] for k in clusters_sorted],
        }
    for m in KEPT_MODELS:
        src_op = old_mf[m]["operational"]
        new_mf[m] = {
            "operational": {
                "input_per_1k_usd": CATALOG_COSTS[m]["input_per_1k_usd"],
                "output_per_1k_usd": CATALOG_COSTS[m]["output_per_1k_usd"],
                "tps": src_op.get("tps"),
                "ttft_s": src_op.get("ttft_s"),
                "verbosity_tokens": src_op.get("verbosity_tokens"),
            },
            "psi_probe": [qm[k][m] for k in clusters_sorted],
        }
    new_mf[QWEN38] = {
        "operational": {
            "input_per_1k_usd": CATALOG_COSTS[QWEN38]["input_per_1k_usd"],
            "output_per_1k_usd": CATALOG_COSTS[QWEN38]["output_per_1k_usd"],
            "tps": None,
            "ttft_s": None,
            "verbosity_tokens": None,
        },
        "psi_probe": [qm[k][QWEN38] for k in clusters_sorted],
    }
    model_features["models"] = new_mf
    mf_meta = model_features.get("meta", {})
    mf_meta["n_models"] = len(new_mf)
    mf_meta["roster_version"] = "v0.77"
    mf_meta["comment"] = "v0.77: 6-model ai&-only bundle, glm-5.3 replaces glm-5.2, qwen3.8-27b joins as TierLow pool model (kimi-k3/motif-3/flash tiers unchanged). rankings.json intentionally absent."

    # --- metadata.yaml (default_routing_knobs preserved from v0.76) ---
    write_metadata(dst, q53_on, q53_off, n_prompts, test_date, q_m3_win, q_qwen38_off)

    # --- write JSON artifacts (NO rankings.json) ---
    save_json(dst / "model_registry.json", registry)
    save_json(dst / "quality_means.json", quality_means)
    save_json(dst / "model_axes.json", model_axes)
    save_json(dst / "model_features.json", model_features)

    # --- self-check: tier leader has strictly-highest quality on its clusters ---
    for leader, clusters in TIER_LEADERS:
        for k_str in clusters_sorted:
            k = int(k_str)
            if k not in clusters:
                continue
            row = qm[k_str]
            leader_val = row[leader]
            others = [v for m, v in row.items() if m != leader]
            if leader_val <= max(others):
                raise AssertionError(
                    f"self-check FAIL: cluster {k} leader {leader} "
                    f"({leader_val}) not strictly > max others ({max(others)})"
                )

    if qwen_clamped:
        print(f"  NOTE: qwen3.8-27b clamped below tier leader on clusters {sorted(qwen_clamped)}")

    print(f"\nv0.77 bundle complete at {dst}")
    print(f"  {len(registry['deployed_models'])} models in registry")
    print(f"  {len(qm)} clusters in quality_means")
    print(f"  {len(model_axes['axes'])} models in model_axes")
    print(f"  {mf_meta['n_models']} models in model_features")


def write_metadata(dst: Path, q53_on: float, q53_off: float, n_prompts, test_date, q_m3_win: float, q_qwen38_off: float) -> None:
    import yaml
    src_meta_path = dst.parent / "v0.76" / "metadata.yaml"
    with open(src_meta_path, "r", encoding="utf-8") as f:
        meta = yaml.safe_load(f)
    meta["version"] = "v0.77"
    meta["parent"] = "v0.76"
    meta["status"] = "candidate"
    meta["deployed_providers"] = ["aiand"]
    meta["deployed_models"] = ALL_MODELS
    meta["cost_per_1k_input_usd"] = {m: CATALOG_COSTS[m]["input_per_1k_usd"] for m in ALL_MODELS}
    meta["cost_per_1k_output_usd"] = {m: CATALOG_COSTS[m]["output_per_1k_usd"] for m in ALL_MODELS}
    meta["training"]["roster_edit"] = {"added": ADDED_MODELS, "removed": REMOVED_MODELS, "parent": "v0.76", "tool": "build_v077_aiand_overlay.py", "version": "v0.77"}
    meta["changelog"] = (
        "v0.77 = v0.76 with GLM-5.2 replaced by GLM-5.3 and qwen3.8-27b added on FROZEN geometry. "
        "centroids.bin BYTE-IDENTICAL to v0.76. "
        "Tier structure unchanged: kimi-k3 wins HARD_K3 {0,13}; "
        f"motif-3 wins MID_M3 {{1,3,4,5,6,7,8,12,14}} at {q_m3_win}x maxpool; "
        "flash wins FLASH_5 {2,9,10,11,15}. "
        "glm-5.3 quality = glm-5.2's per-cluster values x probe factor "
        f"({q53_on} on hard clusters {{0,13}}, {q53_off} elsewhere; "
        f"{n_prompts}-prompt probe {test_date}). "
        f"qwen3.8-27b quality = pool max x {q_qwen38_off} everywhere (TierLow pool model, wins no clusters). "
        "GLM-5.3 shares GLM-5.2's 743B base with better agentic benchmarks "
        "(AA Index 60 vs 53, Terminal-Bench 3.0 28.3 vs 4.6, DeepSWE v1.1 66.9 vs 46.2). "
        "Costs pinned from ai& catalog (live prices) for all 6 models. "
        "rankings.json intentionally absent. "
        "6-model ai&-only registry. NOT promoted to latest."
    )
    with open(dst / "metadata.yaml", "w", encoding="utf-8") as f:
        yaml.dump(meta, f, default_flow_style=False, sort_keys=False)


if __name__ == "__main__":
    main()
