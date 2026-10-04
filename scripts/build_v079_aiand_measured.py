#!/usr/bin/env python3
"""Build artifacts/v0.79: AIand measured-label bundle (the training step).

Consumes the label files the RouterArena campaign produced
(.context/label-runs/<split>-labels.jsonl — per-(model, prompt) score rows
with REAL ground truth) and computes the scorer's quality cells from
measurement instead of researched anchors:

1. Embed every prompt with the runtime contract (jina-v2-base-code-int8,
   tail-truncate 1024 bytes, left-truncate 256 tokens, mean-pool, L2) and
   assign each prompt to its nearest centroid (argmax cosine) — the frozen
   v0.75/v0.78 geometry, byte-identical.
2. Per (cluster, model): mean score over the cluster's prompts, shrunk
   toward the model's global mean with shrinkage_k0=10 (the trainer's
   documented constant): cell = (n*mean + k0*global) / (n + k0).
3. Per-prompt z-score across the 8 model columns (score_normalization:
   per_prompt_zscore_across_bench_columns) BEFORE cluster aggregation —
   a prompt all 8 models get right (or all wrong) carries no signal and
   z-scores to 0, exactly like the original trainer.
4. Emit the v0.79 bundle from v0.78: quality_means cells replaced with
   measured values (8 AIand columns; non-AIand columns inherited
   unchanged), model_axes verbosity from measured token usage, rankings
   regenerated (parity-gated), registry flipped to measured:true,
   metadata changelog + parent v0.78. artifacts/latest NOT touched here.

Usage:
  python scripts/build_v079_aiand_measured.py --labels .context/label-runs/full-labels.jsonl
  python scripts/build_v079_aiand_measured.py --labels .context/label-runs/sub_10-labels.jsonl --split sub_10

Requires: numpy, onnxruntime, tokenizers, PyYAML. Deterministic.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import shutil
import sys
from pathlib import Path

import numpy as np

REPO_ROOT = Path(__file__).resolve().parents[1]
ART = REPO_ROOT / "internal" / "router" / "cluster" / "artifacts"
SRC = ART / "v0.78"
DST = ART / "v0.79"
ASSETS = REPO_ROOT / "assets-extracted" / "jina-v2-base-code-int8"

MODELS = [
    "deepseek-ai/deepseek-v4-pro",
    "deepseek-ai/deepseek-v4-flash",
    "deepseek-ai/deepseek-v4.1-flash",
    "zai-org/glm-5.3",
    "zai-org/glm-5.3-flash",
    "qwen/qwen3.8-27b",
    "moonshotai/kimi-k3",
    "motif-technologies/motif-3",
]

MAX_TOKENS = 256
MAX_PROMPT_BYTES = 1024
BATCH = 32
K = 16
SHRINKAGE_K0 = 10.0
RANKINGS_ALPHA = 0.96
EPS = 1e-9

# CRT1 centroids header (artifacts.go writeCentroids/readCentroids contract):
# magic[4] "CRT1", version uint8, k uint32, dim uint32, then k*dim float32.


def tail_truncate(s: str, max_bytes: int) -> str:
    b = s.encode("utf-8")
    if len(b) <= max_bytes:
        return s
    cut = max_bytes
    while cut > 0 and (b[cut] & 0xC0) == 0x80:
        cut -= 1
    return b[cut:].decode("utf-8")


def embed(texts: list[str]) -> np.ndarray:
    import onnxruntime as ort
    from tokenizers import Tokenizer

    tok = Tokenizer.from_file(str(ASSETS / "tokenizer.json"))
    sess = ort.InferenceSession(str(ASSETS / "model.onnx"), providers=["CPUExecutionProvider"])
    out = []
    for i in range(0, len(texts), BATCH):
        chunk = [tail_truncate(t, MAX_PROMPT_BYTES) for t in texts[i:i + BATCH]]
        enc = tok.encode_batch(chunk)
        input_ids = np.zeros((len(chunk), MAX_TOKENS), dtype=np.int64)
        attention = np.zeros((len(chunk), MAX_TOKENS), dtype=np.int64)
        for j, e in enumerate(enc):
            ids = e.ids[-MAX_TOKENS:]
            input_ids[j, :len(ids)] = ids
            attention[j, :len(ids)] = 1
        logits, = sess.run(None, {"input_ids": input_ids, "attention_mask": attention})
        mask = attention[:, :, None].astype(np.float32)
        pooled = (logits * mask).sum(axis=1) / np.clip(mask.sum(axis=1), 1, None)
        pooled /= np.clip(np.linalg.norm(pooled, axis=1, keepdims=True), EPS, None)
        out.append(pooled.astype(np.float32))
    return np.vstack(out)


def load_centroids() -> np.ndarray:
    raw = (SRC / "centroids.bin").read_bytes()
    assert raw[:4] == b"CRT1", "bad centroids magic"
    k = int.from_bytes(raw[5:9], "little")
    dim = int.from_bytes(raw[9:13], "little")
    assert k == K and dim == 768, f"unexpected geometry k={k} dim={dim}"
    arr = np.frombuffer(raw[13:13 + k * dim * 4], dtype="<f4").reshape(k, dim).copy()
    arr /= np.clip(np.linalg.norm(arr, axis=1, keepdims=True), EPS, None)
    return arr


def blend_row(qm_row, models, costs, alpha):
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
    ap = argparse.ArgumentParser()
    ap.add_argument("--labels", required=True, help="path to <split>-labels.jsonl")
    ap.add_argument("--split", default="full", help="campaign split (metadata only)")
    ap.add_argument("--skip-embed", action="store_true",
                    help="reuse .context/label-runs/prompt-clusters.json if present")
    args = ap.parse_args()

    # --- 1. load labels: (global_index -> {model: score})
    rows = [json.loads(l) for l in Path(args.labels).read_text(encoding="utf-8").splitlines() if l.strip()]
    by_prompt: dict[str, dict[str, float]] = {}
    verbosity: dict[str, tuple[float, float]] = {}  # model -> (n, sum_out_tokens)
    for r in rows:
        gi = r["global_index"]
        by_prompt.setdefault(gi, {})[r["model"]] = r["score"]
        n, s = verbosity.get(r["model"], (0, 0.0))
        verbosity[r["model"]] = (n + 1, s + (r.get("token_usage") or {}).get("output_tokens", 0))
    prompts = sorted(by_prompt)
    print(f"labels: {len(prompts)} prompts, {len(rows)} rows")

    # --- 2. cluster assignment (embed + argmax cosine)
    cache = REPO_ROOT / ".context" / "label-runs" / "prompt-clusters.json"
    if args.skip_embed and cache.exists():
        cl = json.loads(cache.read_text())
        assert set(cl) >= set(prompts), "cluster cache does not cover all labeled prompts"
        assign = {p: cl[p] for p in prompts}
    else:
        # need the prompt text: read from the labels' companion (the dataset
        # rows are keyed by global_index; the label rows carry the question)
        texts = {}
        for r in rows:
            texts.setdefault(r["global_index"], r.get("question") or "")
        ordered = [texts[p] for p in prompts]
        print(f"embedding {len(ordered)} prompts...")
        vecs = embed(ordered)
        cents = load_centroids()
        sims = vecs @ cents.T
        assign = {p: int(np.argmax(sims[i])) for i, p in enumerate(prompts)}
        cache.parent.mkdir(parents=True, exist_ok=True)
        cache.write_text(json.dumps(assign))

    # --- 3. per-prompt z-score across model columns, then shrunk cluster means
    # z per prompt over the models present; prompts with <2 models or no
    # variance contribute 0 for all.
    z_scores: dict[str, dict[str, float]] = {}
    for p, models_scores in by_prompt.items():
        ms = [m for m in MODELS if m in models_scores]
        if len(ms) < 2:
            z_scores[p] = {m: 0.0 for m in MODELS}
            continue
        vals = np.array([models_scores[m] for m in ms], dtype=np.float64)
        std = vals.std()
        if std < EPS:
            z_scores[p] = {m: 0.0 for m in MODELS}
            continue
        z = (vals - vals.mean()) / std
        z_scores[p] = {m: (float(z[i]) if m in models_scores else 0.0) for i, m in enumerate(ms)}

    # per-model global mean (over all prompts), then per-cluster shrunk mean
    global_mean = {m: float(np.mean([z_scores[p][m] for p in prompts])) for m in MODELS}
    cluster_cells = {k: {} for k in range(K)}
    for k in range(K):
        ps = [p for p in prompts if assign[p] == k]
        for m in MODELS:
            if not ps:
                cluster_cells[k][m] = 0.0
                continue
            mean = float(np.mean([z_scores[p][m] for p in ps]))
            cluster_cells[k][m] = (len(ps) * mean + SHRINKAGE_K0 * global_mean[m]) / (len(ps) + SHRINKAGE_K0)

    # --- 4. load v0.78 bundle, swap AIand cells, regenerate
    qm = json.loads((SRC / "quality_means.json").read_text(encoding="utf-8"))
    axes = json.loads((SRC / "model_axes.json").read_text(encoding="utf-8"))
    reg = json.loads((SRC / "model_registry.json").read_text(encoding="utf-8"))
    rk = json.loads((SRC / "rankings.json").read_text(encoding="utf-8"))

    all_models = sorted(qm["quality_means"]["0"].keys())
    costs_all = {m: (axes["axes"][m]["input_per_1k_usd"] or 0.0) for m in all_models}
    # parity gate on v0.78's 8-model rankings
    for k in range(K):
        got = blend_row(qm["quality_means"][str(k)], MODELS, {m: costs_all[m] for m in MODELS}, RANKINGS_ALPHA)
        for m in MODELS:
            assert abs(got[m] - rk["rankings"][str(k)][m]) < 1e-12, (
                f"v0.78 rankings parity failed at cluster {k} model {m}")

    # scale measured z-cells into each cluster's existing [min,max] range so
    # the blend's min-max normalization sees comparable magnitudes
    for k in range(K):
        row = qm["quality_means"][str(k)]
        lo, hi = min(row.values()), max(row.values())
        cells = cluster_cells[k]
        cmin, cmax = min(cells.values()), max(cells.values())
        for m in MODELS:
            if cmax - cmin < EPS:
                row[m] = (lo + hi) / 2
            else:
                t = (cells[m] - cmin) / (cmax - cmin)
                row[m] = lo + t * (hi - lo)

    # verbosity: measured output tokens per model (median-imputed for absent)
    med = float(np.median([s / n for n, s in verbosity.values() if n]))
    for m in MODELS:
        n, s = verbosity.get(m, (0, 0.0))
        axes["axes"][m]["verbosity_tokens"] = (s / n) if n else med

    # rankings regenerate over the 8
    costs8 = {m: costs_all[m] for m in MODELS}
    rk["rankings"] = {
        str(k): blend_row(qm["quality_means"][str(k)], MODELS, costs8, RANKINGS_ALPHA)
        for k in range(K)
    }

    # registry: measured:true
    for e in reg["deployed_models"]:
        e["bench_column"] = f"routerarena_measured_{args.split}"
        e["measured"] = True
        e["measured_split"] = args.split
    reg["meta"]["parent"] = "v0.78"
    reg["meta"]["last_refreshed"] = "2026-10-05"

    import yaml
    meta = yaml.safe_load((SRC / "metadata.yaml").read_text(encoding="utf-8"))
    meta["version"] = "v0.79"
    meta["parent"] = "v0.78"
    meta["changelog"] = (
        f"v0.79 = v0.78 with the 8 AIand quality columns MEASURED from the RouterArena "
        f"campaign ({args.split} split, real ground-truth grading via RouterArena's own "
        "evaluators; scripts/build_v079_aiand_measured.py). Pipeline: embed prompts with "
        "the runtime contract (jina-v2-base-code-int8), assign clusters by centroid argmax "
        "(frozen geometry), per-prompt z-score across the 8 model columns, per-(cluster, "
        f"model) shrunk means (shrinkage_k0={SHRINKAGE_K0:g}), min-max rescaled into each "
        "cluster's existing range. model_axes verbosity from measured output tokens. "
        "centroids BYTE-IDENTICAL. rankings regenerated (parity-gated on v0.78 first). "
        "Measured cells replace v0.78's researched anchors — the comparison report is in "
        "the parent issue.\n\n" + meta["changelog"])

    DST.mkdir(exist_ok=True)
    shutil.copyfile(SRC / "centroids.bin", DST / "centroids.bin")
    (DST / "quality_means.json").write_text(json.dumps(qm, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    (DST / "model_axes.json").write_text(json.dumps(axes, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    (DST / "rankings.json").write_text(json.dumps(rk, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    (DST / "model_registry.json").write_text(json.dumps(reg, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    with open(DST / "metadata.yaml", "w", encoding="utf-8", newline="\n") as f:
        yaml.safe_dump(meta, f, sort_keys=False, allow_unicode=True, width=100)

    features = {
        "meta": {
            "comment": "v0.79: v0.78 geometry + AIand MEASURED columns (build_v079_aiand_measured.py).",
            "k": K,
            "n_models": len(all_models),
            "roster_version": "v0.79",
            "source": "quality_means.json + model_axes.json",
        },
        "models": {
            m: {
                "psi_probe": [qm["quality_means"][str(k)][m] for k in range(K)],
                "operational": axes["axes"][m],
            } for m in all_models
        },
    }
    (DST / "model_features.json").write_text(json.dumps(features, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")

    sha = hashlib.sha256((SRC / "centroids.bin").read_bytes()).hexdigest()
    assert sha == hashlib.sha256((DST / "centroids.bin").read_bytes()).hexdigest()
    print("wrote", DST)
    print("cluster sizes:", {k: sum(1 for p in prompts if assign[p] == k) for k in range(K)})


if __name__ == "__main__":
    main()
