# Strip-to-aiand live and perf receipts

Evidence for `docs/strip-to-aiand-plan.md` on `origin/main` after PRs #10–#15.

| Field | Value |
| --- | --- |
| HEAD SHA | `e337559cda9d4aacf1762a6e7d23f47f932304ae` |
| Tip subject | `test: align smoke and unit fixtures to aiand-only catalog (#15)` |
| Host | WSL2, Go 1.25.9, Supabase session pooler via `.env.local` |
| Boot | `go build -tags ORT -o bin/server ./cmd/router`; `./bin/server` |
| Health | `curl -sf http://127.0.0.1:8080/health` → `{"status":"ok"}` |
| Lever | `scripts/strip-verify-live-perf.sh` |
| Machine summary | `docs/media/strip-verify/summary.tsv` |
| Raw receipts | `docs/media/strip-verify/<pr-id>/` |

This file does **not** mark Close complete. Unit evidence already exists from the merge wave. Live and perf boxes below are this host’s verdicts.

## Blockers on this host

- `make setup` / `migrate-up` hit `permission denied for database postgres` on shared Supabase. `make seed` succeeded (documented fallback in `docs/HOST_WSL_SUPABASE.md`).
- `assets/ui` is absent, so `/ui/` returns 404.
- Docker CLI is missing in this WSL distro, so `make smoke` cannot run.
- Perf probes record **absolute** numbers only. No pre-strip trunk binary or process remains on disk, so “vs trunk baseline” rules are not proved.
- No screenshot or video capture tooling. Review-gated items use redacted text receipts under `docs/media/`.
- Operator constraint: keep analytics/training. `/v1/analytics/routing-decisions` stays mounted.

## Perf numbers (absolute)

| PR | Metric | Value | Vs trunk |
| --- | --- | --- | --- |
| docs-host | Cold `/health` median `time_total` | 0.000776 s (~0.8 ms) | Absolute only |
| cut-pubsub | `bin/server` size (`-tags ORT`) | 79990512 bytes (~76.3 MiB) | Absolute only; no `cloud.google.com/go/pubsub` in `go list -deps` |
| cut-catalog | `BenchmarkResolveBinding` | 88.37 ns/op, 0 B/op | Absolute only |
| cut-gemini | Non-stream `/v1/messages` e2e | samples ~1.35–2.20 s (includes upstream) | Absolute only; not router-only overhead |
| cut-sidecars-extras | Idle RSS after boot | VmRSS 295640 kB (~289 MiB) | Absolute only |
| fix-tests-smoke | `make smoke` wall time | SKIP (no Docker) | — |

## docs-host

| Box | Status | Evidence | Note |
| --- | --- | --- | --- |
| live-1 setup | PASS | `docs/media/strip-verify/docs-host/setup-ok.txt` | Seed printed `rk_`; migrate skipped per host doc |
| live-2 dev boot | PASS | `docs/media/strip-verify/docs-host/dev-boot.txt` | Health 200 |
| live-3 skip db | PASS | `docs/media/strip-verify/docs-host/skip-db.txt` | Doc skip list |
| live-4 pubsub off | PASS | `docs/media/strip-verify/docs-host/pubsub-off.txt` | No panic with `PUBSUB_DISABLED=true` |
| live-5 pubsub trap | PASS | `docs/media/strip-verify/docs-host/pubsub-trap.txt` | Doc names forbidden half-config |
| live-6 pooler 5432 | PASS | `docs/media/strip-verify/docs-host/pooler-5432.txt` | Avoids 6543 |
| live-7 ONNX paths | PASS | `docs/media/strip-verify/docs-host/onnx-paths.txt` | `ROUTER_ONNX_*` + `CGO_LDFLAGS` |
| live-8 aiand-only env | PASS | `docs/media/strip-verify/docs-host/aiand-only-env.txt` | Minimal list has `AIAND_API_KEY` only |
| live-9 validate | PASS | `docs/media/strip-verify/docs-host/validate-ok.txt` | 200 |
| live-10 admin UI | FAIL | `docs/media/strip-verify/docs-host/admin-ui.txt` | `/ui/` 404; `assets/ui` missing |
| perf health | PASS* | `docs/media/strip-verify/docs-host/perf-health.txt` | Absolute median only |

## cut-pubsub

| Box | Status | Evidence | Note |
| --- | --- | --- | --- |
| live-1 boot disabled | PASS | `docs/media/strip-verify/cut-pubsub/boot-disabled.txt` | Health 200 |
| live-2 boot unset | SKIP | `docs/media/strip-verify/cut-pubsub/boot-unset.txt` | Second process with all `PUBSUB_*` unset not run |
| live-3 validate | PASS | `docs/media/strip-verify/cut-pubsub/validate-after-cut.txt` | 200 |
| live-4 messages | PASS | `docs/media/strip-verify/cut-pubsub/messages-proxy.txt` | http=200 |
| live-5 install cache | PASS | `docs/media/strip-verify/cut-pubsub/install-cache.txt` | Second validate; no Pub/Sub |
| live-6 compose | PASS | `docs/media/strip-verify/cut-pubsub/compose-no-pubsub.txt` | No `pubsub-emulator` |
| live-7 gomod | PASS | `docs/media/strip-verify/cut-pubsub/gomod-no-pubsub.txt` | No GCP pubsub module |
| live-8 billing noop | PASS | `docs/media/strip-verify/cut-pubsub/billing-noop.txt` | Selfhosted stayed up |
| live-9 buildio docs | PASS | `docs/media/strip-verify/cut-pubsub/buildio-docs.txt` | `PUBSUB_DISABLED=true` documented |
| live-10 restart | PASS | `docs/media/strip-verify/cut-pubsub/restart-idempotent.txt` | Two boots, health 200 |
| perf binary | PASS* | `docs/media/strip-verify/cut-pubsub/perf-binary.txt` | Absolute size only |

## cut-catalog

| Box | Status | Evidence | Note |
| --- | --- | --- | --- |
| live-1 cluster v0.76 | PASS | `docs/media/strip-verify/cut-catalog/cluster-v076.txt` | Boot log scan |
| live-2 route aiand | PASS | `docs/media/strip-verify/cut-catalog/route-aiand.txt` | provider `aiand` |
| live-3 hardpin | PASS | `docs/media/strip-verify/cut-catalog/hardpin-default-retry.txt` | `motif-technologies/motif-3` → 200, `X-Router-Provider: aiand` |
| live-4 no openai bind | PASS | `docs/media/strip-verify/cut-catalog/no-openai-bind.txt` | Catalog tests |
| live-5 handover | PASS | `docs/media/strip-verify/cut-catalog/handover-aiand.txt` | Boot log |
| live-6 excluded providers | SKIP | `docs/media/strip-verify/cut-catalog/excluded-providers.txt` | Admin auth shape not fully exercised |
| live-7 session pin | SKIP | `docs/media/strip-verify/cut-catalog/session-pin-aiand.txt` | Needs pin-row proof |
| live-8 price | PASS | `docs/media/strip-verify/cut-catalog/price-aiand.txt` | Via catalog tests |
| live-9 URL override | SKIP | `docs/media/strip-verify/cut-catalog/aiand-url-override.txt` | No alternate host |
| live-10 restart | PASS | `docs/media/strip-verify/cut-catalog/restart-catalog.txt` | Health 200 |
| perf ResolveBinding | PASS* | `docs/media/strip-verify/cut-catalog/perf-resolvebinding.txt` | 88.37 ns/op absolute |

## cut-gemini

| Box | Status | Evidence | Note |
| --- | --- | --- | --- |
| live-1 gemini 404 | PASS | `docs/media/strip-verify/cut-gemini/gemini-404.txt` | `/v1beta/...` 404 |
| live-2 messages nonstream | PASS | `docs/media/strip-verify/cut-gemini/messages-nonstream.txt` | Content blocks, 200 |
| live-3 messages stream | PASS | `docs/media/strip-verify/cut-gemini/messages-stream.txt` | SSE `message_stop` |
| live-4 count_tokens | PASS | `docs/media/strip-verify/cut-gemini/count-tokens.txt` | Not 404 |
| live-5 openai chat | PASS | `docs/media/strip-verify/cut-gemini/openai-chat.txt` | Mount accepts |
| live-6 tools | PASS | `docs/media/strip-verify/cut-gemini/tools-roundtrip.txt` | http=200 |
| live-7 route preview | PASS | `docs/media/strip-verify/cut-gemini/route-preview.txt` | 200 |
| live-8 translate a2o | PASS | `docs/media/strip-verify/cut-gemini/translate-a2o.txt` | Path via aiand OpenAI-compat |
| live-9 no gemini pkg | PASS | `docs/media/strip-verify/cut-gemini/no-gemini-pkg.txt` | `internal/api/gemini` gone |
| live-10 health | PASS | `docs/media/strip-verify/cut-gemini/health-after-gemini-cut.txt` | 200 |
| perf messages | PASS* | `docs/media/strip-verify/cut-gemini/perf-messages.txt` | E2E only |
| review screenshots | SKIP | `docs/media/cut-gemini-review-messages.txt`, `...-stream.txt` | Text stand-ins; no PNG |
| review video | SKIP | `docs/media/cut-gemini-review.mp4.SKIP.txt` | No capture tooling |

## cut-sidecars-extras

| Box | Status | Evidence | Note |
| --- | --- | --- | --- |
| live-1 no sidecar boot | PASS | `docs/media/strip-verify/cut-sidecars-extras/no-sidecar-boot.txt` | Health 200 |
| live-2 cluster default | PASS | `docs/media/strip-verify/cut-sidecars-extras/cluster-default.txt` | Route decision |
| live-3 hmm | PASS | `docs/media/strip-verify/cut-sidecars-extras/hmm-503.txt` | Header ignored; cluster still wins (200) |
| live-4 rl | PASS | `docs/media/strip-verify/cut-sidecars-extras/rl-503.txt` | Same |
| live-5 bandit | PASS | `docs/media/strip-verify/cut-sidecars-extras/bandit-503.txt` | Same |
| live-6 analytics 404 | FAIL | `docs/media/strip-verify/cut-sidecars-extras/analytics-real-path.txt` | Plan path 404; real `/v1/analytics/routing-decisions` mounted (401 with `rk_`). Analytics kept by operator. |
| live-7 feedback 404 | PASS | `docs/media/strip-verify/cut-sidecars-extras/feedback-404.txt` | `/f/test` 404 |
| live-8 planner pin | SKIP | `docs/media/strip-verify/cut-sidecars-extras/planner-pin.txt` | Multi-turn pin not proved |
| live-9 explore off | PASS | `docs/media/strip-verify/cut-sidecars-extras/explore-off.txt` | Explore env unset |
| live-10 messages | PASS | `docs/media/strip-verify/cut-sidecars-extras/messages-after-extras.txt` | http=200 |
| perf RSS | PASS* | `docs/media/strip-verify/cut-sidecars-extras/perf-rss.txt` | Absolute only |

## fix-tests-smoke

| Box | Status | Evidence | Note |
| --- | --- | --- | --- |
| live-1 smoke basic | SKIP | `docs/media/strip-verify/fix-tests-smoke/smoke-full.txt` | Docker missing |
| live-2 smoke stream | SKIP | `docs/media/strip-verify/fix-tests-smoke/smoke-stream.txt` | Docker missing |
| live-3 smoke provider | PASS | `docs/media/strip-verify/fix-tests-smoke/smoke-provider.txt` | Harness expects aiand |
| live-4 no Anthropic key | SKIP | `docs/media/strip-verify/fix-tests-smoke/smoke-no-ant-key.txt` | Smoke did not run |
| live-5 proxy unit | PASS | `docs/media/strip-verify/fix-tests-smoke/proxy-unit.txt` | `go test ./internal/proxy/` ok |
| live-6 catalog unit | PASS | `docs/media/strip-verify/fix-tests-smoke/catalog-unit.txt` | ok |
| live-7 live hardpin | PASS | `docs/media/strip-verify/fix-tests-smoke/live-hardpin.txt` | Live messages 200 |
| live-8 live stream | PASS | `docs/media/strip-verify/fix-tests-smoke/live-stream.txt` | SSE complete |
| live-9 smoke docs | PASS | `docs/media/strip-verify/fix-tests-smoke/smoke-docs.txt` | Doc commands present |
| live-10 CI paths | PASS | `docs/media/strip-verify/fix-tests-smoke/ci-paths.txt` | Workflow paths |
| perf smoke | SKIP | `docs/media/strip-verify/fix-tests-smoke/perf-smoke.txt` | No Docker |
| review live/stream | SKIP | `docs/media/fix-tests-smoke-review-*.txt` | Text stand-ins |
| review video | SKIP | `docs/media/fix-tests-smoke-review.mp4.SKIP.txt` | No capture |

`PASS*` means the probe ran and recorded a number, but the plan’s “vs trunk baseline” inequality is unproven on this host.

## What remains before Close

1. Clear or accept FAIL: admin UI (`assets/ui` on a lane VM) and analytics-404 vs operator keep-analytics (update plan box or remount expectation).
2. Run SKIP lanes that need a second boot, admin cookie, session-pin DB proof, `AIAND_API_URL` override host.
3. Run smoke + smoke perf on a host with Docker.
4. Capture trunk baselines (binary size, health, ResolveBinding, RSS, smoke time) and re-run head interleaved probes for true PASS on perf rules.
5. Produce PNG/MP4 review artifacts for `cut-gemini` and `fix-tests-smoke`, then operator review clicks.
6. Check every unit/live/perf box in `docs/strip-to-aiand-plan.md` only when its evidence exists. Do not redefine Close as unit-only.
