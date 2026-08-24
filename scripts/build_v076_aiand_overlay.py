#!/usr/bin/env python3
"""Build v0.76 ai&-only cluster artifact bundle.

Reads v0.75 artifacts, filters to 5 ai&-eligible models, adds kimi-k3 + motif-3
with manually-assigned quality scores, writes a 5-model v0.76 bundle.
centroids.bin is byte-identical (frozen geometry).

Usage: python scripts/build_v076_aiand_overlay.py [--force]
"""
import json
import shutil
import sys
from pathlib import Path

KIMI_K3_MARGIN = 1.03
MOTIF_3_FACTOR = 0.95

KEEP_MODELS = ["z-ai/glm-5.2", "deepseek/deepseek-v4-flash", "moonshotai/kimi-k2.7"]
NEW_MODELS = ["moonshotai/kimi-k3", "motif-technologies/motif-3"]
ALL_MODELS = KEEP_MODELS + NEW_MODELS

NEW_MODEL_AXES = {
    "moonshotai/kimi-k3": {"input_per_1k_usd": 0.003, "output_per_1k_usd": 0.0125, "tps": None, "ttft_s": None, "verbosity_tokens": None},
    "motif-technologies/motif-3": {"input_per_1k_usd": 0.0005, "output_per_1k_usd": 0.002, "tps": None, "ttft_s": None, "verbosity_tokens": None},
}

NEW_REGISTRY_ENTRIES = [
    {"model": "moonshotai/kimi-k3", "provider": "aiand", "bench_column": "routerarena_moonshotai/kimi-k2.6", "direct_label": "aa", "proxy": True,
     "proxy_note": "kimi-k3 quality = max(glm-5.2, deepseek-flash, kimi-k2.7) x 1.03 per cluster. Validated by 100-prompt probe. User positioning: most capable frontier model."},
    {"model": "motif-technologies/motif-3", "provider": "aiand", "bench_column": "routerarena_z-ai/glm-5", "direct_label": "aa", "proxy": True,
     "proxy_note": "motif-3 quality = glm-5.2 x 0.95 per cluster. Inert (wins no clusters) but available via /force-model. User positioning: just behind glm-5.2 at half pricing."},
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

def main():
    force = "--force" in sys.argv
    artifacts_dir = find_artifacts_dir()
    src = artifacts_dir / "v0.75"
    dst = artifacts_dir / "v0.76"

    if dst.exists():
        if not force:
            sys.exit(f"ERROR: {dst} exists — use --force to overwrite")
        shutil.rmtree(dst)
    dst.mkdir(parents=True)

    shutil.copy2(src / "centroids.bin", dst / "centroids.bin")
    print(f"  copied centroids.bin (byte-identical)")

    registry = load_json(src / "model_registry.json")
    quality_means = load_json(src / "quality_means.json")
    model_axes = load_json(src / "model_axes.json")
    model_features = load_json(src / "model_features.json")

    registry["deployed_models"] = [
        {"model": e["model"], "provider": "aiand",
         "bench_column": e.get("bench_column", ""),
         "proxy": True,
         "proxy_note": f"ai& overlay: kept from v0.75, provider rewritten to aiand."}
        for e in registry["deployed_models"]
        if e["model"] in KEEP_MODELS
    ]
    registry["deployed_models"].extend(NEW_REGISTRY_ENTRIES)

    for k_str, row in quality_means["quality_means"].items():
        pool_max = max(row[m] for m in KEEP_MODELS)
        row["moonshotai/kimi-k3"] = round(pool_max * KIMI_K3_MARGIN, 16)
        row["motif-technologies/motif-3"] = round(row["z-ai/glm-5.2"] * MOTIF_3_FACTOR, 16)
        for m in list(row.keys()):
            if m not in ALL_MODELS:
                del row[m]
    qm_meta = quality_means.get("meta", {})
    qm_meta.setdefault("cost_per_1k_input_usd", {})
    for m in NEW_MODELS:
        qm_meta["cost_per_1k_input_usd"][m] = NEW_MODEL_AXES[m]["input_per_1k_usd"]
    qm_meta["roster_edit"] = {"added": NEW_MODELS, "parent": "v0.75", "tool": "build_v076_aiand_overlay.py", "version": "v0.76"}

    for m in NEW_MODELS:
        model_axes["axes"][m] = NEW_MODEL_AXES[m]
    for m in list(model_axes["axes"].keys()):
        if m not in ALL_MODELS:
            del model_axes["axes"][m]
    ma_meta = model_axes.get("meta", {})
    ma_meta.setdefault("cost_per_1k_input_usd", {})
    for m in NEW_MODELS:
        ma_meta["cost_per_1k_input_usd"][m] = NEW_MODEL_AXES[m]["input_per_1k_usd"]

    clusters_sorted = sorted(quality_means["quality_means"].keys(), key=int)
    for m in NEW_MODELS:
        psi_probe = [quality_means["quality_means"][k][m] for k in clusters_sorted]
        model_features["models"][m] = {"operational": NEW_MODEL_AXES[m], "psi_probe": psi_probe}
    for m in list(model_features["models"].keys()):
        if m not in ALL_MODELS:
            del model_features["models"][m]
    mf_meta = model_features.get("meta", {})
    mf_meta["n_models"] = len(model_features["models"])
    mf_meta["roster_version"] = "v0.76"

    write_metadata(dst)

    save_json(dst / "model_registry.json", registry)
    save_json(dst / "quality_means.json", quality_means)
    save_json(dst / "model_axes.json", model_axes)
    save_json(dst / "model_features.json", model_features)

    shutil.copy2(src / "rankings.json", dst / "rankings.json")

    print(f"\nv0.76 bundle complete at {dst}")
    print(f"  {len(registry['deployed_models'])} models in registry")
    print(f"  {len(quality_means['quality_means'])} clusters in quality_means")
    print(f"  {len(model_axes['axes'])} models in model_axes")
    print(f"  {mf_meta['n_models']} models in model_features")

def write_metadata(dst: Path) -> None:
    import yaml
    src_meta_path = dst.parent / "v0.75" / "metadata.yaml"
    with open(src_meta_path, "r", encoding="utf-8") as f:
        meta = yaml.safe_load(f)
    meta["version"] = "v0.76"
    meta["parent"] = "v0.75"
    meta["status"] = "candidate"
    meta["deployed_providers"] = ["aiand"]
    meta["deployed_models"] = ALL_MODELS
    meta["cost_per_1k_input_usd"] = {
        "z-ai/glm-5.2": 0.0014, "deepseek/deepseek-v4-flash": 0.00014,
        "moonshotai/kimi-k2.7": 0.00095, "moonshotai/kimi-k3": 0.003,
        "motif-technologies/motif-3": 0.0005,
    }
    meta["training"]["roster_edit"] = {"added": NEW_MODELS, "parent": "v0.75", "tool": "build_v076_aiand_overlay.py", "version": "v0.76"}
    meta["changelog"] = (
        "v0.76 = v0.75 filtered to 5 ai& models + kimi-k3 + motif-3 added "
        "on FROZEN geometry. centroids.bin BYTE-IDENTICAL to v0.75. "
        "kimi-k3: quality = max(glm-5.2, flash, k2.7) x 1.03. "
        "motif-3: quality = glm-5.2 x 0.95 (inert). "
        "5-model ai&-only registry. NOT promoted to latest — use "
        "ROUTER_CLUSTER_VERSION=v0.76. Probe-validated multiplier."
    )
    # default_routing_knobs is PRESERVED from v0.75 (not stripped)
    with open(dst / "metadata.yaml", "w", encoding="utf-8") as f:
        yaml.dump(meta, f, default_flow_style=False, sort_keys=False)

if __name__ == "__main__":
    main()
