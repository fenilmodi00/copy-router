#!/usr/bin/env python3
"""Probe motif-3 quality vs glm-5.2 / kimi-k2.7-code through ai&.

Sends 30 medium-difficulty coding prompts to 3 models via ai& /v1/chat/completions,
scores pass/fail, computes per-model pass rates, derives Q_M3_WIN.

Usage: python scripts/probe_motif3_medium.py
Reads AIAND_API_KEY + AIAND_API_URL from .env (fallback: .env.local).
Budget: ~$1-3 (30 prompts x 3 models x ~2k tokens).
"""
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
    "motif-technologies/motif-3",
    "zai-org/glm-5.2",
    "moonshotai/kimi-k2.7-code",
]

MOTIF_3 = "motif-technologies/motif-3"
GLM = "zai-org/glm-5.2"
RESULTS_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "motif_probe_results.json")


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
    print(f"API key: {env['AIAND_API_KEY'][:8]}...{env['AIAND_API_KEY'][-4:]}")
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

    # Ratio: motif-3 pass count / glm pass count (guard division by zero).
    pass_motif3 = pass_rates[MOTIF_3]["passed"]
    pass_glm = pass_rates[GLM]["passed"]

    if pass_glm == 0:
        ratio = 0.0
        go_no_go = "FLAG"
    else:
        ratio = pass_motif3 / pass_glm
        go_no_go = "PASS" if ratio >= 0.95 else "FLAG"

    recommended = max(1.02, min(ratio, 1.10))

    total_errors = sum(
        1 for p in results.values()
        for m, v in p.items()
        if v is not None and v.get("error")
    )
    error_rate = total_errors / total_calls if total_calls > 0 else 0.0

    # Print results.
    print()
    print("=" * 70)
    print("PROBE RESULTS")
    print("=" * 70)
    for model, stats in sorted(pass_rates.items(), key=lambda x: -x[1]["rate"]):
        print(f"  {model:45s} {stats['passed']:3d}/{stats['total']:3d} = {stats['rate']:.1%}")
    print()
    print(f"  pass_motif3:  {pass_motif3}")
    print(f"  pass_glm:     {pass_glm}")
    print(f"  ratio_m3_vs_glm: {ratio:.4f}")
    print(f"  recommended_m3_win: {recommended:.4f}")
    print(f"  go_no_go: {go_no_go}")
    print(f"  Error rate: {total_errors}/{total_calls} = {error_rate:.1%}")
    print()

    report = {
        "pass_rates": {
            m: {"passed": s["passed"], "total": s["total"], "rate": s["rate"]}
            for m, s in pass_rates.items()
        },
        "ratio_m3_vs_glm": ratio,
        "recommended_m3_win": recommended,
        "go_no_go": go_no_go,
        "results": results,
        "pass_motif3": pass_motif3,
        "pass_glm": pass_glm,
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
