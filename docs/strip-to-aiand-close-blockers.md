# Strip-to-aiand Close blockers

Tip: `e6aa944` (ResolveBinding index-map fix, #23). Verification mode: **host + Supabase only** (`make setup` / `make dev`, `PUBSUB_DISABLED=true`). No Docker Compose / `make db` for Close. Analytics stays mounted.

## Cleared on host (this lane)

| Gate | Evidence |
|---|---|
| Review PNGs (cut-gemini L2/L3, fix-tests-smoke L7/L8) | `docs/media/cut-gemini-review-messages.png`, `docs/media/cut-gemini-review-stream.png`, `docs/media/fix-tests-smoke-review-live.png`, `docs/media/fix-tests-smoke-review-stream.png` |
| Review MP4s (cut-gemini + fix-tests-smoke) | `docs/media/cut-gemini-review.mp4`, `docs/media/fix-tests-smoke-review.mp4` (~46 s each). Slideshow artifacts from review PNGs + live host `/v1/messages` (WSL `:0` x11grab blank) |
| Analytics kept | `GET /v1/analytics/routing-decisions` with `rk_` → **401** (mounted), not 404 |
| Feedback cut | `GET /f/test` → **404** |
| cut-pubsub binary size | Pre-cut `b08561d` = 83667632 B; head = 79976560 B (**PASS**) |
| cut-pubsub unit | `go test ./internal/auth/ ./cmd/router/ -count=1` on tip → EXIT=0 |
| docs-host health latency | Trunk median ~0.92 ms vs head ~0.86 ms (**PASS**, within 50 ms) |
| cut-sidecars RSS | Trunk ~311 MB, head ~272 MB (**PASS**) |
| Host smoke replay + Rule | Interleaved `SMOKE_HOST=1` twice each: base `e7ad086` mean **47.70 s** (55.37, 40.02) vs head `e6aa944` mean **44.46 s** (44.20, 44.72) = **−6.8%** (**PASS**, ≤25%). Pre-#20 has no host smoke path |
| cut-catalog `ResolveBinding` Rule | Index-map fix `e6aa944` / #23. Interleaved head mean **30.66 ns/op** vs trunk **54.31 ns/op** (**PASS**, −43.5%) |
| cut-gemini router-overhead Rule | `AccessLog` has only total `latency_ms`. Probe used `ProxyMessages complete` `route_ms` on hard-pin `deepseek-ai/deepseek-v4-flash`. Trunk `87cca0a` median **312 ms**; head `faf1507` median **314 ms** (delta **+2 ms**, **PASS**, within 100 ms) |

## Still operator / sibling

| Gate | Status | Why |
|---|---|---|
| Operator review click | **OPERATOR** | Media is on the branch. Post in chat, then click cut-gemini + fix-tests-smoke. Paths: `docs/media/cut-gemini-review-messages.png`, `docs/media/cut-gemini-review-stream.png`, `docs/media/cut-gemini-review.mp4`, `docs/media/fix-tests-smoke-review-live.png`, `docs/media/fix-tests-smoke-review-stream.png`, `docs/media/fix-tests-smoke-review.mp4` |
| cut-gemini unit command | **OPEN** | Exact plan command FAIL: `TestMessagesHandler_AgentShadowDoesNotUpsertRouterUser` expects 200; tip returns 502 (`claude-opus-4-8` not canonical after catalog trim). `translate` + `server` alone ok |
| cut-sidecars-extras unit command | **OPEN** | Exact plan command FAIL: `internal/router/rl` `TestRouteImageTurnDropsImageUnsupported` (no eligible vision candidate after catalog trim) |
| Process / Graphite / swarm ritual boxes | **OPERATOR / historical** | Code path already on main via host Close. Leave open; no operator message superseded the Graphite/swarm ceremony |

## Do not invent

Do not tick Close while unit boxes above stay open, or while review clicks are pending. Absolute head samples without a comparable trunk rebuild do not satisfy trunk-vs-head rules.
