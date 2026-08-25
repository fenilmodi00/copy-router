# Strip-to-aiand Close blockers

Tip: `#28` (`efffe9b`) or newer after provider-surface trim merges. Verification mode: **host + Supabase only** (`make setup` / `make dev`, `PUBSUB_DISABLED=true`). No Docker Compose / `make db` for Close. Analytics stays mounted. Host Close path supersedes Graphite / cloud-VM swarm (plan Appendix E). Media on main: four review PNGs + two ~46 s MP4s under `docs/media/`.

## Only remaining

| Gate | Status | Why |
|---|---|---|
| Operator review click | **OPERATOR** | Post media in chat, then click `cut-gemini` + `fix-tests-smoke`. Paths: `docs/media/cut-gemini-review-messages.png`, `docs/media/cut-gemini-review-stream.png`, `docs/media/cut-gemini-review.mp4`, `docs/media/fix-tests-smoke-review-live.png`, `docs/media/fix-tests-smoke-review-stream.png`, `docs/media/fix-tests-smoke-review.mp4` |
| Close the program | **blocked** | Do not tick Close or invent the review click. Arm / Graphite / swarm boxes stay unchecked on purpose (Appendix E). Residual unchecked file boxes: Gemini translate (deferred; still proxy-wired), hmm/rl/bandit + `sidecars/hmm` + `internal/feedback` (**kept for training/analytics**, not delete targets). |

Do not invent operator approval.
