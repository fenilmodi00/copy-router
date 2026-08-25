# Strip-to-aiand Close blockers

Tip: `0897b35`+ (this Close docs PR on top). Verification mode: **host + Supabase only** (`make setup` / `make dev`, `PUBSUB_DISABLED=true`). No Docker Compose / `make db` for Close. Analytics stays mounted.

## Cleared on host (this lane)

| Gate | Evidence |
|---|---|
| Review PNGs (cut-gemini L2/L3, fix-tests-smoke L7/L8) | `docs/media/cut-gemini-review-*.png`, `docs/media/fix-tests-smoke-review-*.png` from live `/v1/messages` on host |
| Review MP4s (cut-gemini + fix-tests-smoke) | `docs/media/cut-gemini-review.mp4` and `docs/media/fix-tests-smoke-review.mp4` (~46 s each). Built from review PNGs plus a live host capture (`http=200` non-stream + `message_stop` stream on `deepseek-ai/deepseek-v4-flash`). DISPLAY `:0` x11grab is blank on this WSL host, so the MP4s are slideshow review artifacts, not a live desktop recording |
| Analytics kept | `GET /v1/analytics/routing-decisions` with `rk_` → **401** (mounted), not 404 |
| Feedback cut | `GET /f/test` → **404** |
| cut-pubsub binary size | Rebuilt ORT `bin/server`: pre-cut-pubsub `b08561d` = 83667632 B; tip-era head = 79976560 B (**PASS**, head ≤ trunk) |
| docs-host health latency | Interleaved `curl /health` trunk binary on `:8081` vs host on `:8080`; medians ~0.92 ms vs ~0.86 ms (**PASS**, within 50 ms) |
| cut-sidecars RSS | VmRSS after idle: trunk ~311 MB, head ~272 MB (**PASS**, head ≤ trunk + 5 MB) |
| Host smoke replay | `SMOKE_HOST=1` / `make smoke-host` against `make dev` :8080, EXIT=0, ~48 s wall, no compose / pubsub-emulator (#20 / `e7ad086`) |
| cut-catalog `ResolveBinding` Metric/Probe/Baseline | Interleaved `go test ./internal/router/catalog/ -bench=BenchmarkResolveBinding -benchtime=2s` on pre-cut tip `a1c7434` vs head, same three IDs present on both (`z-ai/glm-5.2`, `moonshotai/kimi-k3`, `motif-technologies/motif-3`), available=`{aiand}`. Original FAIL: trunk mean **59.41 ns/op**, regressing head **77.90 ns/op**. After index-map fix: trunk **54.31 ns/op**, head **30.66 ns/op** (**PASS**) |

## Still operator / sibling

| Gate | Status | Why |
|---|---|---|
| Operator review click | **OPERATOR** | PNGs + MP4s are on the branch/main; post them in chat; wait for click on cut-gemini + fix-tests-smoke |
| Smoke Rule vs trunk | **UNPROVEN** | Head wall ~48 s via `make smoke-host`. Pre-#20 tip has no host smoke path (compose+MITM only). Host-only Close cannot run an interleaved trunk `make smoke-host` mean |
| cut-catalog `ResolveBinding` Rule | **PASS** | Index maps into `Models` remove Model duffcopy on `ResolveBinding`. Interleaved head mean **30.66 ns/op** ≤ trunk **54.31 ns/op** + 20%. Rule ticked in plan |
| cut-gemini messages router-overhead | **UNPROVEN** | Plan wants access-log overhead before upstream. `AccessLog` only emits total `latency_ms` (includes upstream). `ProxyMessages complete` logs `route_ms` (samples ~499–1640 ms on this host, embeds included) but that is not the access-log probe, and no pre-cut-gemini trunk interleaved baseline was taken |
| Process / Graphite / swarm ritual boxes | **OPERATOR / historical** | Arm, spawn, merge-ritual checkboxes are program process; code path already on main. Prefer leave open unless operator marks N/A |

## Do not invent

Absolute head samples without a comparable trunk rebuild do not satisfy trunk-vs-head rules. Host smoke PASS clears replay lanes and the wall-time probe; it does not clear the Rule without a trunk baseline. Access-log `latency_ms` is not router-overhead-before-upstream.
