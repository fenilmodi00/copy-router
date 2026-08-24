#!/usr/bin/env python3
"""Probe hard-tier coding quality: glm-5.2 vs kimi-k3 vs kimi-k2.7-code via ai&.

Sends 30 HARD-difficulty coding prompts (agentic / multi-step algorithms /
systems / large reasoning) to 3 models through ai& /v1/chat/completions,
scores pass/fail on a distinctive answer token, computes per-model pass rates,
derives ratio_glm_vs_k3 + go_no_go gate for whether glm-5.2 can honestly win
hard clusters {0,13} from kimi-k3 on cost.

GATE: ratio_glm_vs_k3 = pass_glm / pass_k3 (guard pass_k3==0 -> FAIL).
  ratio >= 0.97 -> glm merely needs to TIE; cost decides -> GLM_HARD_TIER.
  ratio <  0.97 -> glm clearly underperforms -> K3_HARD_TIER (kimi-k3 stays).

Usage: python scripts/probe_hard_tier.py
Reads AIAND_API_KEY + AIAND_API_URL from .env (fallback: .env.local).
Budget: ~$2-4 (30 prompts x 3 models x ~2k tokens; kimi-k3 most expensive).
"""
import json, os, sys, time, urllib.request, urllib.error

# Each probe is a HARD problem with a single deterministic answer. The model
# is asked to end its reply with `@@P<NN>=<answer>` (a code-unfriendly token
# that cannot appear in valid Python), so pass/fail is deterministic, measures
# correctness (not just function-name presence), and the index keeps test
# substrings unique even when two problems share an answer value.
PROBES = [
    {"prompt": "Implement a function that computes how many units of rainwater are trapped, given an elevation map as a list of non-negative integers (classic 'trapping rain water': water above bar i = min(max-left, max-right) - height[i], clamped at 0; sum over all i). Compute the answer for heights = [0,1,0,2,1,0,1,3,2,1,2,1]. Show your code, then on the final line print exactly @@P01=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p01=6"},
    {"prompt": "Implement longest-strictly-increasing-subsequence length (O(n log n) patience or O(n^2) DP). Compute the LIS length for A = [10,9,2,5,3,7,101,18,4,6,12,1,8,11]. Show your code, then on the final line print exactly @@P02=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p02=6"},
    {"prompt": "Implement 0/1 knapsack (max total value, each item used at most once). Items as (weight,value): (2,3),(3,4),(4,5),(5,6); knapsack capacity W = 5. Compute the maximum achievable value. Show your code, then on the final line print exactly @@P03=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p03=6"},
    {"prompt": "Implement the coin-change minimum-coins function (fewest coins summing to amount, unlimited supply of each denomination; -1 if impossible). coins = [1,3,4], amount = 6. Compute the minimum number of coins. Show your code, then on the final line print exactly @@P04=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p04=2"},
    {"prompt": "Implement BFS shortest-path length (number of steps, 4-directional movement, cannot enter walls) on a grid. Grid 5x5, 0=open 1=wall, rows top-to-bottom: row0=[0,0,0,0,0], row1=[1,1,1,1,0], row2=[0,0,0,0,0], row3=[0,1,1,1,1], row4=[0,0,0,0,0]. Start top-left (0,0), end bottom-right (4,4). Compute the shortest path length in steps. Show your code, then on the final line print exactly @@P05=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p05=16"},
    {"prompt": "Implement Dijkstra shortest path on an undirected weighted graph. Nodes 0..5, edges (u,v,weight): (0,1,4),(0,2,1),(2,1,1),(1,3,1),(2,3,5),(3,4,3),(4,5,2),(3,5,8). Compute the shortest distance from node 0 to node 5. Show your code, then on the final line print exactly @@P06=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p06=8"},
    {"prompt": "Implement number-of-islands (4-connected components of '1' in a grid of '1'/'0'). Grid rows: [1,1,0,0,0],[1,1,0,0,0],[0,0,1,0,0],[0,0,0,1,1]. Compute the count of islands. Show your code, then on the final line print exactly @@P07=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p07=3"},
    {"prompt": "Implement a flood-fill / 4-connected component size starting from a given cell. Grid: [1,1,0],[1,0,0],[0,0,1]. Start at (0,0) (value 1). Compute the size (number of connected 1s) of the component containing (0,0). Show your code, then on the final line print exactly @@P08=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p08=3"},
    {"prompt": "Implement union-find and count connected components. Nodes 0..6, edges: (0,1),(1,2),(3,4),(5,6),(4,5). Compute the number of connected components. Show your code, then on the final line print exactly @@P09=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p09=2"},
    {"prompt": "Implement Kruskal's minimum spanning tree and return the total MST weight. Undirected graph, nodes 0..4, edges (u,v,weight): (0,1,4),(0,2,1),(1,2,2),(1,3,5),(2,3,3),(3,4,6). Compute the MST total weight. Show your code, then on the final line print exactly @@P10=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p10=12"},
    {"prompt": "Implement Kadane's maximum-contiguous-subarray sum. A = [-2,1,-3,4,-1,2,1,-5,4]. Compute the maximum subarray sum. Show your code, then on the final line print exactly @@P11=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p11=6"},
    {"prompt": "Implement maximum-product-subarray (contiguous, can be negative). A = [2,3,-2,4,-1]. Compute the maximum product of any contiguous subarray. Show your code, then on the final line print exactly @@P12=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p12=48"},
    {"prompt": "Implement decode-ways (count of decodings of a digit string where '1'->'A'...'26'->'Z'; '0' alone is invalid, only '10'/'20' pair-valid). s = '11106'. Compute the number of ways to decode it. Show your code, then on the final line print exactly @@P13=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p13=2"},
    {"prompt": "Implement maximal-square (largest square of '1's in a binary matrix, return its side length). Matrix rows: [1,0,1,0,0],[1,0,1,1,1],[1,1,1,1,1],[1,0,1,1,1]. Compute the maximum square side length. Show your code, then on the final line print exactly @@P14=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p14=3"},
    {"prompt": "Implement a basic-calculator supporting +, -, parentheses, and integers (with whitespace). Evaluate the expression ' (1+(4+5+2)-3)+(6+8) '. Compute the integer result. Show your code, then on the final line print exactly @@P15=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p15=23"},
    {"prompt": "Implement a small stack-based interpreter with instructions: PUSH n (push integer), POP, DUP (duplicate top), SWAP (swap top two), ADD, SUB, MUL, DIV (integer division), and it halts returning the top of stack. Execute this program and return the final top-of-stack value: PUSH 7, PUSH 3, SUB, DUP, MUL, PUSH 2, MUL, PUSH 1, SUB. Show your code, then on the final line print exactly @@P16=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p16=31"},
    {"prompt": "Implement alien-dictionary order derivation: given a list of words sorted lexicographically in an unknown alien language, find a character order (topological sort of first-differing char pairs). Words = ['wrt','wrf','er','ett','rftt','te']. Derive a valid character order and output it as a single lowercase string. Show your code, then on the final line print exactly @@P17=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p17=wertf"},
    {"prompt": "Implement burst-balloons maximum coins (bursting balloon i with coins nums[left]*nums[i]*nums[right], neighbours become adjacent; pad with virtual 1 at both ends). nums = [2,4,3,5]. Compute the maximum coins obtainable by bursting all balloons. Show your code, then on the final line print exactly @@P18=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p18=115"},
    {"prompt": "Implement matrix-chain multiplication returning the minimum number of scalar multiplications. Dimensions p = [40,20,30,10,30] (4 matrices: 40x20, 20x30, 30x10, 10x30). Compute the minimum scalar multiplications to multiply the whole chain. Show your code, then on the final line print exactly @@P19=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p19=36000"},
    {"prompt": "Implement unique-paths II (count paths from top-left to bottom-right moving only right or down, avoiding obstacle cells marked 1). Grid 5x5 rows: [0,0,0,0,0],[0,1,0,1,0],[0,0,0,0,0],[0,1,0,1,0],[0,0,0,0,0]. Compute the number of distinct paths. Show your code, then on the final line print exactly @@P20=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p20=6"},
    {"prompt": "Implement longest-common-subsequence length. s1 = 'XMJYAUZ', s2 = 'MZJAWU'. Compute the LCS length. Show your code, then on the final line print exactly @@P21=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p21=4"},
    {"prompt": "Implement word-break counting the number of distinct segmentations of s into a sequence of dictionary words (each word usable unlimited times). s = 'aaaa', wordDict = ['a','aa']. Compute the number of segmentations. Show your code, then on the final line print exactly @@P22=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p22=5"},
    {"prompt": "Implement distinct-subsequences count (number of distinct subsequences of string s that equal string t). s = 'babgbag', t = 'bag'. Compute the count. Show your code, then on the final line print exactly @@P23=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p23=5"},
    {"prompt": "Implement median-of-two-sorted-arrays in O(log(min(m,n))). nums1 = [1,3,5], nums2 = [2,4,6]. Compute the median (if total length is even, the average of the two middle elements; print with a decimal point if non-integer). Show your code, then on the final line print exactly @@P24=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p24=3.5"},
    {"prompt": "Implement sliding-window-maximum and return the SUM of the window maxima. nums = [1,3,-1,-3,5,3,6,7], k = 3. Compute the sum of the maximum of each sliding window. Show your code, then on the final line print exactly @@P25=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p25=29"},
    {"prompt": "Implement an LRU cache (capacity 2) supporting PUT(key,value) and GET(key) returning value or -1, updating recency on access. Run these operations in order: PUT 1 10, PUT 2 20, GET 1, PUT 3 30, GET 2, PUT 4 40, GET 1, GET 2, GET 3, GET 4. Compute the SUM of the four GET-1/GET-2/GET-3/GET-4 return values (treat -1 as -1). Show your code, then on the final line print exactly @@P26=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p26=68"},
    {"prompt": "Implement word-search II (count how many given words can be formed by a path of adjacent cells, each cell used at most once per word). Board rows: ['oaan','etae','ihkr','iflv']. Words = ['oath','pea','eat','rain']. Compute how many of the words exist in the board. Show your code, then on the final line print exactly @@P27=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p27=2"},
    {"prompt": "Implement a reverse-polish-notation evaluator (+, -, *, / integer division truncating toward zero). Evaluate the token list ['5','1','2','+','4','*','+','3','-']. Compute the result. Show your code, then on the final line print exactly @@P28=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p28=14"},
    {"prompt": "Implement Tarjan's or Kosaraju's strongly-connected-components algorithm and return the count of SCCs in a directed graph. Edges: 0->1, 1->2, 2->0, 2->3, 3->4, 4->3. Compute the number of SCCs. Show your code, then on the final line print exactly @@P29=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p29=2"},
    {"prompt": "Implement minimum-window-substring (the smallest contiguous substring of s that contains every character of t including multiplicity) and return its LENGTH. s = 'ADOBECODEBANC', t = 'ABC'. Compute the length of the minimum window. Show your code, then on the final line print exactly @@P30=<answer> with no spaces (replace <answer> with your computed value).", "test": "@@p30=4"},
]

MODELS = [
    "zai-org/glm-5.2",
    "moonshotai/kimi-k3",
    "moonshotai/kimi-k2.7-code",
]

GLM = "zai-org/glm-5.2"
KIMI_K3 = "moonshotai/kimi-k3"
RESULTS_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "hard_tier_probe_results.json")


def load_env():
    env = {}
    base = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    candidates = [
        os.path.join(base, ".env"),
        os.path.join(base, ".env.local"),
        os.path.join(os.getcwd(), ".env"),
        os.path.join(os.getcwd(), ".env.local"),
    ]
    for path in candidates:
        if not os.path.exists(path):
            continue
        with open(path) as f:
            for line in f:
                line = line.strip()
                if not line or line.startswith("#"):
                    continue
                if "=" in line:
                    k, v = line.split("=", 1)
                    k, v = k.strip(), v.strip().strip('"').strip("'")
                    if k not in env:
                        env[k] = v
    return env


def call_aiand(env, model, prompt):
    url = env.get("AIAND_API_URL", "https://api.aiand.com/v1").rstrip("/") + "/chat/completions"
    api_key = env.get("AIAND_API_KEY", "")
    body = json.dumps({
        "model": model,
        "messages": [{"role": "user", "content": prompt}],
        "max_tokens": 2000,
        "temperature": 0.0,
    }).encode()
    req = urllib.request.Request(url, data=body, method="POST")
    req.add_header("Content-Type", "application/json")
    req.add_header("Authorization", f"Bearer {api_key}")
    req.add_header("User-Agent", "Mozilla/5.0")
    try:
        with urllib.request.urlopen(req, timeout=120) as resp:
            data = json.loads(resp.read())
            choices = data.get("choices", [])
            if choices:
                return choices[0].get("message", {}).get("content", "") or ""
            return ""
    except urllib.error.HTTPError as e:
        err_body = ""
        try:
            err_body = e.read().decode()[:200]
        except Exception:
            pass
        print(f"  [ERROR] {model}: HTTP {e.code} {err_body}", file=sys.stderr)
        return ""
    except Exception as e:
        print(f"  [ERROR] {model}: {e}", file=sys.stderr)
        return ""


def save_partial(results):
    with open(RESULTS_FILE, "w") as f:
        json.dump({"results": results}, f, indent=2)


def main():
    # Adversarial: validate test substrings are non-empty and unique.
    tests = [p["test"] for p in PROBES]
    if not all(tests):
        print("FATAL: one or more probes have an empty test substring", file=sys.stderr)
        sys.exit(1)
    if len(set(tests)) != len(tests):
        print("FATAL: duplicate test substrings detected", file=sys.stderr)
        sys.exit(1)
    assert len(PROBES) == 30, f"expected 30 probes, got {len(PROBES)}"

    env = load_env()
    if not env.get("AIAND_API_KEY"):
        print("FATAL: AIAND_API_KEY not found in .env or .env.local", file=sys.stderr)
        sys.exit(1)

    total_calls = len(PROBES) * len(MODELS)
    print(f"Probe: {len(PROBES)} prompts x {len(MODELS)} models = {total_calls} calls")
    print(f"API URL: {env.get('AIAND_API_URL', 'https://api.aiand.com/v1')}")
    print()

    # Verify model IDs with a trivial call.
    print("Verifying model IDs...")
    verified = []
    for model in MODELS:
        response = call_aiand(env, model, "Say hello in one word.")
        if response:
            print(f"  {model}: OK ({len(response)} chars)")
            verified.append(model)
        else:
            print(f"  {model}: FAILED - check model ID", file=sys.stderr)
        time.sleep(0.5)
    if len(verified) < len(MODELS):
        print(f"\nWARNING: {len(MODELS) - len(verified)} model(s) failed verification.", file=sys.stderr)
    print()

    # Load partial results for resume.
    results = {}
    if os.path.exists(RESULTS_FILE):
        try:
            with open(RESULTS_FILE) as f:
                partial = json.load(f)
                results = partial.get("results", {})
        except Exception:
            results = {}

    completed = sum(1 for p in results.values() for v in p.values() if v is not None)
    if completed > 0:
        print(f"Resuming: {completed}/{total_calls} already done")
    print()

    # Run probe.
    errors = 0
    for i, probe in enumerate(PROBES):
        probe_key = f"probe_{i:03d}"
        if probe_key not in results:
            results[probe_key] = {}

        for model in MODELS:
            if model in results[probe_key] and results[probe_key][model] is not None:
                continue

            response = call_aiand(env, model, probe["prompt"])
            passed = probe["test"].lower() in response.lower() if response else False

            if not response:
                errors += 1

            results[probe_key][model] = {
                "passed": passed,
                "response_length": len(response),
                "error": not bool(response),
            }
            time.sleep(0.5)

        save_partial(results)
        done = sum(1 for p in results.values() for v in p.values() if v is not None)
        print(f"  [{i+1:3d}/{len(PROBES)}] {probe['prompt'][:60]}... ({done}/{total_calls})")

    # Compute pass rates from actual results.
    pass_rates = {}
    for model in MODELS:
        passed = 0
        total = 0
        for p in results.values():
            if model in p and p[model] is not None:
                total += 1
                if p[model].get("passed"):
                    passed += 1
        pass_rates[model] = {
            "passed": passed,
            "total": total,
            "rate": passed / total if total > 0 else 0.0,
        }

    pass_glm = pass_rates[GLM]["passed"]
    pass_k3 = pass_rates[KIMI_K3]["passed"]

    # Ratio: glm pass count / kimi-k3 pass count (guard division by zero -> FAIL).
    if pass_k3 == 0:
        ratio_glm_vs_k3 = 0.0
        recommended_glm_win = None
        go_no_go = "K3_HARD_TIER"
    else:
        ratio_glm_vs_k3 = pass_glm / pass_k3
        recommended_glm_win = 1.0 if ratio_glm_vs_k3 >= 0.97 else None
        go_no_go = "GLM_HARD_TIER" if ratio_glm_vs_k3 >= 0.97 else "K3_HARD_TIER"

    total_errors = sum(
        1 for p in results.values()
        for m, v in p.items()
        if v is not None and v.get("error")
    )
    error_rate = total_errors / total_calls if total_calls > 0 else 0.0

    # Print results.
    print()
    print("=" * 70)
    print("HARD-TIER PROBE RESULTS")
    print("=" * 70)
    for model, stats in sorted(pass_rates.items(), key=lambda x: -x[1]["rate"]):
        print(f"  {model:45s} {stats['passed']:3d}/{stats['total']:3d} = {stats['rate']:.1%}")
    print()
    print(f"  pass_glm:     {pass_glm}")
    print(f"  pass_k3:      {pass_k3}")
    print(f"  ratio_glm_vs_k3: {ratio_glm_vs_k3:.4f}")
    print(f"  recommended_glm_win: {recommended_glm_win}")
    print(f"  go_no_go: {go_no_go}")
    print(f"  Error rate: {total_errors}/{total_calls} = {error_rate:.1%}")
    print()

    report = {
        "pass_rates": {
            m: {"passed": s["passed"], "total": s["total"], "rate": s["rate"]}
            for m, s in pass_rates.items()
        },
        "ratio_glm_vs_k3": ratio_glm_vs_k3,
        "recommended_glm_win": recommended_glm_win,
        "go_no_go": go_no_go,
        "results": results,
        "pass_glm": pass_glm,
        "pass_k3": pass_k3,
        "error_rate": error_rate,
        "total_errors": total_errors,
        "total_calls": total_calls,
        "models_verified": verified,
        "probe_count": len(PROBES),
    }
    with open(RESULTS_FILE, "w") as f:
        json.dump(report, f, indent=2)

    print(f"Results saved to {RESULTS_FILE}")


if __name__ == "__main__":
    main()
