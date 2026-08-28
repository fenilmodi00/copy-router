#!/usr/bin/env python3
"""Build v0.76 ai&-only three-tier cluster artifact bundle.

Reads v0.75 artifacts, filters to 5 ai&-eligible models (renamed to exact
ai& catalog IDs), assigns per-cluster quality by a 3-tier factor table so
motif-3 wins the medium tier, kimi-k3 wins the hard tier, and flash wins the
conversational tier. Pins catalog-accurate costs for all 5 models.
centroids.bin is byte-identical (frozen geometry). rankings.json is NOT shipped.

Q_M3_WIN is read from scripts/motif_probe_results.json (go_no_go gate).
Override with --mq-win <float>.

Usage: python scripts/build_v076_aiand_overlay.py [--force] [--mq-win <float>]
"""
import json
import shutil
import sys
from pathlib import Path

# --- model-name rename map (v0.75 key -> v0.76 ai& catalog ID) ---
RENAME_MAP = {
    "zai-org/glm-5.2": "zai-org/glm-5.2",
    "deepseek-ai/deepseek-v4-flash": "deepseek-ai/deepseek-v4-flash",
    "moonshotai/kimi-k2.7": "moonshotai/kimi-k2.7-code",
}
KEEP_OLD = list(RENAME_MAP.keys())
NEW_MODELS = ["moonshotai/kimi-k3", "motif-technologies/motif-3"]
ALL_MODELS = [RENAME_MAP[m] for m in KEEP_OLD] + NEW_MODELS

# --- per-cluster tier assignment ---
HARD_K3 = {0, 13}
MID_M3 = {1, 3, 4, 5, 6, 7, 8, 12, 14}
FLASH_5 = {2, 9, 10, 11, 15}

Q_K3_ON = 1.03
Q_K3_OFF = 0.97
Q_M3_OFF = 0.92

# --- pinned catalog costs (per 1K tokens, USD) ---
CATALOG_COSTS = {
    "zai-org/glm-5.2": {"input_per_1k_usd": 0.001, "output_per_1k_usd": 0.004},
    "deepseek-ai/deepseek-v4-flash": {"input_per_1k_usd": 0.00015, "output_per_1k_usd": 0.00025},
    "moonshotai/kimi-k2.7-code": {"input_per_1k_usd": 0.00075, "output_per_1k_usd": 0.0035},
    "moonshotai/kimi-k3": {"input_per_1k_usd": 0.003, "output_per_1k_usd": 0.0125},
    "motif-technologies/motif-3": {"input_per_1k_usd": 0.0005, "output_per_1k_usd": 0.002},
}

# --- registry entries for the 2 new models ---
NEW_REGISTRY_ENTRIES = [
    {"model": "moonshotai/kimi-k3", "provider": "aiand",
     "bench_column": "routerarena_moonshotai/kimi-k2.6", "proxy": True,
     "proxy_note": "kimi-k3 wins HARD_K3 {0,13} at 1.03x maxpool. Below tier leader elsewhere (0.97x)."},
    {"model": "motif-technologies/motif-3", "provider": "aiand",
     "bench_column": "routerarena_z-ai/glm-5", "proxy": True,
     "proxy_note": "motif-3 wins MID_M3 at Q_M3_WIN x maxpool (30-prompt medium-coding probe). Below tier leader elsewhere (0.92x)."},
]

# tier -> (leader model, cluster set)
TIER_LEADERS = [
    ("moonshotai/kimi-k3", HARD_K3),
    ("motif-technologies/motif-3", MID_M3),
    ("deepseek-ai/deepseek-v4-flash", FLASH_5),
]


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
        sys.exit(f"ERROR: {probe_path} not found — run Task 1 probe first.")
    with open(probe_path, "r", encoding="utf-8") as f:
        probe = json.load(f)
    if probe.get("go_no_go") != "PASS":
        sys.exit(f"ERROR: probe go_no_go={probe.get('go_no_go')!r} — do not build. Report to human.")
    return float(probe["recommended_m3_win"])


def factor_for(cluster: int, model: str, q_m3_win: float) -> float:
    if model == "moonshotai/kimi-k3":
        return Q_K3_ON if cluster in HARD_K3 else Q_K3_OFF
    if model == "motif-technologies/motif-3":
        return q_m3_win if cluster in MID_M3 else Q_M3_OFF
    raise KeyError(model)


def main():
    force = "--force" in sys.argv
    q_m3_win = None
    if "--mq-win" in sys.argv:
        idx = sys.argv.index("--mq-win")
        q_m3_win = float(sys.argv[idx + 1])
    if q_m3_win is None:
        q_m3_win = read_m3_win()
    print(f"  Q_M3_WIN = {q_m3_win}")

    artifacts_dir = find_artifacts_dir()
    src = artifacts_dir / "v0.75"
    dst = artifacts_dir / "v0.76"

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

    # --- model_registry.json: keep 3 (renamed) + add 2 new, all provider=aiand ---
    kept = []
    for e in registry["deployed_models"]:
        if e["model"] in KEEP_OLD:
            kept.append({
                "model": RENAME_MAP[e["model"]],
                "provider": "aiand",
                "bench_column": e.get("bench_column", ""),
                "proxy": True,
                "proxy_note": "ai& overlay: kept from v0.75, renamed to ai& catalog ID.",
            })
    kept.extend(NEW_REGISTRY_ENTRIES)
    registry["deployed_models"] = kept

    # --- quality_means.json: rename existing, add kimi-k3/motif-3 per tier ---
    qm = quality_means["quality_means"]
    for k_str, row in qm.items():
        k = int(k_str)
        pool_max = max(row[old] for old in KEEP_OLD)
        for old in KEEP_OLD:
            row[RENAME_MAP[old]] = row.pop(old)
        row["moonshotai/kimi-k3"] = round(pool_max * factor_for(k, "moonshotai/kimi-k3", q_m3_win), 16)
        row["motif-technologies/motif-3"] = round(pool_max * factor_for(k, "motif-technologies/motif-3", q_m3_win), 16)
        for m in list(row.keys()):
            if m not in ALL_MODELS:
                del row[m]
    qm_meta = quality_means.get("meta", {})
    qm_meta["cost_per_1k_input_usd"] = {m: CATALOG_COSTS[m]["input_per_1k_usd"] for m in ALL_MODELS}
    qm_meta["cost_per_1k_output_usd"] = {m: CATALOG_COSTS[m]["output_per_1k_usd"] for m in ALL_MODELS}
    qm_meta["roster_edit"] = {"added": NEW_MODELS, "parent": "v0.75", "tool": "build_v076_aiand_overlay.py", "version": "v0.76"}

    # --- model_axes.json: 5 models, pinned costs, preserve tps/ttft for existing ---
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
    for m in NEW_MODELS:
        new_axes[m] = {
            "input_per_1k_usd": CATALOG_COSTS[m]["input_per_1k_usd"],
            "output_per_1k_usd": CATALOG_COSTS[m]["output_per_1k_usd"],
            "tps": None,
            "ttft_s": None,
            "verbosity_tokens": None,
        }
    model_axes["axes"] = new_axes
    ma_meta = model_axes.get("meta", {})
    ma_meta["cost_per_1k_input_usd"] = {m: CATALOG_COSTS[m]["input_per_1k_usd"] for m in ALL_MODELS}
    ma_meta["cost_per_1k_output_usd"] = {m: CATALOG_COSTS[m]["output_per_1k_usd"] for m in ALL_MODELS}

    # --- model_features.json: 5 models, pinned costs, psi_probe from quality_means ---
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
    for m in NEW_MODELS:
        new_mf[m] = {
            "operational": {
                "input_per_1k_usd": CATALOG_COSTS[m]["input_per_1k_usd"],
                "output_per_1k_usd": CATALOG_COSTS[m]["output_per_1k_usd"],
                "tps": None,
                "ttft_s": None,
                "verbosity_tokens": None,
            },
            "psi_probe": [qm[k][m] for k in clusters_sorted],
        }
    model_features["models"] = new_mf
    mf_meta = model_features.get("meta", {})
    mf_meta["n_models"] = len(new_mf)
    mf_meta["roster_version"] = "v0.76"
    mf_meta["comment"] = "v0.76: 5-model ai&-only three-tier bundle (kimi-k3/motif-3/flash). rankings.json intentionally absent."

    # --- metadata.yaml (default_routing_knobs preserved from v0.75) ---
    write_metadata(dst, q_m3_win)

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

    print(f"\nv0.76 bundle complete at {dst}")
    print(f"  {len(registry['deployed_models'])} models in registry")
    print(f"  {len(qm)} clusters in quality_means")
    print(f"  {len(model_axes['axes'])} models in model_axes")
    print(f"  {mf_meta['n_models']} models in model_features")


def write_metadata(dst: Path, q_m3_win: float) -> None:
    import yaml
    src_meta_path = dst.parent / "v0.75" / "metadata.yaml"
    with open(src_meta_path, "r", encoding="utf-8") as f:
        meta = yaml.safe_load(f)
    meta["version"] = "v0.76"
    meta["parent"] = "v0.75"
    meta["status"] = "candidate"
    meta["deployed_providers"] = ["aiand"]
    meta["deployed_models"] = ALL_MODELS
    meta["cost_per_1k_input_usd"] = {m: CATALOG_COSTS[m]["input_per_1k_usd"] for m in ALL_MODELS}
    meta["cost_per_1k_output_usd"] = {m: CATALOG_COSTS[m]["output_per_1k_usd"] for m in ALL_MODELS}
    meta["training"]["roster_edit"] = {"added": NEW_MODELS, "parent": "v0.75", "tool": "build_v076_aiand_overlay.py", "version": "v0.76"}
    meta["changelog"] = (
        "v0.76 = v0.75 filtered to 5 ai& models + kimi-k3 + motif-3 added "
        "on FROZEN geometry. centroids.bin BYTE-IDENTICAL to v0.75. "
        "Three-tier per-cluster quality: kimi-k3 wins HARD_K3 {0,13} at 1.03x maxpool; "
        f"motif-3 wins MID_M3 {{1,3,4,5,6,7,8,12,14}} at {q_m3_win}x maxpool (30-prompt probe); "
        "flash wins FLASH_5 {2,9,10,11,15} (maxpool). "
        "Model IDs corrected to exact ai& catalog names. "
        "Costs pinned from ai& catalog for all 5 models. "
        "rankings.json intentionally absent (stale 18-model data). "
        "5-model ai&-only registry. NOT promoted to latest."
    )
    with open(dst / "metadata.yaml", "w", encoding="utf-8") as f:
        yaml.dump(meta, f, default_flow_style=False, sort_keys=False)


if __name__ == "__main__":
    main()
