#!/usr/bin/env python3
"""Offline v0.78 -> v0.79 comparison report (pure stdlib).

Reads the two artifact bundles and prints:

1. Per-cluster within-AIand winner changes (quality_means argmax
   over the 8-model roster), plus the blend (rankings) winner
   that actually routes.
2. Per-model average-rank deltas across the 16 clusters.
3. Clusters where the measured bundle flipped a researched anchor:
   v0.78's researched winner (its cells ARE the researched
   anchors) vs v0.79's measured winner, annotated with the AA
   Intelligence Index v4.3.2 score of each side and whether the
   flip goes against (measured winner lower-researched) or with
   the researched capability signal.

Also verifies the frozen-geometry invariant (centroids.bin
byte-identical). Not a gate — exit 0 unless a bundle is
missing or malformed.

Usage:
  python scripts/v079_comparison.py
  python scripts/v079_comparison.py --v078 DIR --v079 DIR
"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_V078 = REPO_ROOT / "internal" / "router" / "cluster" / "artifacts" / "v0.78"
DEFAULT_V079 = REPO_ROOT / "internal" / "router" / "cluster" / "artifacts" / "v0.79"

# The 8-model AIand roster (parent issue #64).
ROSTER = [
    "deepseek-ai/deepseek-v4-pro",
    "deepseek-ai/deepseek-v4-flash",
    "deepseek-ai/deepseek-v4.1-flash",
    "zai-org/glm-5.3",
    "zai-org/glm-5.3-flash",
    "qwen/qwen3.8-27b",
    "moonshotai/kimi-k3",
    "motif-technologies/motif-3",
]

# AA Intelligence Index v4.3.2 per AIand model (the researched
# capability signal; artifacts_v077_test.go's v077ResearchedScore,
# from .context/bench-scores.md, retrieved 2026-10-04).
RESEARCHED_AA = {
    "zai-org/glm-5.3": 45,
    "moonshotai/kimi-k3": 44,
    "zai-org/glm-5.3-flash": 42,
    "deepseek-ai/deepseek-v4.1-flash": 39,
    "deepseek-ai/deepseek-v4-pro": 36,
    "deepseek-ai/deepseek-v4-flash": 34,
    "qwen/qwen3.8-27b": 34,
    "motif-technologies/motif-3": 34,
}


def load_json(path: Path):
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError:
        sys.exit(f"error: missing file {path}")
    except json.JSONDecodeError as e:
        sys.exit(f"error: malformed JSON in {path}: {e}")


def bundle_tables(bundle_dir: Path):
    """Return (quality_means, rankings) keyed by int cluster."""
    qm = load_json(bundle_dir / "quality_means.json")["quality_means"]
    rk = load_json(bundle_dir / "rankings.json")["rankings"]
    return ({int(k): row for k, row in qm.items()},
            {int(k): row for k, row in rk.items()})


def winner(row):
    """Deterministic argmax over the roster (ties -> first sorted)."""
    best, best_v = None, None
    for m in sorted(ROSTER):
        v = row.get(m)
        if v is None:
            sys.exit(f"error: roster model {m} missing from a cluster row")
        if best_v is None or v > best_v:
            best, best_v = m, v
    return best, best_v


def avg_ranks(qm):
    """Per-model average competition rank across clusters (1 = best)."""
    ranks = {m: [] for m in ROSTER}
    for _k, row in qm.items():
        vals = {m: row[m] for m in ROSTER}
        for m in ROSTER:
            ranks[m].append(1 + sum(1 for o in ROSTER if vals[o] > vals[m]))
    return {m: sum(v) / len(v) for m, v in ranks.items()}


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--v078", type=Path, default=DEFAULT_V078)
    ap.add_argument("--v079", type=Path, default=DEFAULT_V079)
    args = ap.parse_args()

    qm78, rk78 = bundle_tables(args.v078)
    qm79, rk79 = bundle_tables(args.v079)
    clusters = sorted(set(qm78) | set(qm79))

    print(f"=== v0.78 -> v0.79 comparison (AIand roster, {len(clusters)} clusters) ===")
    print(f"v0.78: {args.v078}")
    print(f"v0.79: {args.v079}")
    same = sha256(args.v078 / "centroids.bin") == sha256(args.v079 / "centroids.bin")
    print(f"centroids.bin: {'BYTE-IDENTICAL' if same else 'DRIFTED (geometry not frozen!)'}")
    print()

    # --- 1. per-cluster within-AIand winners -------------------------
    q_win78 = {k: winner(qm78[k]) for k in clusters}
    q_win79 = {k: winner(qm79[k]) for k in clusters}
    r_win78 = {k: winner(rk78[k]) for k in clusters}
    r_win79 = {k: winner(rk79[k]) for k in clusters}

    print("-- per-cluster within-AIand QUALITY winner (quality_means argmax) --")
    n_q_changed = 0
    for k in clusters:
        m78, _ = q_win78[k]
        m79, _ = q_win79[k]
        changed = m78 != m79
        n_q_changed += changed
        print(f"c{k:02d}  {m78} -> {m79}" + ("   CHANGED" if changed else ""))
    print(f"{n_q_changed}/{len(clusters)} clusters changed quality winner")
    print()

    print("-- per-cluster BLEND winner (rankings argmax = the routing decision) --")
    n_r_changed = 0
    for k in clusters:
        m78, _ = r_win78[k]
        m79, _ = r_win79[k]
        changed = m78 != m79
        n_r_changed += changed
        print(f"c{k:02d}  {m78} -> {m79}" + ("   CHANGED" if changed else ""))
    print(f"{n_r_changed}/{len(clusters)} clusters changed blend winner")
    print()

    # --- 2. per-model average-rank deltas ----------------------------
    print("-- per-model average rank across clusters (1 = best; delta = v0.79 - v0.78) --")
    ar78, ar79 = avg_ranks(qm78), avg_ranks(qm79)
    print(f"{'model':<38} {'v0.78':>6} {'v0.79':>6} {'delta':>7}")
    for m in sorted(ROSTER, key=lambda m: ar79[m] - ar78[m]):
        d = ar79[m] - ar78[m]
        print(f"{m:<38} {ar78[m]:>6.2f} {ar79[m]:>6.2f} {d:>+7.2f}")
    print("(negative delta = measured campaign improved the model's average rank)")
    print()

    # --- 3. researched-anchor flips ----------------------------------
    print("-- researched-anchor flips (measured winner != v0.78 researched winner) --")
    n_flips = 0
    for k in clusters:
        m78, _ = q_win78[k]
        m79, _ = q_win79[k]
        if m78 == m79:
            continue
        n_flips += 1
        aa78, aa79 = RESEARCHED_AA[m78], RESEARCHED_AA[m79]
        if aa79 < aa78:
            flag = f"AGAINST RESEARCH ({aa79 - aa78:+d} AA)"
        elif aa79 > aa78:
            flag = f"WITH RESEARCH ({aa79 - aa78:+d} AA)"
        else:
            flag = "NEUTRAL (equal AA)"
        print(f"c{k:02d}  {m78} (AA {aa78}) -> {m79} (AA {aa79})  {flag}")
    if n_flips == 0:
        print("none — every cluster's researched anchor survived measurement")
    print(f"{n_flips}/{len(clusters)} clusters flipped a researched anchor")


if __name__ == "__main__":
    main()
