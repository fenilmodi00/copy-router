#!/usr/bin/env python3
"""Build artifacts/v0.81: corpus-matched re-clustering from measured prompts (#76).

The v0.75/v0.78 geometry was trained on a coding-heavy corpus while the
serving corpus (RouterArena sub_10) is knowledge-heavy: six of sixteen
clusters hold 1-7 labeled prompts, so their quality cells are noise.
This ticket deliberately releases the frozen-geometry constraint — the
re-cluster and the labels are the SAME corpus, so there is no train/serve
mismatch to protect against, and the runtime embedder is untouched.

Pipeline (mirrors scripts/build_v079_aiand_measured.py exactly where the
two overlap):

1. Embed every labeled prompt with the runtime contract
   (jina-v2-base-code-int8, tail-truncate 1024 bytes, left-truncate 256
   tokens, mean-pool, L2) — identical to the v0.79 builder's embed().
2. Cosine k-means (K=16, k-means++ with D^2 weighting, 10 restarts,
   seed 42, pure numpy Lloyd's on the L2-normalized embeddings).
3. Health check: per-cluster labeled-prompt counts; the ticket bar is
   no cluster < 20. A tiny cluster is merged into its nearest neighbor
   center and re-assigned (merges are documented in the analysis dump).
4. Per-(cluster, model) quality cells from the measured labels with the
   builder's exact filter: per-prompt z-score across the 8 model columns
   (prompts with <2 scored models or zero variance carry no signal),
   shrunk toward the model's global mean with shrinkage_k0=10, then
   min-max rescaled into each cluster's existing [min,max] range.
5. Replay the labeled corpus (cluster-size-weighted, the anchor
   methodology): per prompt, nearest-centroid assignment -> cluster
   winner under the alpha-blend -> the winner's measured score/cost.
   Compared against v0.79-geometry replays under the same knobs.
6. VERDICT: ship v0.81 only if replay accuracy > v0.80's AND cost <=
   v0.80's; otherwise emit the negative result and keep v0.80.

Usage (sandbox, /work/build repo root):
  python3 scripts/build_v081_recluster.py \
      --labels labels/sub_10-labels.jsonl --split sub_10

Requires: numpy, onnxruntime, tokenizers, PyYAML. Deterministic.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import shutil
from pathlib import Path

import numpy as np
import yaml

REPO_ROOT = Path(__file__).resolve().parents[1]
ART = REPO_ROOT / "internal" / "router" / "cluster" / "artifacts"
LABELS_DIR = REPO_ROOT / "labels"
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
EPS = 1e-9

# v0.79's committed uniform ranking alpha (validates the replay harness
# against the committed rankings.json) and the v0.79 default knob vector
# (per-cluster alpha; clusters 2, 9, 10, 11, 15 demoted to 0.80).
RANKINGS_ALPHA_V079 = 0.96
V079_KNOB_ALPHA = [0.96, 0.96, 0.8, 0.96, 0.96, 0.96, 0.96, 0.96,
                   0.96, 0.8, 0.8, 0.8, 0.96, 0.96, 0.96, 0.8]

# T1 (v0.80) retune is not yet committed as of this build; the likely
# outcome per the parent plan is a uniform alpha=0.92. ASSUMPTION.
ALPHA_V080 = 0.92

# Ticket health bar and k-means hyperparameters (ticket spec).
MIN_CLUSTER_LABELS = 20
KM_RESTARTS = 10
KM_SEED = 42
TOP_P = 4  # runtime Config.TopP (metadata training.top_p)


# ---------------------------------------------------------------------------
# Runtime-contract embedding (byte-identical logic to build_v079 builder).
# ---------------------------------------------------------------------------

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


# ---------------------------------------------------------------------------
# CRT1 centroids (artifacts.go writeCentroids/readCentroids contract):
# magic[4] "CRT1", version uint32, k uint32, dim uint32, then k*dim float32.
# ---------------------------------------------------------------------------

def read_centroids(path: Path) -> np.ndarray:
    raw = path.read_bytes()
    assert raw[:4] == b"CRT1", "bad centroids magic"
    version = int.from_bytes(raw[4:8], "little")
    assert version == 1, f"unsupported centroids version {version}"
    k = int.from_bytes(raw[8:12], "little")
    dim = int.from_bytes(raw[12:16], "little")
    assert k == K and dim == 768, f"unexpected geometry k={k} dim={dim}"
    arr = np.frombuffer(raw[16:16 + k * dim * 4], dtype="<f4").reshape(k, dim).copy()
    arr /= np.clip(np.linalg.norm(arr, axis=1, keepdims=True), EPS, None)
    return arr


def write_centroids(path: Path, centers: np.ndarray) -> None:
    centers = np.asarray(centers, dtype="<f4")
    assert centers.shape == (K, 768)
    centers = centers / np.clip(np.linalg.norm(centers, axis=1, keepdims=True), EPS, None)
    raw = b"CRT1" + (1).to_bytes(4, "little") + K.to_bytes(4, "little") + (768).to_bytes(4, "little")
    raw += centers.astype("<f4").tobytes()
    path.write_bytes(raw)


# ---------------------------------------------------------------------------
# Cosine k-means (spherical Lloyd's, k-means++ init, pure numpy).
# Vectors are already L2-normalized, so cosine = dot.
# ---------------------------------------------------------------------------

def kmeanspp_init(X: np.ndarray, k: int, rng: np.random.Generator) -> np.ndarray:
    n = len(X)
    first = int(rng.integers(n))
    cents = [X[first]]
    d2 = (1.0 - X @ X[first]) ** 2  # squared cosine distance
    for _ in range(1, k):
        probs = d2 / d2.sum()
        idx = int(rng.choice(n, p=probs))
        cents.append(X[idx])
        d2 = np.min((1.0 - X @ np.stack(cents).T) ** 2, axis=1)
    return np.stack(cents)


def lloyd(X: np.ndarray, cents: np.ndarray, max_iter: int = 100) -> tuple[np.ndarray, np.ndarray, float]:
    k = len(cents)
    assign = np.zeros(len(X), dtype=np.int64)
    for _ in range(max_iter):
        sims = X @ cents.T
        new_assign = sims.argmax(axis=1)
        if np.array_equal(new_assign, assign):
            break
        assign = new_assign
        counts = np.bincount(assign, minlength=k)
        sums = np.zeros_like(cents)
        np.add.at(sums, assign, X)
        new_cents = sums / np.maximum(counts, 1)[:, None]
        # Empty clusters keep their previous center.
        new_cents[counts == 0] = cents[counts == 0]
        cents = new_cents / np.clip(np.linalg.norm(new_cents, axis=1, keepdims=True), EPS, None)
    sims = X @ cents.T
    inertia = float((1.0 - sims.max(axis=1)).sum())
    return cents, sims.argmax(axis=1), inertia


def kmeans_cosine(X: np.ndarray, k: int, restarts: int, seed: int) -> tuple[np.ndarray, np.ndarray, float]:
    best = None
    for r in range(restarts):
        rng = np.random.default_rng(seed + r)
        cents = kmeanspp_init(X, k, rng)
        cents, assign, inertia = lloyd(X, cents)
        if best is None or inertia < best[2] - 1e-12:
            best = (cents, assign, inertia)
    return best  # type: ignore[return-value]


# ---------------------------------------------------------------------------
# Blend (identical to build_v079 builder / runtime blendScoresV2 with
# speed_weight=0, output_cost_ratio=0, per_model_verbosity irrelevant
# because output_cost_ratio=0).
# ---------------------------------------------------------------------------

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


def cluster_winner(qm_row, alpha, costs, order):
    """Single-cluster runtime blend winner (tie-break: registry order)."""
    row = blend_row(qm_row, MODELS, costs, alpha)
    best_m, best_v = None, None
    for m in order:
        if best_m is None or row[m] > best_v:
            best_m, best_v = m, row[m]
    return best_m, row


def topP_winner(vec, cents, qm_rows, alpha_vec, costs, order, top_p=TOP_P):
    """Runtime-exact path: blend summed over the top-P nearest clusters."""
    sims = cents @ vec
    # topPNearest: sort by sim desc, tie-break lower index, take p.
    idx = sorted(range(len(cents)), key=lambda i: (-sims[i], i))[:top_p]
    scores = {}
    cmin, cmax = min(costs.values()), max(costs.values())
    cr = cmax - cmin
    for m in order:
        s = 0.0
        for k in idx:
            row = qm_rows[k]
        qmin = min(row[m2] for m2 in MODELS)
        qmax = max(row[m2] for m2 in MODELS)
        qr = qmax - qmin
        qn = (row[m] - qmin) / qr if qr > 0 else 0.0
        cn = (costs[m] - cmin) / cr if cr > 0 else 0.0
        a = alpha_vec[k]
        s += a * qn + (1.0 - a) * (1.0 - cn)
        scores[m] = s
    best_m, best_v = None, None
    for m in order:
        if best_m is None or scores[m] > best_v:
            best_m, best_v = m, scores[m]
    return best_m


# ---------------------------------------------------------------------------
# Replay — T1's exact convention, both aggregations
# (scripts/build_v080_cost_retune.py, replay()):
#   V1 — the ticket convention: cluster-size-weighted mean of the
#        winner's measured accuracy/cost over each cluster's labeled
#        prompts; clusters whose winner was never measured drop out
#        and the denominator reweights over measured clusters.
#   V2 — per-prompt aggregation (reproduces the campaign anchors:
#        oracle 74.3%, constant v4.1-flash 69.3% @ $0.34/1k).
# ---------------------------------------------------------------------------

def replay(assign, cents, qm_rows, alpha_vec, costs, order, by_prompt, prompts):
    k_count = len(cents)
    winners = {}
    for k in range(k_count):
        a = alpha_vec[k] if isinstance(alpha_vec, list) else alpha_vec
        winners[k], _ = cluster_winner(qm_rows[k], a, costs, order)
    cl_ps = {k: [i for i in range(len(prompts)) if int(assign[i]) == k]
             for k in range(k_count)}
    table = {}
    num_a = num_c = wn = 0.0
    for k in range(k_count):
        ps = cl_ps[k]
        w = winners[k]
        idx = [i for i in ps
               if w in by_prompt[prompts[i]]
               and by_prompt[prompts[i]][w].get("score") is not None]
        sc = [by_prompt[prompts[i]][w]["score"] for i in idx]
        co = [by_prompt[prompts[i]][w]["cost"] for i in idx]
        table[str(k)] = {
            "n_labeled": len(ps),
            "n_scored": len(sc),
            "winner": w,
            "accuracy": float(np.mean(sc)) if sc else None,
            "cost_per_1k_prompts_usd": 1000.0 * float(np.mean(co)) if co else None,
        }
        if not sc:
            continue  # winner never measured here: no measured accuracy to weight
        num_a += len(ps) * float(np.mean(sc))
        num_c += len(ps) * float(np.mean(co))
        wn += len(ps)
    hits = cost = nh = 0
    for i, p in enumerate(prompts):
        w = winners[int(assign[i])]
        r = by_prompt[p].get(w)
        if r is None or r.get("score") is None:
            continue
        nh += 1
        hits += r["score"]
        cost += r["cost"]
    return {
        "n_prompts": len(prompts),
        "n_scored_v2": nh,
        "measured_weight_v1": wn / len(prompts),
        "acc_v1": num_a / wn,
        "cost_per_1k_v1": num_c / wn * 1000.0,
        "acc_v2": hits / nh,
        "cost_per_1k_v2": cost / nh * 1000.0,
        "winners": winners,
        "clusters": table,
    }


# ---------------------------------------------------------------------------
# Main.
# ---------------------------------------------------------------------------

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--labels", required=True, help="path to <split>-labels.jsonl")
    ap.add_argument("--split", default="sub_10")
    ap.add_argument("--src", default=None,
                    help="parent bundle version (default: v0.80 when committed, else v0.79)")
    ap.add_argument("--dst", default="v0.81")
    ap.add_argument("--alpha-v080-vector", default=None,
                    help="comma-separated per-cluster alpha vector applied to BOTH baseline "
                         "and candidate (default: read from the committed v0.80 metadata "
                         "when present, else uniform 0.92 with a printed assumption)")
    ap.add_argument("--min-cluster-labels", type=int, default=MIN_CLUSTER_LABELS)
    ap.add_argument("--restarts", type=int, default=KM_RESTARTS)
    ap.add_argument("--seed", type=int, default=KM_SEED)
    ap.add_argument("--emit", action="store_true",
                    help="write the dst bundle (only when the verdict ships)")
    args = ap.parse_args()

    # parent bundle: v0.80 once committed (its non-knob files are byte-identical
    # to v0.79's; only the metadata knobs and rankings.json differ)
    if args.src is None:
        args.src = "v0.80" if (ART / "v0.80" / "metadata.yaml").exists() else "v0.79"
    SRC = ART / args.src
    DST = ART / args.dst

    # per-cluster knobs applied to BOTH sides of the comparison: the committed
    # v0.80 bundle's vector when present, else the uniform fallback
    if args.alpha_v080_vector:
        alpha_vec = [float(x) for x in args.alpha_v080_vector.split(",")]
        if len(alpha_vec) != K:
            raise SystemExit(f"--alpha-v080-vector needs {K} values, got {len(alpha_vec)}")
        alpha_src = "cli"
    else:
        v080_meta = ART / "v0.80" / "metadata.yaml"
        if v080_meta.exists():
            alpha_vec = list(yaml.safe_load(v080_meta.read_text(encoding="utf-8"))
                             ["training"]["default_routing_knobs"]["alpha"])
            alpha_src = "artifacts/v0.80/metadata.yaml (T1 per-cluster-knapsack)"
        else:
            alpha_vec = [ALPHA_V080] * K
            alpha_src = f"ASSUMED uniform alpha={ALPHA_V080} (v0.80 not committed)"
    print(f"applied knobs [{alpha_src}]: alpha={[round(a, 4) for a in alpha_vec]}")
    # --- 1. load labels: (global_index -> {model: row})
    rows = [json.loads(l) for l in Path(args.labels).read_text(encoding="utf-8").splitlines() if l.strip()]
    by_prompt: dict[str, dict[str, dict]] = {}
    verbosity: dict[str, tuple[int, float]] = {}
    for r in rows:
        gi = r["global_index"]
        by_prompt.setdefault(gi, {})[r["model"]] = r
        n, s = verbosity.get(r["model"], (0, 0.0))
        verbosity[r["model"]] = (n + 1, s + (r.get("token_usage") or {}).get("output_tokens", 0))
    prompts = sorted(by_prompt)
    print(f"labels: {len(prompts)} prompts, {len(rows)} rows")

    # --- 2. embed every labeled prompt (runtime contract)
    texts = {gi: (next(iter(by_prompt[gi].values())).get("question") or "") for gi in prompts}
    ordered = [texts[p] for p in prompts]
    print(f"embedding {len(ordered)} prompts...")
    vecs = embed(ordered)

    # --- 3. BEFORE: current-geometry assignment (the ticket's baseline)
    old_cents = read_centroids(SRC / "centroids.bin")
    old_assign = (vecs @ old_cents.T).argmax(axis=1)
    old_counts = np.bincount(old_assign, minlength=K).tolist()
    print("BEFORE (v0.79 geometry) cluster label counts:", old_counts)

    # --- 4. k-means over the measured-prompt embeddings
    print(f"k-means: K={K}, k-means++, {args.restarts} restarts, seed {args.seed}, cosine")
    cents, assign, inertia = kmeans_cosine(vecs, K, args.restarts, args.seed)
    counts = np.bincount(assign, minlength=K).tolist()
    print("AFTER (k-means) cluster label counts:", counts)

    # --- 5. health check + merge pass (ticket bar: no cluster < 20)
    merges = []
    max_merges = 2 * K  # guard against pathological oscillation
    while True:
        counts = np.bincount(assign, minlength=len(cents)).tolist()
        small = [c for c in range(len(cents)) if counts[c] < args.min_cluster_labels]
        if not small:
            break
        if len(merges) >= max_merges:
            raise SystemExit(f"merge pass did not converge after {max_merges} merges: {counts}")
        c = min(small, key=lambda c: counts[c])
        others = [c2 for c2 in range(len(cents)) if c2 != c]
        nearest = max(others, key=lambda c2: float(cents[c] @ cents[c2]))
        merges.append({"merged_cluster": c, "into": nearest, "labeled_prompts": counts[c]})
        print(f"MERGE: cluster {c} ({counts[c]} labeled prompts) -> nearest neighbor {nearest}")
        cents = np.stack([cents[i] for i in range(len(cents)) if i != c])
        assign = (vecs @ cents.T).argmax(axis=1)
        if len(cents) < K:
            # Restore K=16 (the CRT1 header and the runtime pin K):
            # re-split the largest cluster with 2-means.
            counts2 = np.bincount(assign, minlength=len(cents))
            biggest = int(counts2.argmax())
            members = vecs[assign == biggest]
            rng = np.random.default_rng(args.seed)
            init = members[rng.choice(len(members), 2, replace=False)]
            sub, _, _ = lloyd(members, init, max_iter=100)
            cents = np.vstack([cents, sub[1][None, :]])
            assign = (vecs @ cents.T).argmax(axis=1)
            print(f"RESTORE: re-split largest cluster {biggest} to restore K={K}")

    counts = np.bincount(assign, minlength=K).tolist()
    print("FINAL cluster label counts:", counts)
    assert len(cents) == K, f"K drifted to {len(cents)}"
    assert min(counts) >= args.min_cluster_labels, f"health bar failed: {counts}"

    # --- 6. z-score filter (builder-identical), shrunk cluster means
    z_scores: dict[str, dict[str, float]] = {}
    for p, models_rows in by_prompt.items():
        scored = {m: v["score"] for m, v in models_rows.items() if v.get("score") is not None}
        ms = [m for m in MODELS if m in scored]
        if len(ms) < 2:
            continue  # no signal: exclude the prompt entirely
        vals = np.array([scored[m] for m in ms], dtype=np.float64)
        std = vals.std()
        if std < EPS:
            continue  # all models agree: no signal
        z = (vals - vals.mean()) / std
        z_scores[p] = {m: float(z[i]) for i, m in enumerate(ms)}
    z_prompts = [p for p in prompts if p in z_scores]
    print(f"z-scored prompts: {len(z_prompts)} of {len(by_prompt)}")

    z_assign = np.array([int(assign[prompts.index(p)]) for p in z_prompts])
    z_counts = np.bincount(z_assign, minlength=K).tolist()
    print("z-scored prompts per cluster:", z_counts)

    global_mean = {m: float(np.mean([z_scores[p].get(m, 0.0) for p in z_prompts])) for m in MODELS}
    cluster_cells = {k: {} for k in range(K)}
    for k in range(K):
        ps = [p for p in z_prompts if int(assign[prompts.index(p)]) == k]
        for m in MODELS:
            if not ps:
                cluster_cells[k][m] = 0.0
                continue
            mean = float(np.mean([z_scores[p].get(m, 0.0) for p in ps]))
            cluster_cells[k][m] = (len(ps) * mean + SHRINKAGE_K0 * global_mean[m]) / (len(ps) + SHRINKAGE_K0)


    # --- 7. parent bundle: swap the 8 AIand cells, rescale into the
    # parent's per-cluster [min,max] range (builder-identical)
    qm = json.loads((SRC / "quality_means.json").read_text(encoding="utf-8"))
    axes = json.loads((SRC / "model_axes.json").read_text(encoding="utf-8"))
    reg = json.loads((SRC / "model_registry.json").read_text(encoding="utf-8"))
    rk = json.loads((SRC / "rankings.json").read_text(encoding="utf-8"))
    all_models = sorted(qm["quality_means"]["0"].keys())
    costs_all = {m: (axes["axes"][m]["input_per_1k_usd"] or 0.0) for m in all_models}
    costs8 = {m: costs_all[m] for m in MODELS}

    # parity gates: the blend formula must reproduce the committed
    # rankings before we trust it for the new bundle.
    # (a) v0.79's rankings are the uniform alpha=0.96 record (canonical).
    rk079 = json.loads((ART / "v0.79" / "rankings.json").read_text(encoding="utf-8"))
    for k in range(K):
        got = blend_row(qm["quality_means"][str(k)], MODELS, costs8, RANKINGS_ALPHA_V079)
        for m in MODELS:
            assert abs(got[m] - rk079["rankings"][str(k)][m]) < 1e-12, (
                f"v0.79 rankings parity failed at cluster {k} model {m}")
    print("parity gate: blend(0.96) reproduces v0.79 rankings (canonical record)")
    # (b) the committed v0.80 rankings are the applied-knob record.
    if args.src == "v0.80":
        for k in range(K):
            got = blend_row(qm["quality_means"][str(k)], MODELS, costs8, alpha_vec[k])
            for m in MODELS:
                assert abs(got[m] - rk["rankings"][str(k)][m]) < 1e-12, (
                    f"v0.80 rankings parity failed at cluster {k} model {m}")
        print("parity gate: applied knob vector reproduces v0.80 rankings")
    # registry order = runtime argmax tie-break order
    order = [e["model"] for e in reg["deployed_models"]]

    # min-max rescale the measured z-cells into each cluster's existing range
    new_qm = {"meta": qm.get("meta", {}), "quality_means": {}}
    for k in range(K):
        row = dict(qm["quality_means"][str(k)])
        lo, hi = min(row.values()), max(row.values())
        cells = cluster_cells[k]
        cmin, cmax = min(cells.values()), max(cells.values())
        for m in MODELS:
            if cmax - cmin < EPS:
                row[m] = (lo + hi) / 2
            else:
                t = (cells[m] - cmin) / (cmax - cmin)
                row[m] = lo + t * (hi - lo)
        new_qm["quality_means"][str(k)] = row

    # verbosity: measured output tokens per model (median-imputed for absent)
    med = float(np.median([s / n for n, s in verbosity.values() if n]))
    new_axes = {"meta": axes.get("meta", {}), "axes": {}}
    for m, ax in axes["axes"].items():
        new_axes["axes"][m] = dict(ax)
    for m in MODELS:
        n, s = verbosity.get(m, (0, 0.0))
        new_axes["axes"][m]["verbosity_tokens"] = (s / n) if n else med

    # --- 8. replays
    # (a) harness validation: v0.79 geometry + committed uniform alpha
    val = replay(old_assign, old_cents, [qm["quality_means"][str(k)] for k in range(K)],
                 [RANKINGS_ALPHA_V079] * K, costs8, order, by_prompt, prompts)
    # (b) v0.79 geometry + v0.79's real per-cluster knob vector
    val_knobs = replay(old_assign, old_cents, [qm["quality_means"][str(k)] for k in range(K)],
                       V079_KNOB_ALPHA, costs8, order, by_prompt, prompts)
    # (c) v0.80 baseline: OLD geometry + the applied knob vector
    base = replay(old_assign, old_cents, [qm["quality_means"][str(k)] for k in range(K)],
                  alpha_vec, costs8, order, by_prompt, prompts)
    # (d) v0.81 candidate: NEW geometry + the same knob vector
    cand = replay(assign, cents, [new_qm["quality_means"][str(k)] for k in range(K)],
                  alpha_vec, costs8, order, by_prompt, prompts)
    # (e) runtime-exact top-P blends (production path, secondary check)
    qm_rows_old = [qm["quality_means"][str(k)] for k in range(K)]
    qm_rows_new = [new_qm["quality_means"][str(k)] for k in range(K)]
    for name, a_assign, a_cents, a_rows, a_alpha in [
        ("v079_geometry_v079_knobs_topP", old_assign, old_cents, qm_rows_old, V079_KNOB_ALPHA),
        ("v079_geometry_v080_alpha_topP", old_assign, old_cents, qm_rows_old, alpha_vec),
        ("v081_geometry_v080_alpha_topP", assign, cents, qm_rows_new, alpha_vec),
    ]:
        scores, costs_seen = [], []
        for i, p in enumerate(prompts):
            w = topP_winner(vecs[i], a_cents, a_rows, a_alpha, costs8, order)
            r = by_prompt[p].get(w)
            if r is None or r.get("score") is None:
                continue
            scores.append(r["score"])
            costs_seen.append(r["cost"])
        cand.setdefault("topP_checks", {})[name] = {
            "n_scored": len(scores),
            "accuracy": float(np.mean(scores)) if scores else 0.0,
            "cost_per_1k_prompts_usd": 1000.0 * float(np.mean(costs_seen)) if costs_seen else 0.0,
        }
        print(f"topP replay {name}: acc={cand['topP_checks'][name]['accuracy']:.4f} "
              f"cost/1k=${cand['topP_checks'][name]['cost_per_1k_prompts_usd']:.2f}")

    print("VALIDATION v0.79 geometry, uniform alpha 0.96 "
          f"(V1 acc={val['acc_v1']:.4f} @ ${val['cost_per_1k_v1']:.2f}/1k; "
          f"V2 acc={val['acc_v2']:.4f} @ ${val['cost_per_1k_v2']:.2f}/1k)")
    print("VALIDATION v0.79 geometry, v0.79 knob vector "
          f"(V1 acc={val_knobs['acc_v1']:.4f} @ ${val_knobs['cost_per_1k_v1']:.2f}/1k; "
          f"V2 acc={val_knobs['acc_v2']:.4f} @ ${val_knobs['cost_per_1k_v2']:.2f}/1k)")
    print("BASELINE  v0.79 geometry (v0.80 knobs, applied vector) "
          f"(V1 acc={base['acc_v1']:.4f} @ ${base['cost_per_1k_v1']:.2f}/1k; "
          f"V2 acc={base['acc_v2']:.4f} @ ${base['cost_per_1k_v2']:.2f}/1k)")
    print("CANDIDATE v0.81 geometry (same knobs) "
          f"(V1 acc={cand['acc_v1']:.4f} @ ${cand['cost_per_1k_v1']:.2f}/1k; "
          f"V2 acc={cand['acc_v2']:.4f} @ ${cand['cost_per_1k_v2']:.2f}/1k)")

    # --- 9. verdict: ship only if replay accuracy improves without
    # cost regression under BOTH aggregations (V1 = the ticket
    # convention, V2 = the campaign-anchor convention)
    ship = (cand["acc_v1"] > base["acc_v1"]
            and cand["acc_v2"] > base["acc_v2"]
            and cand["cost_per_1k_v1"] <= base["cost_per_1k_v1"]
            and cand["cost_per_1k_v2"] <= base["cost_per_1k_v2"])
    print("VERDICT:", "SHIP v0.81" if ship else "NEGATIVE — keep v0.80 geometry")
    if not ship:
        print(f"  deltas: V1 acc {cand['acc_v1'] - base['acc_v1']:+.4f} "
              f"cost {cand['cost_per_1k_v1'] - base['cost_per_1k_v1']:+.2f}; "
              f"V2 acc {cand['acc_v2'] - base['acc_v2']:+.4f} "
              f"cost {cand['cost_per_1k_v2'] - base['cost_per_1k_v2']:+.2f}")

    analysis = {
        "ticket": "#76 T2",
        "split": args.split,
        "labels": str(args.labels),
        "src_bundle": args.src,
        "applied_alpha_vector": alpha_vec,
        "alpha_vector_source": alpha_src,
        "kmeans": {
            "k": K, "restarts": args.restarts, "seed": args.seed,
            "init": "k-means++ (D^2 cosine distance weighting)",
            "inertia": inertia,
        },
        "merges": merges,
        "cluster_label_counts_before": old_counts,
        "cluster_label_counts_after": counts,
        "z_scored_prompts_per_cluster": z_counts,
        "z_scored_total": len(z_prompts),
        "global_mean_z": global_mean,
        "replay_validation_v079_uniform": {k: v for k, v in val.items() if k != "clusters"},
        "replay_validation_v079_knobs": {k: v for k, v in val_knobs.items() if k != "clusters"},
        "replay_v080_baseline": {k: v for k, v in base.items() if k != "clusters"},
        "replay_v081_candidate": {k: v for k, v in cand.items() if k != "clusters"},
        "v080_baseline_clusters": base["clusters"],
        "v081_candidate_clusters": cand["clusters"],
        "verdict": "ship" if ship else "negative",
    }
    out = Path(args.labels).parent / "v081-analysis.json"
    out.write_text(json.dumps(analysis, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print("wrote", out)

    # --- 10. emit the v0.81 bundle (only on a ship verdict)
    if not ship:
        print("negative verdict: bundle NOT emitted (keep v0.80 geometry)")
        return

    if not args.emit:
        print("ship verdict but --emit not passed; bundle NOT written")
        return

    # rankings regenerated over the 8 with the applied knob vector
    new_rankings = {
        "meta": dict(rk.get("meta", {})),
        "rankings": {
            str(k): blend_row(new_qm["quality_means"][str(k)], MODELS, costs8, alpha_vec[k])
            for k in range(K)
        },
    }
    new_rankings["meta"]["alpha_per_cluster"] = list(alpha_vec)
    new_rankings["meta"]["alpha"] = float(sum(alpha_vec) / len(alpha_vec))
    new_rankings["meta"]["regenerated_by"] = "scripts/build_v081_recluster.py"
    new_rankings["meta"]["note"] = (
        f"{args.dst}: rankings over the re-clustered geometry; per-cluster "
        f"alpha = the applied {args.src} knob vector (positional).")

    # registry: measured:true, re-keyed to this build's split
    for e in reg["deployed_models"]:
        e["bench_column"] = f"routerarena_measured_{args.split}"
        e["measured"] = True
        e["measured_split"] = args.split
    reg["meta"]["parent"] = args.src
    reg["meta"]["last_refreshed"] = "2026-10-06"

    meta = yaml.safe_load((SRC / "metadata.yaml").read_text(encoding="utf-8"))
    meta["version"] = args.dst
    meta["parent"] = args.src
    meta["training"] = dict(meta.get("training", {}))
    meta["training"]["n_prompts"] = len(prompts)
    meta["training"]["seed"] = args.seed
    meta["training"]["n_restarts"] = args.restarts
    meta["training"]["include_routerarena_labels"] = f"{args.split}-labels.jsonl"
    meta["training"]["reclustering"] = {
        "method": "cosine k-means over the labeled-prompt embeddings (ticket #76)",
        "cluster_label_counts": counts,
        "z_scored_prompts_per_cluster": z_counts,
        "merges": merges,
        "min_cluster_labels": args.min_cluster_labels,
        "replay": {
            "method": "T1's exact dual-aggregation replay over the "
                      "labeled corpus (nearest-centroid assignment, "
                      "per-cluster alpha-blend winner, the winner's "
                      "measured score/cost)",
            "aggregations": {
                "v1": "cluster-size-weighted (the ticket convention): "
                      "sum_k len(cluster_k) * mean(winner's measured "
                      "scores), over clusters where the winner was "
                      "measured; denominator reweights over measured clusters",
                "v2": "per-prompt aggregation (reproduces the campaign "
                      "anchors: oracle 74.3%, constant v4.1-flash 69.3%)",
            },
            "applied_alpha_vector": list(alpha_vec),
            "alpha_vector_source": alpha_src,
            "n_prompts": len(prompts),
            "n_scored_v2": cand["n_scored_v2"],
            "measured_weight_v1": cand["measured_weight_v1"],
            "accuracy_v1": cand["acc_v1"],
            "cost_per_1k_prompts_usd_v1": cand["cost_per_1k_v1"],
            "accuracy_v2": cand["acc_v2"],
            "cost_per_1k_prompts_usd_v2": cand["cost_per_1k_v2"],
            "baseline": f"{args.src} geometry, same per-cluster knob vector",
            "baseline_accuracy_v1": base["acc_v1"],
            "baseline_cost_per_1k_prompts_usd_v1": base["cost_per_1k_v1"],
            "baseline_accuracy_v2": base["acc_v2"],
            "baseline_cost_per_1k_prompts_usd_v2": base["cost_per_1k_v2"],
            "delta_accuracy_v1": cand["acc_v1"] - base["acc_v1"],
            "delta_cost_per_1k_prompts_usd_v1": cand["cost_per_1k_v1"] - base["cost_per_1k_v1"],
            "delta_accuracy_v2": cand["acc_v2"] - base["acc_v2"],
            "delta_cost_per_1k_prompts_usd_v2": cand["cost_per_1k_v2"] - base["cost_per_1k_v2"],
        },
    }
    meta["training"]["default_routing_knobs"]["alpha"] = list(alpha_vec)
    meta["changelog"] = (
        f"{args.dst} = {args.src} with the cluster geometry RE-CLUSTERED on the measured "
        f"corpus (RouterArena {args.split} split, {len(prompts)} labeled prompts; ticket #76). "
        f"Cosine k-means (K=16, k-means++, {args.restarts} restarts, seed {args.seed}) over the "
        f"runtime-contract embeddings; per-cluster labeled-prompt counts {counts} (minimum "
        f"{min(counts)}; the {args.src} geometry held 1-7 prompts in six of sixteen clusters). "
        f"Quality cells re-fitted from the measured labels with the v0.79 builder's exact "
        f"pipeline (per-prompt z-score across the 8 columns, shrinkage_k0={SHRINKAGE_K0:g}, "
        f"min-max rescale into each cluster's {args.src} range). Runtime embedder untouched "
        f"(jina-v2-base-code-int8, dim 768). Routing knobs UNCHANGED from {args.src}: the "
        f"same per-cluster alpha vector applied positionally to the new clusters, so the "
        f"replay isolates the geometry change. Replay on the labeled corpus, both "
        f"aggregations: V1 {cand['acc_v1']:.4f} @ ${cand['cost_per_1k_v1']:.2f}/1k vs "
        f"{base['acc_v1']:.4f} @ ${base['cost_per_1k_v1']:.2f}/1k; V2 {cand['acc_v2']:.4f} "
        f"@ ${cand['cost_per_1k_v2']:.2f}/1k vs {base['acc_v2']:.4f} @ "
        f"${base['cost_per_1k_v2']:.2f}/1k — ship verdict. "
        f"Generated by scripts/build_v081_recluster.py.\n\n"
        + meta.get("changelog", ""))

    DST.mkdir(exist_ok=True)
    write_centroids(DST / "centroids.bin", cents)
    (DST / "quality_means.json").write_text(json.dumps(new_qm, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    (DST / "model_axes.json").write_text(json.dumps(new_axes, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    (DST / "rankings.json").write_text(json.dumps(new_rankings, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    (DST / "model_registry.json").write_text(json.dumps(reg, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    with open(DST / "metadata.yaml", "w", encoding="utf-8", newline="\n") as f:
        yaml.safe_dump(meta, f, sort_keys=False, allow_unicode=True, width=100)

    features = {
        "meta": {
            "comment": f"{args.dst}: corpus-matched re-clustering from measured prompts (build_v081_recluster.py, ticket #76).",
            "k": K,
            "n_models": len(all_models),
            "roster_version": args.dst,
            "source": "quality_means.json + model_axes.json",
        },
        "models": {
            m: {
                "psi_probe": [new_qm["quality_means"][str(k)][m] for k in range(K)],
                "operational": new_axes["axes"][m],
            } for m in all_models
        },
    }
    (DST / "model_features.json").write_text(json.dumps(features, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")

    # centroids header sanity: CRT1, K=16, dim=768, rows L2-normalized
    raw = (DST / "centroids.bin").read_bytes()
    assert raw[:4] == b"CRT1" and int.from_bytes(raw[8:12], "little") == K and int.from_bytes(raw[12:16], "little") == 768
    chk = np.frombuffer(raw[16:], dtype="<f4").reshape(K, 768)
    norms = np.linalg.norm(chk, axis=1)
    assert np.allclose(norms, 1.0, atol=1e-5), norms
    print("wrote", DST)


if __name__ == "__main__":
    main()
