#!/usr/bin/env python3
"""Probe glm-5.3 quality vs the v0.76 tier leaders through ai&.

Sends the same 30 medium-difficulty coding prompts used by probe_motif3_medium.py
to 5 models via ai& /v1/chat/completions, scores pass/fail, computes per-model
pass rates, and derives recommended_q_glm53_on / recommended_q_glm53_off /
recommended_q_qwen38_off for the v0.77 maxpool quality factors.

Models:
  - zai-org/glm-5.3            (candidate, HARD_K3 challenger)
  - moonshotai/kimi-k3          (HARD_K3 leader, q {0,13})
  - motif-technologies/motif-3  (MID_M3 leader)
  - deepseek-ai/deepseek-v4-flash (FLASH_5 leader)
  - qwen/qwen3.8-27b            (TierLow pool model, wins no clusters)

Usage: python3 scripts/probe_glm53_medium.py [--resume]
Reads DEV_AIAND_API (bearer key) + AIAND_API_URL from .env.local (fallback: .env).
Budget: ~$3-6 (30 prompts x 5 models x ~2k tokens).
"""
import argparse
import datetime
import json, os, sys, time, urllib.request, urllib.error

PROBES = [
    {"prompt": "Write a Python function called `slugify` that takes a string, converts it to lowercase, replaces spaces and special characters with hyphens, removes consecutive hyphens, and strips leading/trailing hyphens. Reply with only the code.", "test": "slugify"},
    {"prompt": "Write a Python function called `compress_string` that takes a string and returns a compressed version where consecutive repeated characters are replaced by the character followed by the count. If the compressed version is not shorter than the original, return the original. Reply with only the code.", "test": "compress_string"},
    {"prompt": "Write a Python function called `roman_to_int` that takes a Roman numeral string and returns its integer value. Reply with only the code.", "test": "roman_to_int"},
    {"prompt": "Write a Python function called `int_to_roman` that takes an integer and returns its Roman numeral string representation. Reply with only the code.", "test": "int_to_roman"},
    {"prompt": "Write a Python function called `longest_unique_substr` that takes a string and returns the length of the longest substring without repeating characters. Reply with only the code.", "test": "longest_unique_substr"},
    {"prompt": "Write a Python function called `group_anagrams` that takes a list of strings and returns a list of lists, where each inner list contains strings that are anagrams of each other. Reply with only the code.", "test": "group_anagrams"},
    {"prompt": "Write a Python function called `merge_intervals` that takes a list of intervals (each a list of two integers [start, end]) and returns a new list of merged non-overlapping intervals sorted by start. Reply with only the code.", "test": "merge_intervals"},
    {"prompt": "Write a Python function called `spiral_order` that takes an m by n matrix (list of lists) and returns all elements in spiral order (clockwise from top-left). Reply with only the code.", "test": "spiral_order"},
    {"prompt": "Write a Python function called `rotate_matrix` that takes an n by n matrix (list of lists) and rotates it 90 degrees clockwise in place. Reply with only the code.", "test": "rotate_matrix"},
    {"prompt": "Write a Python function called `is_valid_parens` that takes a string containing parentheses, brackets, and braces and returns True if they are balanced, False otherwise. Reply with only the code.", "test": "is_valid_parens"},
    {"prompt": "Write a Python function called `eval_rpn` that takes a list of strings representing a reverse Polish notation expression (with +, -, *, /) and returns the result. Reply with only the code.", "test": "eval_rpn"},
    {"prompt": "Write a Python function called `word_break` that takes a string and a list of dictionary words, and returns True if the string can be segmented into a space-separated sequence of dictionary words. Reply with only the code.", "test": "word_break"},
    {"prompt": "Write a Python function called `min_window_substring` that takes two strings s and t, and returns the minimum-length substring of s that contains all characters of t. If no such substring exists, return an empty string. Reply with only the code.", "test": "min_window_substring"},
    {"prompt": "Write a Python function called `decode_string` that takes an encoded string like '3[a2[c]]' and returns the decoded string 'accaccacc'. Reply with only the code.", "test": "decode_string"},
    {"prompt": "Write a Python function called `top_k_frequent` that takes a list of integers and an integer k, and returns the k most frequent elements. Reply with only the code.", "test": "top_k_frequent"},
    {"prompt": "Write a Python function called `product_except_self` that takes a list of integers and returns a new list where each element at index i is the product of all elements except the one at i. Do not use division. Reply with only the code.", "test": "product_except_self"},
    {"prompt": "Write a Python function called `max_subarray_sum` that takes a list of integers and returns the maximum sum of a contiguous subarray. Reply with only the code.", "test": "max_subarray_sum"},
    {"prompt": "Write a Python function called `trap_water` that takes a list of non-negative integers representing an elevation map and returns the total amount of rainwater that can be trapped. Reply with only the code.", "test": "trap_water"},
    {"prompt": "Write a Python function called `first_unique_char` that takes a string and returns the index of the first non-repeating character, or -1 if none exists. Reply with only the code.", "test": "first_unique_char"},
    {"prompt": "Write a Python function called `count_palindromes` that takes a string and returns the total number of palindromic substrings. Reply with only the code.", "test": "count_palindromes"},
    {"prompt": "Write a Python function called `reorganize_string` that takes a string and returns a rearranged string where no two adjacent characters are the same. If impossible, return an empty string. Reply with only the code.", "test": "reorganize_string"},
    {"prompt": "Write a Python function called `simplify_path` that takes an absolute Unix path string and returns its simplified canonical form (resolving . and .., removing redundant slashes). Reply with only the code.", "test": "simplify_path"},
    {"prompt": "Write a Python function called `fraction_to_decimal` that takes a numerator and denominator (both integers) and returns the decimal string representation, with repeating decimals enclosed in parentheses. Reply with only the code.", "test": "fraction_to_decimal"},
    {"prompt": "Write a Python function called `eval_basic_expr` that takes a string containing a basic arithmetic expression with +, -, *, / and parentheses, and returns the integer result (truncating toward zero). Reply with only the code.", "test": "eval_basic_expr"},
    {"prompt": "Write a Python function called `zigzag_convert` that takes a string and a number of rows, and returns the string read row-by-row after writing the original in a zigzag pattern across those rows. Reply with only the code.", "test": "zigzag_convert"},
    {"prompt": "Write a Python function called `length_of_lis` that takes a list of integers and returns the length of the longest strictly increasing subsequence. Reply with only the code.", "test": "length_of_lis"},
    {"prompt": "Write a Python function called `count_islands` that takes a 2D grid (list of lists) of '1's (land) and '0's (water) and returns the number of islands. Reply with only the code.", "test": "count_islands"},
    {"prompt": "Write a Python function called `can_partition` that takes a list of positive integers and returns True if it can be partitioned into two subsets with equal sums. Reply with only the code.", "test": "can_partition"},
    {"prompt": "Write a Python function called `find_course_order` that takes a number of courses and a list of prerequisite pairs [a, b] (meaning b must be taken before a), and returns a valid course ordering, or an empty list if impossible. Reply with only the code.", "test": "find_course_order"},
    {"prompt": "Write a Python function called `min_path_sum` that takes a grid (list of lists of non-negative integers) and returns the minimum path sum from the top-left to the bottom-right, moving only right or down. Reply with only the code.", "test": "min_path_sum"},
]

MODELS = [
    "zai-org/glm-5.3",
    "moonshotai/kimi-k3",
    "motif-technologies/motif-3",
    "deepseek-ai/deepseek-v4-flash",
    "qwen/qwen3.8-27b",
]

GLM53 = "zai-org/glm-5.3"
KIMI_K3 = "moonshotai/kimi-k3"
MOTIF_3 = "motif-technologies/motif-3"
DS_FLASH = "deepseek-ai/deepseek-v4-flash"
QWEN38 = "qwen/qwen3.8-27b"
# Models whose reasoning effort we pin explicitly. glm-5.3's live menu is
# none/low/xhigh/max with default max; qwen3.8-27b's menu is
# none/low/medium/xhigh with default medium. Every other model runs its own
# default (parameter omitted), mirroring probe_motif3_medium.py's call body.
REASONING_EFFORT = {GLM53: "max", QWEN38: "medium"}
RESULTS_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "glm53_probe_results.json")

# Default quality factor for clusters where glm-5.3 should NOT win.
RECOMMENDED_Q_GLM53_OFF = 0.97
# Default quality factor for qwen3.8-27b (pool model, wins no clusters).
RECOMMENDED_Q_QWEN38_OFF = 0.55


def load_env():
    env = {}
    base = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    candidates = [
        os.path.join(base, ".env.local"),
        os.path.join(base, ".env"),
        os.path.join(os.getcwd(), ".env.local"),
        os.path.join(os.getcwd(), ".env"),
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


def call_aiand(env, model, prompt, max_retries=3):
    url = env.get("AIAND_API_URL", "https://api.aiand.com/v1").rstrip("/") + "/chat/completions"
    api_key = env.get("DEV_AIAND_API", "")
    payload = {
        "model": model,
        "messages": [{"role": "user", "content": prompt}],
        # glm-5.3 at max effort can spend >2000 tokens on reasoning_content
        # alone and return content=null with finish_reason=length; 16k leaves
        # room for the answer after reasoning.
        "max_tokens": 16000,
        "temperature": 0.0,
    }
    if model in REASONING_EFFORT:
        payload["reasoning_effort"] = REASONING_EFFORT[model]
    body = json.dumps(payload).encode()
    last_err = None
    for attempt in range(max_retries):
        req = urllib.request.Request(url, data=body, method="POST")
        req.add_header("Content-Type", "application/json")
        req.add_header("Authorization", f"Bearer {api_key}")
        # Browser UA required: plain curl/python UA gets Cloudflare 403 error 1010.
        req.add_header("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
        try:
            with urllib.request.urlopen(req, timeout=180) as resp:
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
            last_err = f"HTTP {e.code} {err_body}"
            if e.code in (429, 500, 502, 503, 504):
                time.sleep(5 * (attempt + 1))
                continue
            break
        except Exception as e:
            last_err = str(e)
            time.sleep(3 * (attempt + 1))
            continue
    print(f"  [ERROR] {model}: {last_err} (after {max_retries} attempts)", file=sys.stderr)
    return ""


def save_partial(results):
    with open(RESULTS_FILE, "w") as f:
        json.dump({"results": results}, f, indent=2)


def main():
    parser = argparse.ArgumentParser(
        description="Probe glm-5.3 pass-rate vs v0.76 tier leaders on the 30 medium coding prompts."
    )
    parser.add_argument(
        "--resume", action="store_true",
        help="Resume from partial results in the results file instead of starting fresh.",
    )
    args = parser.parse_args()

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
    if not env.get("DEV_AIAND_API"):
        print("FATAL: DEV_AIAND_API not found in .env.local or .env", file=sys.stderr)
        sys.exit(1)

    total_calls = len(PROBES) * len(MODELS)
    print(f"Probe: {len(PROBES)} prompts x {len(MODELS)} models = {total_calls} calls")
    print(f"API URL: {env.get('AIAND_API_URL', 'https://api.aiand.com/v1')}")
    print("API key: loaded (DEV_AIAND_API)")
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

    # Load partial results for resume (default: fresh run overwrites).
    results = {}
    if args.resume and os.path.exists(RESULTS_FILE):
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
    for i, probe in enumerate(PROBES):
        probe_key = f"probe_{i:03d}"
        if probe_key not in results:
            results[probe_key] = {}

        for model in MODELS:
            if model in results[probe_key] and results[probe_key][model] is not None:
                continue

            response = call_aiand(env, model, probe["prompt"])
            passed = probe["test"].lower() in response.lower() if response else False

            results[probe_key][model] = {
                "passed": passed,
                "response_length": len(response),
                "error": not bool(response),
            }
            time.sleep(0.5)

        save_partial(results)
        done = sum(1 for p in results.values() for v in p.values() if v is not None)
        print(f"  [{i+1:3d}/{len(PROBES)}] {probe['prompt'][:60]}... ({done}/{total_calls})")

    # Compute pass rates from actual results. Errored calls (no response after
    # retries) are excluded from the denominator so a transient network failure
    # doesn't count as a model failure.
    pass_rates = {}
    for model in MODELS:
        passed = 0
        total = 0
        errors = 0
        for p in results.values():
            if model in p and p[model] is not None:
                if p[model].get("error"):
                    errors += 1
                    continue
                total += 1
                if p[model].get("passed"):
                    passed += 1
        pass_rates[model] = {
            "passed": passed,
            "total": total,
            "errors": errors,
            "rate": passed / total if total > 0 else 0.0,
        }

    def rate_of(model):
        return pass_rates[model]["rate"]

    def ratio(numer, denom):
        # Guard division by zero: a zero denominator means the leader scored 0.
        if rate_of(denom) == 0:
            return 1.12 if rate_of(numer) > 0 else 0.0
        return rate_of(numer) / rate_of(denom)

    ratio_vs_k3 = ratio(GLM53, KIMI_K3)
    ratio_vs_m3 = ratio(GLM53, MOTIF_3)
    ratio_vs_flash = ratio(GLM53, DS_FLASH)

    # go/no-go: glm-5.3 must match or beat the HARD_K3 leader's pass rate.
    go_no_go = "PASS" if rate_of(GLM53) >= rate_of(KIMI_K3) else "FAIL"

    # Recommended "on" quality factor: glm-5.3 vs the best other model,
    # floored at 1.02 and capped at 1.12 (mirrors motif's M3_WIN derivation).
    best_other = max(rate_of(KIMI_K3), rate_of(MOTIF_3), rate_of(DS_FLASH))
    if best_other == 0:
        raw_ratio = 1.12 if rate_of(GLM53) > 0 else 1.02
    else:
        raw_ratio = rate_of(GLM53) / best_other
    recommended_q_on = max(1.02, min(raw_ratio, 1.12))

    # qwen3.8-27b is a TierLow pool model that must never win a cluster. Derive
    # its off-factor from its pass rate vs the TierLow leader (flash), clamped
    # to [0.40, 0.92] — 0.92 mirrors Q_M3_OFF, safely below every tier leader.
    raw_qwen = ratio(QWEN38, DS_FLASH)
    recommended_q_qwen38_off = max(0.40, min(raw_qwen, 0.92))

    total_errors = sum(
        1 for p in results.values()
        for m, v in p.items()
        if v is not None and v.get("error")
    )
    error_rate = total_errors / total_calls if total_calls > 0 else 0.0
    print(f"  recommended_q_glm53_on:  {recommended_q_on:.4f}")
    print(f"  recommended_q_glm53_off: {RECOMMENDED_Q_GLM53_OFF}")
    print(f"  recommended_q_qwen38_off: {recommended_q_qwen38_off:.4f}")
    print(f"  go_no_go: {go_no_go}")
    print(f"  Error rate: {total_errors}/{total_calls} = {error_rate:.1%}")
    print()

    report = {
        "pass_rates": {
            m: {"passed": s["passed"], "total": s["total"], "errors": s["errors"], "rate": s["rate"]}
            for m, s in pass_rates.items()
        },
        "ratio_glm53_vs_k3": ratio_vs_k3,
        "ratio_glm53_vs_m3": ratio_vs_m3,
        "ratio_glm53_vs_flash": ratio_vs_flash,
        "recommended_q_glm53_on": recommended_q_on,
        "recommended_q_glm53_off": RECOMMENDED_Q_GLM53_OFF,
        "recommended_q_qwen38_off": recommended_q_qwen38_off,
        "go_no_go": go_no_go,
        "results": results,
        "n_prompts": len(PROBES),
        "test_date": datetime.date.today().isoformat(),
        "price_at_test_time": {
            "zai-org/glm-5.3": {"input_per_1m": 1.000, "output_per_1m": 4.000, "cached_input_per_1m": 0.30},
            "qwen/qwen3.8-27b": {"input_per_1m": 0.40, "output_per_1m": 3.000, "cached_input_per_1m": 0.20},
        },
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
