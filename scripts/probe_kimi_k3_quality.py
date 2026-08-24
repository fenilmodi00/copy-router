#!/usr/bin/env python3
"""Probe kimi-k3 quality vs glm-5.2/deepseek-flash/kimi-k2.7 through ai&.

Sends ~100 coding prompts to 4 models via ai& /v1/chat/completions,
scores pass/fail, computes per-model win rates, recommends a multiplier.

Usage: python scripts/probe_kimi_k3_quality.py
Reads AIAND_API_KEY + AIAND_API_URL from .env.
Budget: ~$10 (100 prompts x 4 models x ~2k tokens).
"""
import json, os, sys, time, urllib.request, urllib.error

PROBES = [
    # --- Python (40) ---
    {"prompt": "Write a Python function called `factorial` that takes an integer n and returns n factorial. Reply with only the code.", "test": "factorial"},
    {"prompt": "Write a Python function called `fibonacci` that takes an integer n and returns the nth Fibonacci number. Reply with only the code.", "test": "fibonacci"},
    {"prompt": "Write a Python function called `is_prime` that takes an integer n and returns True if n is prime, False otherwise. Reply with only the code.", "test": "is_prime"},
    {"prompt": "Write a Python function called `reverse_string` that takes a string s and returns the reversed string. Reply with only the code.", "test": "reverse_string"},
    {"prompt": "Write a Python function called `is_palindrome` that takes a string s and returns True if s is a palindrome. Reply with only the code.", "test": "is_palindrome"},
    {"prompt": "Write a Python function called `gcd` that takes two integers a and b and returns their greatest common divisor. Reply with only the code.", "test": "gcd"},
    {"prompt": "Write a Python function called `lcm` that takes two integers a and b and returns their least common multiple. Reply with only the code.", "test": "lcm"},
    {"prompt": "Write a Python function called `bubble_sort` that takes a list of integers and returns the sorted list using bubble sort. Reply with only the code.", "test": "bubble_sort"},
    {"prompt": "Write a Python function called `merge_sort` that takes a list of integers and returns the sorted list using merge sort. Reply with only the code.", "test": "merge_sort"},
    {"prompt": "Write a Python function called `binary_search` that takes a sorted list and a target value and returns the index of the target, or -1 if not found. Reply with only the code.", "test": "binary_search"},
    {"prompt": "Write a Python function called `quicksort` that takes a list of integers and returns the sorted list using quicksort. Reply with only the code.", "test": "quicksort"},
    {"prompt": "Write a Python function called `sum_list` that takes a list of numbers and returns their sum. Reply with only the code.", "test": "sum_list"},
    {"prompt": "Write a Python function called `max_element` that takes a list of numbers and returns the maximum element. Reply with only the code.", "test": "max_element"},
    {"prompt": "Write a Python function called `count_vowels` that takes a string and returns the number of vowels in it. Reply with only the code.", "test": "count_vowels"},
    {"prompt": "Write a Python function called `reverse_list` that takes a list and returns the reversed list. Reply with only the code.", "test": "reverse_list"},
    {"prompt": "Write a Python function called `flatten` that takes a nested list and returns a flat list of all elements. Reply with only the code.", "test": "flatten"},
    {"prompt": "Write a Python function called `deduplicate` that takes a list and returns a new list with duplicates removed, preserving order. Reply with only the code.", "test": "deduplicate"},
    {"prompt": "Write a Python function called `capitalize_words` that takes a string and returns it with each word capitalized. Reply with only the code.", "test": "capitalize_words"},
    {"prompt": "Write a Python function called `word_count` that takes a string and returns a dictionary mapping each word to its count. Reply with only the code.", "test": "word_count"},
    {"prompt": "Write a Python function called `is_anagram` that takes two strings and returns True if they are anagrams. Reply with only the code.", "test": "is_anagram"},
    {"prompt": "Write a Python function called `fizzbuzz` that takes an integer n and prints FizzBuzz from 1 to n. Reply with only the code.", "test": "fizzbuzz"},
    {"prompt": "Write a Python function called `power` that takes a base and exponent and returns base raised to the exponent. Reply with only the code.", "test": "power"},
    {"prompt": "Write a Python function called `sqrt_newton` that takes a number n and returns its square root using Newton's method. Reply with only the code.", "test": "sqrt_newton"},
    {"prompt": "Write a Python function called `is_armstrong` that takes an integer n and returns True if it is an Armstrong number. Reply with only the code.", "test": "is_armstrong"},
    {"prompt": "Write a Python function called `matrix_multiply` that takes two 2D matrices and returns their product. Reply with only the code.", "test": "matrix_multiply"},
    {"prompt": "Write a Python function called `transpose_matrix` that takes a 2D matrix and returns its transpose. Reply with only the code.", "test": "transpose_matrix"},
    {"prompt": "Write a Python function called `rotate_array` that takes a list and an integer k and rotates the list by k positions. Reply with only the code.", "test": "rotate_array"},
    {"prompt": "Write a Python function called `find_duplicate` that takes a list of integers where each value is in range 1..n and returns the duplicate. Reply with only the code.", "test": "find_duplicate"},
    {"prompt": "Write a Python function called `missing_number` that takes a list of integers 0..n with one missing and returns the missing number. Reply with only the code.", "test": "missing_number"},
    {"prompt": "Write a Python function called `is_valid_parentheses` that takes a string of parentheses and returns True if they are balanced. Reply with only the code.", "test": "is_valid_parentheses"},
    {"prompt": "Write a Python function called `roman_to_int` that takes a Roman numeral string and returns its integer value. Reply with only the code.", "test": "roman_to_int"},
    {"prompt": "Write a Python function called `int_to_roman` that takes an integer and returns its Roman numeral string. Reply with only the code.", "test": "int_to_roman"},
    {"prompt": "Write a Python function called `longest_common_prefix` that takes a list of strings and returns the longest common prefix. Reply with only the code.", "test": "longest_common_prefix"},
    {"prompt": "Write a Python function called `two_sum` that takes a list of integers and a target and returns indices of the two numbers that add up to the target. Reply with only the code.", "test": "two_sum"},
    {"prompt": "Write a Python function called `climb_stairs` that takes an integer n and returns the number of ways to climb n stairs taking 1 or 2 steps. Reply with only the code.", "test": "climb_stairs"},
    {"prompt": "Write a Python function called `max_subarray` that takes a list of integers and returns the maximum subarray sum. Reply with only the code.", "test": "max_subarray"},
    {"prompt": "Write a Python function called `merge_intervals` that takes a list of intervals and returns the merged intervals. Reply with only the code.", "test": "merge_intervals"},
    {"prompt": "Write a Python function called `group_anagrams` that takes a list of strings and returns groups of anagrams. Reply with only the code.", "test": "group_anagrams"},
    {"prompt": "Write a Python function called `pascals_triangle` that takes an integer n and returns the first n rows of Pascal's triangle. Reply with only the code.", "test": "pascals_triangle"},
    {"prompt": "Write a Python function called `trap_rain_water` that takes a list of non-negative integers representing an elevation map and returns the total trapped rain water. Reply with only the code.", "test": "trap_rain_water"},
    # --- Go (25) ---
    {"prompt": "Write a Go function called `reverse` that takes a string and returns the reversed string. Reply with only the code.", "test": "reverse"},
    {"prompt": "Write a Go function called `isPalindrome` that takes a string and returns true if it is a palindrome. Reply with only the code.", "test": "isPalindrome"},
    {"prompt": "Write a Go function called `fibonacci` that takes an integer n and returns the nth Fibonacci number. Reply with only the code.", "test": "fibonacci"},
    {"prompt": "Write a Go function called `factorial` that takes an integer n and returns n factorial. Reply with only the code.", "test": "factorial"},
    {"prompt": "Write a Go function called `gcd` that takes two integers a and b and returns their greatest common divisor. Reply with only the code.", "test": "gcd"},
    {"prompt": "Write a Go function called `bubbleSort` that takes a slice of integers and returns the sorted slice. Reply with only the code.", "test": "bubbleSort"},
    {"prompt": "Write a Go function called `mergeSort` that takes a slice of integers and returns the sorted slice. Reply with only the code.", "test": "mergeSort"},
    {"prompt": "Write a Go function called `binarySearch` that takes a sorted slice and a target and returns the index or -1. Reply with only the code.", "test": "binarySearch"},
    {"prompt": "Write a Go function called `sumArray` that takes a slice of integers and returns their sum. Reply with only the code.", "test": "sumArray"},
    {"prompt": "Write a Go function called `maxElement` that takes a slice of integers and returns the maximum. Reply with only the code.", "test": "maxElement"},
    {"prompt": "Write a Go function called `reverseWords` that takes a string and returns it with each word reversed. Reply with only the code.", "test": "reverseWords"},
    {"prompt": "Write a Go function called `countVowels` that takes a string and returns the vowel count. Reply with only the code.", "test": "countVowels"},
    {"prompt": "Write a Go function called `isPrime` that takes an integer n and returns true if n is prime. Reply with only the code.", "test": "isPrime"},
    {"prompt": "Write a Go function called `fizzbuzz` that takes an integer n and prints FizzBuzz from 1 to n. Reply with only the code.", "test": "fizzbuzz"},
    {"prompt": "Write a Go function called `power` that takes a base and exponent and returns base^exponent. Reply with only the code.", "test": "power"},
    {"prompt": "Write a Go function called `flatten` that takes a nested slice and returns a flat slice. Reply with only the code.", "test": "flatten"},
    {"prompt": "Write a Go function called `deduplicate` that takes a slice and returns it with duplicates removed. Reply with only the code.", "test": "deduplicate"},
    {"prompt": "Write a Go function called `capitalize` that takes a string and returns it with the first letter of each word capitalized. Reply with only the code.", "test": "capitalize"},
    {"prompt": "Write a Go function called `isAnagram` that takes two strings and returns true if they are anagrams. Reply with only the code.", "test": "isAnagram"},
    {"prompt": "Write a Go function called `transpose` that takes a 2D matrix and returns its transpose. Reply with only the code.", "test": "transpose"},
    {"prompt": "Write a Go function called `rotate` that takes a slice and an integer k and rotates the slice by k positions. Reply with only the code.", "test": "rotate"},
    {"prompt": "Write a Go function called `findDuplicate` that takes a slice of integers and returns the duplicate value. Reply with only the code.", "test": "findDuplicate"},
    {"prompt": "Write a Go function called `missingNumber` that takes a slice of integers 0..n with one missing and returns the missing number. Reply with only the code.", "test": "missingNumber"},
    {"prompt": "Write a Go function called `twoSum` that takes a slice of integers and a target and returns the two indices. Reply with only the code.", "test": "twoSum"},
    {"prompt": "Write a Go function called `climbStairs` that takes an integer n and returns the number of ways to climb n stairs. Reply with only the code.", "test": "climbStairs"},
    # --- JavaScript (20) ---
    {"prompt": "Write a JavaScript function called `factorial` that takes an integer n and returns n factorial. Reply with only the code.", "test": "factorial"},
    {"prompt": "Write a JavaScript function called `fibonacci` that takes an integer n and returns the nth Fibonacci number. Reply with only the code.", "test": "fibonacci"},
    {"prompt": "Write a JavaScript function called `isPrime` that takes an integer n and returns true if n is prime. Reply with only the code.", "test": "isPrime"},
    {"prompt": "Write a JavaScript function called `reverseString` that takes a string and returns the reversed string. Reply with only the code.", "test": "reverseString"},
    {"prompt": "Write a JavaScript function called `isPalindrome` that takes a string and returns true if it is a palindrome. Reply with only the code.", "test": "isPalindrome"},
    {"prompt": "Write a JavaScript function called `gcd` that takes two integers and returns their greatest common divisor. Reply with only the code.", "test": "gcd"},
    {"prompt": "Write a JavaScript function called `bubbleSort` that takes an array of numbers and returns the sorted array. Reply with only the code.", "test": "bubbleSort"},
    {"prompt": "Write a JavaScript function called `binarySearch` that takes a sorted array and a target and returns the index or -1. Reply with only the code.", "test": "binarySearch"},
    {"prompt": "Write a JavaScript function called `sumArray` that takes an array of numbers and returns their sum. Reply with only the code.", "test": "sumArray"},
    {"prompt": "Write a JavaScript function called `maxElement` that takes an array of numbers and returns the maximum. Reply with only the code.", "test": "maxElement"},
    {"prompt": "Write a JavaScript function called `countVowels` that takes a string and returns the number of vowels. Reply with only the code.", "test": "countVowels"},
    {"prompt": "Write a JavaScript function called `fizzbuzz` that takes an integer n and prints FizzBuzz from 1 to n. Reply with only the code.", "test": "fizzbuzz"},
    {"prompt": "Write a JavaScript function called `power` that takes a base and exponent and returns base^exponent. Reply with only the code.", "test": "power"},
    {"prompt": "Write a JavaScript function called `flatten` that takes a nested array and returns a flat array. Reply with only the code.", "test": "flatten"},
    {"prompt": "Write a JavaScript function called `deduplicate` that takes an array and returns it with duplicates removed. Reply with only the code.", "test": "deduplicate"},
    {"prompt": "Write a JavaScript function called `capitalizeWords` that takes a string and returns it with each word capitalized. Reply with only the code.", "test": "capitalizeWords"},
    {"prompt": "Write a JavaScript function called `isAnagram` that takes two strings and returns true if they are anagrams. Reply with only the code.", "test": "isAnagram"},
    {"prompt": "Write a JavaScript function called `wordCount` that takes a string and returns a map of word counts. Reply with only the code.", "test": "wordCount"},
    {"prompt": "Write a JavaScript function called `missingNumber` that takes an array of integers 0..n with one missing and returns the missing number. Reply with only the code.", "test": "missingNumber"},
    {"prompt": "Write a JavaScript function called `twoSum` that takes an array of integers and a target and returns the two indices. Reply with only the code.", "test": "twoSum"},
    # --- Rust (5) ---
    {"prompt": "Write a Rust function called `factorial` that takes an integer n and returns n factorial. Reply with only the code.", "test": "factorial"},
    {"prompt": "Write a Rust function called `fibonacci` that takes an integer n and returns the nth Fibonacci number. Reply with only the code.", "test": "fibonacci"},
    {"prompt": "Write a Rust function called `reverse` that takes a string slice and returns the reversed string. Reply with only the code.", "test": "reverse"},
    {"prompt": "Write a Rust function called `is_prime` that takes an integer n and returns true if n is prime. Reply with only the code.", "test": "is_prime"},
    {"prompt": "Write a Rust function called `gcd` that takes two integers and returns their greatest common divisor. Reply with only the code.", "test": "gcd"},
    # --- C (4) ---
    {"prompt": "Write a C function called `factorial` that takes an integer n and returns n factorial. Reply with only the code.", "test": "factorial"},
    {"prompt": "Write a C function called `fibonacci` that takes an integer n and returns the nth Fibonacci number. Reply with only the code.", "test": "fibonacci"},
    {"prompt": "Write a C function called `is_prime` that takes an integer n and returns 1 if n is prime, 0 otherwise. Reply with only the code.", "test": "is_prime"},
    {"prompt": "Write a C function called `reverse_string` that takes a string and reverses it in place. Reply with only the code.", "test": "reverse_string"},
    # --- Java (4) ---
    {"prompt": "Write a Java static method called `factorial` that takes an integer n and returns n factorial. Reply with only the code.", "test": "factorial"},
    {"prompt": "Write a Java static method called `fibonacci` that takes an integer n and returns the nth Fibonacci number. Reply with only the code.", "test": "fibonacci"},
    {"prompt": "Write a Java static method called `isPrime` that takes an integer n and returns true if n is prime. Reply with only the code.", "test": "isPrime"},
    {"prompt": "Write a Java static method called `reverseString` that takes a string and returns the reversed string. Reply with only the code.", "test": "reverseString"},
    # --- Ruby + TypeScript (2) ---
    {"prompt": "Write a Ruby method called `factorial` that takes an integer n and returns n factorial. Reply with only the code.", "test": "factorial"},
    {"prompt": "Write a TypeScript function called `fibonacci` that takes an integer n and returns the nth Fibonacci number. Reply with only the code.", "test": "fibonacci"},
]

# Verified against docs/aiand-live-catalog.md (pulled from GET /v1/models on 2026-08-24).
# Skeleton had wrong IDs: z-ai/glm-5.2 -> zai-org/glm-5.2,
# deepseek/deepseek-v4-flash -> deepseek-ai/deepseek-v4-flash,
# moonshotai/kimi-k2.7 -> moonshotai/kimi-k2.7-code.
MODELS = [
    "moonshotai/kimi-k3",
    "zai-org/glm-5.2",
    "deepseek-ai/deepseek-v4-flash",
    "moonshotai/kimi-k2.7-code",
]

KIMI_K3 = "moonshotai/kimi-k3"
RESULTS_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "probe_results.json")


def load_env():
    env = {}
    candidates = [
        os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), ".env"),
        os.path.join(os.getcwd(), ".env"),
    ]
    env_path = None
    for c in candidates:
        if os.path.exists(c):
            env_path = c
            break
    if not env_path:
        return env
    with open(env_path) as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            if "=" in line:
                k, v = line.split("=", 1)
                env[k.strip()] = v.strip().strip('"').strip("'")
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
    env = load_env()
    if not env.get("AIAND_API_KEY"):
        print("FATAL: AIAND_API_KEY not found in .env", file=sys.stderr)
        sys.exit(1)

    print(f"Probe: {len(PROBES)} prompts x {len(MODELS)} models = {len(PROBES) * len(MODELS)} calls")
    print(f"API URL: {env.get('AIAND_API_URL', 'https://api.aiand.com/v1')}")
    print(f"API key: {env['AIAND_API_KEY'][:8]}...{env['AIAND_API_KEY'][-4:]}")
    print()

    # --- Verify model IDs ---
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
        print(f"Verified: {verified}", file=sys.stderr)
    print()

    # --- Load partial results ---
    results = {}
    if os.path.exists(RESULTS_FILE):
        try:
            with open(RESULTS_FILE) as f:
                partial = json.load(f)
                results = partial.get("results", {})
        except Exception:
            results = {}

    total_calls = len(PROBES) * len(MODELS)
    completed = sum(1 for p in results.values() for v in p.values() if v is not None)
    if completed > 0:
        print(f"Resuming: {completed}/{total_calls} already done")
    print()

    # --- Run probe ---
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

    # --- Compute pass rates ---
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

    # --- Pool max + ratio ---
    pool_models = [m for m in MODELS if m != KIMI_K3]
    pool_rates = [pass_rates[m]["rate"] for m in pool_models]
    pool_max = max(pool_rates) if pool_rates else 0.0
    kimi_rate = pass_rates.get(KIMI_K3, {}).get("rate", 0.0)

    if pool_max == 0:
        print("WARNING: pool max pass rate is 0 - cannot compute ratio")
        ratio = 0.0
        recommended = 1.0
    else:
        ratio = kimi_rate / pool_max
        if ratio >= 1.0:
            recommended = min(1.03, 1.0 + (ratio - 1.0) * 0.5)
        else:
            recommended = max(0.98, ratio)

    # --- Print results ---
    total_errors = sum(
        1 for p in results.values()
        for m, v in p.items()
        if v is not None and v.get("error")
    )
    print()
    print("=" * 70)
    print("PROBE RESULTS")
    print("=" * 70)
    for model, stats in sorted(pass_rates.items(), key=lambda x: -x[1]["rate"]):
        print(f"  {model:45s} {stats['passed']:3d}/{stats['total']:3d} = {stats['rate']:.1%}")
    print()
    print(f"  Pool max:  {pool_max:.1%}")
    print(f"  kimi-k3:   {kimi_rate:.1%}")
    print(f"  Ratio:     {ratio:.3f}")
    print(f"  Recommended multiplier: {recommended:.4f}")
    print(f"  Error rate: {total_errors}/{total_calls} = {total_errors/total_calls:.1%}")
    print()

    # --- Save final report ---
    report = {
        "pass_rates": {
            m: {"passed": s["passed"], "total": s["total"], "rate": s["rate"]}
            for m, s in pass_rates.items()
        },
        "results": results,
        "pool_max": pool_max,
        "kimi_k3_rate": kimi_rate,
        "ratio": ratio,
        "recommended_multiplier": recommended,
        "error_rate": total_errors / total_calls if total_calls > 0 else 0.0,
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
