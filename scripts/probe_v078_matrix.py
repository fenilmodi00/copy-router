#!/usr/bin/env python3
"""v0.78 six-model probe matrix: measure all roster models over four prompt sets.

Sets (results feed scripts/build_v078_aiand_overlay.py):
  medium      - the 30 medium coding prompts from probe_glm53_medium.py (substring grading)
  hard        - the 30 hard-tier probes from probe_hard_tier.py (@@P<NN>=<value> grading)
  bfcl        - 30 function-calling prompts from scripts/v078_probe_bfcl.json (exact tool-call grading)
  routerarena - MCQs from scripts/v078_probe_routerarena.json (letter grading, subsampled for spend)

Writes scripts/v078_probe_<set>_results.json in the established schema
(pass_rates / go_no_go / n_prompts / test_date), resumable, with a live
spend ledger (scripts/v078_spend_ledger.json) enforced against SPEND_CAP_USD.

Usage: python3 scripts/probe_v078_matrix.py --set medium|hard|bfcl|routerarena [--limit N] [--concurrency N]
Reads DEV_AIAND_API + AIAND_API_URL from .env.local (fallback .env).
"""
import argparse, concurrent.futures, datetime, importlib.util, json, os, re, sys, threading, time, urllib.request, urllib.error

HERE = os.path.dirname(os.path.abspath(__file__))
MODELS = [
    "zai-org/glm-5.3",
    "moonshotai/kimi-k3",
    "moonshotai/kimi-k2.7-code",
    "motif-technologies/motif-3",
    "deepseek-ai/deepseek-v4-flash",
    "qwen/qwen3.8-27b",
]
# Per-model output caps: glm-5.3 at max effort burns >2k reasoning tokens
# alone (probe_glm53_medium.py); others finish well below. Caps bound worst-
# case spend per call.
MAX_TOKENS = {
    "zai-org/glm-5.3": 16000,
    "moonshotai/kimi-k3": 6000,
    "moonshotai/kimi-k2.7-code": 4000,
    "motif-technologies/motif-3": 4000,
    "deepseek-ai/deepseek-v4-flash": 4000,
    "qwen/qwen3.8-27b": 4000,
}
REASONING_EFFORT = {"zai-org/glm-5.3": "max", "qwen/qwen3.8-27b": "medium"}
# Catalog prices per 1M tokens (v0.77 pinned costs).
PRICE = {
    "zai-org/glm-5.3": (1.0, 4.0), "moonshotai/kimi-k3": (3.0, 12.5),
    "moonshotai/kimi-k2.7-code": (0.75, 3.5), "motif-technologies/motif-3": (0.5, 2.0),
    "deepseek-ai/deepseek-v4-flash": (0.15, 0.25), "qwen/qwen3.8-27b": (0.4, 3.0),
}
SPEND_CAP_USD = 12.0
LEDGER = os.path.join(HERE, "v078_spend_ledger.json")
UA = ("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")


def load_env():
    env = {}
    base = os.path.dirname(HERE)
    for path in [os.path.join(base, ".env.local"), os.path.join(base, ".env"),
                 os.path.join(os.getcwd(), ".env.local"), os.path.join(os.getcwd(), ".env")]:
        if not os.path.exists(path):
            continue
        with open(path) as f:
            for line in f:
                line = line.strip()
                if line and "=" in line and not line.startswith("#"):
                    k, v = line.split("=", 1)
                    env.setdefault(k.strip(), v.strip().strip('"').strip("'"))
    return env


def load_module_probes(script_name):
    spec = importlib.util.spec_from_file_location(script_name, os.path.join(HERE, script_name))
    mod = importlib.util.module_from_spec(spec)
    sys.modules[script_name] = mod
    spec.loader.exec_module(mod)  # module-level constants only; main() is __main__-guarded
    return mod.PROBES


def load_set(name):
    if name == "medium":
        probes = load_module_probes("probe_glm53_medium.py")
        return [{"id": f"m{i:03d}", "prompt": p["prompt"], "grade": ("substring", p["test"])} for i, p in enumerate(probes)]
    if name == "hard":
        probes = load_module_probes("probe_hard_tier.py")
        return [{"id": f"h{i:03d}", "prompt": p["prompt"], "grade": ("token", p["test"])} for i, p in enumerate(probes)]
    if name == "bfcl":
        data = json.load(open(os.path.join(HERE, "v078_probe_bfcl.json")))
        return [{"id": p["id"], "prompt": p["prompt"],
                 "tools": [{"type": "function", "function": f} for f in p.get("functions", [])],
                 "grade": ("toolcalls", p["expected_calls"])} for p in data["prompts"]]
    if name == "routerarena":
        data = json.load(open(os.path.join(HERE, "v078_probe_routerarena.json")))
        return [{"id": p["id"], "prompt": p["prompt"], "grade": ("letter", p["answer_letter"])}
                for p in data["prompts"]]
    raise SystemExit(f"unknown set {name}")


def ledger_add(cost):
    rec = json.load(open(LEDGER)) if os.path.exists(LEDGER) else {"usd": 0.0, "calls": []}
    rec["usd"] = round(rec["usd"] + cost, 6)
    json.dump(rec, open(LEDGER, "w"), indent=1)
    return rec["usd"]


def call_aiand(env, model, prompt, tools=None, max_retries=3):
    url = env.get("AIAND_API_URL", "https://api.aiand.com/v1").rstrip("/") + "/chat/completions"
    payload = {"model": model, "messages": [{"role": "user", "content": prompt}],
               "max_tokens": MAX_TOKENS[model], "temperature": 0.0}
    if tools:
        payload["tools"] = tools
    if model in REASONING_EFFORT:
        payload["reasoning_effort"] = REASONING_EFFORT[model]
    req = urllib.request.Request(url, data=json.dumps(payload).encode(), method="POST")
    req.add_header("Content-Type", "application/json")
    req.add_header("Authorization", f"Bearer {env.get('DEV_AIAND_API', '')}")
    req.add_header("User-Agent", UA)
    for _ in range(max_retries):
        try:
            with urllib.request.urlopen(req, timeout=300) as resp:
                return json.loads(resp.read())
        except urllib.error.HTTPError as e:
            try:
                body = e.read().decode()[:200]
            except Exception:
                body = ""
            print(f"  [ERROR] {model}: HTTP {e.code} {body}", file=sys.stderr)
            if e.code in (400, 401, 403, 404):
                return None  # permanent; retrying won't help
        except Exception as e:
            print(f"  [ERROR] {model}: {e}", file=sys.stderr)
        time.sleep(2)
    return None


def normalize_args(a):
    try:
        return json.loads(a) if isinstance(a, str) else a
    except Exception:
        return {"__unparsed__": str(a)}


def grade(resp_body, grade_spec):
    kind, want = grade_spec
    if resp_body is None:
        return None  # error: excluded from denominator
    msg = (resp_body.get("choices") or [{}])[0].get("message", {}) or {}
    content = msg.get("content") or ""
    if kind == "substring":
        return want.lower() in content.lower()
    if kind == "token":
        # want like "@@p01=6": require the full token incl. value.
        return want.lower() in content.lower()
    if kind == "toolcalls":
        calls = msg.get("tool_calls") or []
        got = [{"name": c.get("function", {}).get("name"),
                "arguments": normalize_args(c.get("function", {}).get("arguments"))} for c in calls]
        if not got and content:
            # Some OpenAI-compat upstreams put the call JSON in content.
            try:
                got = [{"name": c.get("name"), "arguments": normalize_args(c.get("arguments"))}
                       for c in json.loads(re.sub(r"^```(json)?|```$", "", content.strip(), flags=re.M))]
            except Exception:
                got = []
        return got == want
    if kind == "letter":
        m = re.search(r"\b([A-J])\b", content.strip().upper()[:20]) or re.search(r"([A-J])", content.strip().upper()[:8])
        return bool(m) and m.group(1) == want.upper()
    raise ValueError(kind)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--set", required=True, choices=["medium", "hard", "bfcl", "routerarena"])
    ap.add_argument("--limit", type=int, default=0, help="cap number of prompts (spend discipline)")
    ap.add_argument("--concurrency", type=int, default=4, help="parallel prompts (each runs all 6 models serially)")
    args = ap.parse_args()

    env = load_env()
    if not env.get("DEV_AIAND_API"):
        print("FATAL: DEV_AIAND_API not found", file=sys.stderr)
        sys.exit(1)

    probes = load_set(args.set)
    if args.limit:
        probes = probes[:args.limit]
    results_file = os.path.join(HERE, f"v078_probe_{args.set}_results.json")
    results = json.load(open(results_file)).get("results", {}) if os.path.exists(results_file) else {}

    lock = threading.Lock()
    stop = threading.Event()

    def run_prompt(probe):
        """Measure one prompt across all models; returns (key, per-model dict)."""
        key = probe["id"]
        out = {}
        for model in MODELS:
            if stop.is_set():
                out[model] = None
                continue
            body = call_aiand(env, model, probe["prompt"], tools=probe.get("tools"))
            if body is not None:
                u = body.get("usage") or {}
                pin, cin = u.get("prompt_tokens", 0), (u.get("prompt_tokens_details") or {}).get("cached_tokens", 0) or 0
                p = PRICE[model]
                cost = ((pin - cin) * p[0] + cin * p[0] * 0.2 + u.get("completion_tokens", 0) * p[1]) / 1e6
                with lock:
                    total = ledger_add(cost)
                    if total >= SPEND_CAP_USD:
                        stop.set()
            passed = grade(body, probe["grade"])
            out[model] = (None if passed is None else
                          {"passed": passed, "error": body is None,
                           "response_length": len((body.get("choices") or [{}])[0].get("message", {}).get("content") or "")})
        return key, out

    done = 0
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as ex:
        futs = {ex.submit(run_prompt, p): p for p in probes}
        for fut in concurrent.futures.as_completed(futs):
            key, out = fut.result()
            with lock:
                results.setdefault(key, {})
                for model, rec in out.items():
                    if rec is not None:
                        results[key][model] = rec
                done += 1
                json.dump({"results": results}, open(results_file, "w"), indent=1)
            print(f"  [{done:3d}/{len(probes)}] {key} spend=${json.load(open(LEDGER))['usd']:.3f}", flush=True)
    if stop.is_set():
        print(f"SPEND CAP ${SPEND_CAP_USD} hit — partial results kept, aborting")
        sys.exit(2)

    # Aggregate in the established schema.
    pass_rates = {}
    for model in MODELS:
        passed = total = errors = 0
        for p in results.values():
            r = p.get(model)
            if r is None:
                continue
            if r.get("error"):
                errors += 1
                continue
            total += 1
            passed += 1 if r.get("passed") else 0
        pass_rates[model] = {"passed": passed, "total": total, "errors": errors,
                             "rate": round(passed / total, 4) if total else 0.0}
    rates = {m: v["rate"] for m, v in pass_rates.items()}
    leader = max(rates, key=rates.get)
    out = {"pass_rates": pass_rates, "rates": rates, "leader": leader,
           "go_no_go": "PASS",  # informational; overlay gates on hard+medium presence
           "n_prompts": len(probes), "test_date": datetime.date.today().isoformat(),
           "results": results}
    json.dump(out, open(results_file, "w"), indent=1)
    print(json.dumps({k: v for k, v in out.items() if k != "results"}, indent=1))
    print(f"ledger: ${json.load(open(LEDGER))['usd']:.4f}")


if __name__ == "__main__":
    main()
