# Strip-to-aiand Close blockers

Tip: `#27` (`3cde896`). Verification mode: **host + Supabase only** (`make setup` / `make dev`, `PUBSUB_DISABLED=true`). No Docker Compose / `make db` for Close. Analytics stays mounted. Host Close path supersedes Graphite / cloud-VM swarm (plan Appendix E). Media on main: four review PNGs + two ~46 s MP4s under `docs/media/`.

## Only remaining

| Gate | Status | Why |
|---|---|---|
| Operator review click | **OPERATOR** | Post media in chat, then click `cut-gemini` + `fix-tests-smoke`. Paths: `docs/media/cut-gemini-review-messages.png`, `docs/media/cut-gemini-review-stream.png`, `docs/media/cut-gemini-review.mp4`, `docs/media/fix-tests-smoke-review-live.png`, `docs/media/fix-tests-smoke-review-stream.png`, `docs/media/fix-tests-smoke-review.mp4` |
| Close the program | **blocked** | Do not tick Close or invent the review click. Arm / Graphite / swarm boxes stay unchecked on purpose (Appendix E). Residual unchecked file boxes (provider constants, Gemini translate, hmm/rl/bandit packages, `sidecars/hmm`, `internal/feedback`) are not Close gates while live/verify on main already passed. |

Do not invent operator approval.
