# Strip-to-aiand Close blockers

Tip: `f59f478` (+ #18 plan ticks). Verification mode: **host + Supabase only** (`make setup` / `make dev`, `PUBSUB_DISABLED=true`). No Docker Compose / `make db` for Close.

## Cleared on host (this lane)

| Gate | Evidence |
|---|---|
| Review PNGs (cut-gemini L2/L3, fix-tests-smoke L7/L8) | `docs/media/cut-gemini-review-*.png`, `docs/media/fix-tests-smoke-review-*.png` from live `/v1/messages` on host |
| Analytics kept | `GET /v1/analytics/routing-decisions` with `rk_` → **401** (mounted), not 404 |
| Feedback cut | `GET /f/test` → **404** |
| cut-pubsub binary size | Rebuilt ORT `bin/server`: pre-cut-pubsub `b08561d` = 83667632 B; tip-era head = 79976560 B (**PASS**, head ≤ trunk) |
| docs-host health latency | Interleaved `curl /health` trunk binary on `:8081` vs host on `:8080`; medians ~0.92 ms vs ~0.86 ms (**PASS**, within 50 ms) |
| cut-sidecars RSS | VmRSS after idle: trunk ~311 MB, head ~272 MB (**PASS**, head ≤ trunk + 5 MB) |

## Still operator / sibling

| Gate | Status | Why |
|---|---|---|
| Review MP4s | **OPERATOR** | No recording; do not invent empty `docs/media/*-review.mp4` |
| Operator review click | **OPERATOR** | Post PNGs (and MP4s if recorded); wait for click on cut-gemini + fix-tests-smoke |
| Smoke replay + smoke wall-time | **SIBLING** | Host replay needs no-compose smoke path; Docker Compose smoke skipped by operator order |
| cut-catalog `ResolveBinding` trunk-vs-head | **UNPROVEN** | `resolve_binding_bench_test.go` did not exist on pre-cut-catalog tip; head absolute only (~100–120 ns/op). Do not invent trunk |
| cut-gemini messages router-overhead | **UNPROVEN** | Plan wants access-log overhead before upstream; e2e `time_total` / `X-Processing-Ms` includes upstream wait |
| Process / Graphite / swarm ritual boxes | **OPERATOR / historical** | Arm, spawn, merge-ritual checkboxes are program process; code path already on main |

## Do not invent

Absolute head samples without a comparable trunk rebuild do not satisfy trunk-vs-head rules. Smoke wall-time is not claimed until host replay is green without Compose.
