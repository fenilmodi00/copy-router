# Strip-to-aiand Close blockers

Tip: post-`dbaf94d` Close lane (ResolveBinding `e6aa944` / #23 still in ancestry). Verification mode: **host + Supabase only** (`make setup` / `make dev`, `PUBSUB_DISABLED=true`). No Docker Compose / `make db` for Close. Analytics stays mounted.

## Cleared on host (this lane)

| Gate | Evidence |
|---|---|
| Review PNGs (cut-gemini L2/L3, fix-tests-smoke L7/L8) | `docs/media/cut-gemini-review-messages.png`, `docs/media/cut-gemini-review-stream.png`, `docs/media/fix-tests-smoke-review-live.png`, `docs/media/fix-tests-smoke-review-stream.png` |
| Review MP4s (cut-gemini + fix-tests-smoke) | `docs/media/cut-gemini-review.mp4`, `docs/media/fix-tests-smoke-review.mp4` (~46 s each). Slideshow artifacts from review PNGs + live host `/v1/messages` (WSL `:0` x11grab blank) |
| Analytics kept | `GET /v1/analytics/routing-decisions` with `rk_` → **401** (mounted), not 404 |
| Feedback cut | `GET /f/test` → **404** |
| cut-pubsub binary size | Pre-cut `b08561d` = 83667632 B; head = 79976560 B (**PASS**) |
| cut-pubsub unit | `go test ./internal/auth/ ./cmd/router/ -count=1` → EXIT=0 |
| cut-gemini unit | `go test ./internal/translate/ ./internal/server/ ./internal/api/anthropic/ ./internal/api/openai/ -count=1` → EXIT=0 after AgentShadow fixture uses `deepseek-ai/deepseek-v4-flash` + `ProviderAiand` |
| docs-host health latency | Trunk median ~0.92 ms vs head ~0.86 ms (**PASS**, within 50 ms) |
| cut-sidecars RSS | Trunk ~311 MB, head ~272 MB (**PASS**) |
| Host smoke replay + Rule | Interleaved `SMOKE_HOST=1` twice each: base `e7ad086` mean **47.70 s** vs head `e6aa944` mean **44.46 s** = **−6.8%** (**PASS**, ≤25%) |
| cut-catalog `ResolveBinding` Rule | Index-map fix `e6aa944` / #23. Head mean **30.66 ns/op** vs trunk **54.31 ns/op** (**PASS**) |
| cut-gemini router-overhead Rule | `AccessLog` only has total `latency_ms`. Probe used `ProxyMessages complete` `route_ms`. Trunk `87cca0a` median **312 ms**; head `faf1507` median **314 ms** (**PASS**, +2 ms) |

## Still operator / sibling

| Gate | Status | Why |
|---|---|---|
| Operator review click | **OPERATOR** | Post media in chat, then click cut-gemini + fix-tests-smoke. Paths: `docs/media/cut-gemini-review-messages.png`, `docs/media/cut-gemini-review-stream.png`, `docs/media/cut-gemini-review.mp4`, `docs/media/fix-tests-smoke-review-live.png`, `docs/media/fix-tests-smoke-review-stream.png`, `docs/media/fix-tests-smoke-review.mp4` |
| cut-sidecars-extras unit command | **CLEARED** | `go test ./internal/router/... ./internal/server/ ./cmd/router/ -count=1` EXIT 0 after aiand fixture realign (planner/policy/rl/cluster). Packages kept; fixtures only |
| Process / Graphite / swarm ritual boxes | **OPERATOR / historical** | Code path already on main via host Close. Leave open; no operator message superseded the Graphite/swarm ceremony |

## Cleared after fixture realign

| Gate | Evidence |
|---|---|
| cut-sidecars Verify, unit | `go test ./internal/router/... ./internal/server/ ./cmd/router/ -count=1` EXIT 0 on host tip after aiand catalog fixture realign |

## Do not invent

Do not tick Close while operator review clicks or ritual boxes stay open. Unit gate for cut-sidecars is cleared.
