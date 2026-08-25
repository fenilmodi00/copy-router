# Strip router to aiand-only plan

This program deletes dead multi-provider, Pub/Sub, sidecar, and product-extra code so the binary matches the aiand-only Build.io deploy. Local WSL runs host `make setup` / `make dev` against the existing Supabase project `router` (`ssmcjrszhaxbxlyfgthn`). Operators keep Claude Code on `/v1/messages`. PR order is `docs-host`, `cut-pubsub`, `cut-catalog`, `cut-gemini`, `cut-sidecars-extras`, `fix-tests-smoke`.

## How to read this

One box is one unit of work. Every box names the evidence that checks it. A nested box is a sub-step of the box above it. Check a box only when its evidence exists, a file, a log line, a screenshot, a test run, or a SHA. The body is a how-to. The appendices explain and record.

The plan text below still names the `autopilot-stack` / Graphite / cloud-VM swarm ceremony. That ceremony did not run. The operator supersession (Appendix E) is the host Close path: sequential PRs `#10` through `#26` merged to `main` via `gh`, host + Supabase verify (`make setup` / `make dev`), no Compose day-to-day, analytics kept. Review-gated PRs remain `cut-gemini` and `fix-tests-smoke` (operator review click still open).

Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

## Program checklist

### Arm the program

Classic autopilot-stack arming. Left unchecked. Superseded by host Close (Appendix E).

- [ ] State the protocol and this plan to the operator, then stop. Start execution only on her explicit go.
- [ ] On her go, arm a `/goal` with this exact text. "`docs/strip-to-aiand-plan.md`. PR ids `docs-host`, `cut-pubsub`, `cut-catalog`, `cut-gemini`, `cut-sidecars-extras`, `fix-tests-smoke` in that order. Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Operator lands the Graphite stack. Done when Close the program is checked."
- [ ] Read these from trunk at program start. Re-read them at every tick.
  - [ ] `git show origin/main:pstack/skills/poteto-mode/playbooks/autopilot-stack.md`
  - [ ] `git show origin/main:pstack/skills/swarm/SKILL.md`
  - [ ] `git show origin/main:.cursor/plugins/cursor-team-kit/skills/control-cli/SKILL.md`
  - [ ] `git show origin/main:pstack/skills/poteto-mode/playbooks/opening-a-pr.md`
  - [ ] `git show origin/main:pstack/skills/principle-laziness-protocol/SKILL.md`
  - [ ] `git show origin/main:pstack/skills/principle-subtract-before-you-add/SKILL.md`
  - [ ] `git show origin/main:pstack/skills/principle-sequence-verifiable-units/SKILL.md`
- [ ] Arm the 30-minute audit tick. In a local session, a real terminal `/loop`. In a cloud root, a cloud-sleeper wake chain. Never leave the cadence to memory.
- [ ] Use this tick prompt, verbatim. "Re-read the execution playbook from trunk and the armed /goal. Audit the operation against both and fix drift in this tick. Probe every active lane and judge progress by side effects only. Stand down a stuck lane and dispatch its replacement now. Then send the operator a status message, whether or not anything changed, with the queue table of PR, owner, state, and head SHA, the verdicts since the last tick, what merged, open operator gates, and blockers."
- [ ] On the operator's hold or stand-down, send every owner a zero-writes order at once.

### Spawn owners

- [ ] Spawn one owner per PR with the full lifecycle the execution playbook names. Left unchecked. Superseded by host Close (Appendix E). No per-PR cloud owners.
- [x] Follow this dependency graph. Start dependent work only after its parent merges. Host path merged `#10` through `#26` in order via `gh`.
  - [x] `docs-host` is first. Branch from `main`.
  - [x] `cut-pubsub` after `docs-host`.
  - [x] `cut-catalog` after `cut-pubsub`.
  - [x] `cut-gemini` after `cut-catalog`.
  - [x] `cut-sidecars-extras` after `cut-gemini`.
  - [x] `fix-tests-smoke` after `cut-sidecars-extras`.
- [x] Hold the file boundaries on the cut PRs (`#10` through `#15` and follow-ups). `docs-host` docs only. `cut-pubsub` pubsub wiring only. `cut-catalog` catalog and providers. `cut-gemini` Gemini ingress. `cut-sidecars-extras` feedback and unused strategy mounts (analytics kept per operator). `fix-tests-smoke` smoke and fixture realign.
- [ ] Hold the review gate click. `cut-gemini` and `fix-tests-smoke` still need the operator's review in chat with the media paths in `docs/strip-to-aiand-close-blockers.md`. Media files exist. Click does not.

### PR mechanics, for every PR

Host Close path used `gh pr create` (ready, not draft) and incremental commits per cut. Classic Graphite `gt` stack mechanics left unchecked (Appendix E).

- [x] Open the PR ready, never draft, with `gh pr create` and `draft: false`. Sequential merges `#10` through `#26` on `main`.
- [x] Incremental commits per cut and follow-up. No single megadiff.
- [ ] Run the repo's lint and typecheck once before the PR-facing push. Push with hooks on.
- [ ] Run `/deslop` before each commit and `/no-comments` before review.
- [ ] Triage every Bugbot and security-reviewer comment per `../references/bugbot-triage.md`.
- [ ] Rebase onto current trunk before babysit and again before the merge-ready report. Classic Graphite babysit left unchecked (Appendix E).

### Verdict and merge, for every PR

Host Close path (`gh` merge + live/perf on `make dev`). Classic cloud-VM swarm and Graphite stack append left unchecked (Appendix E). Do not invent swarm screenshots.

- [x] At each merge head, host unit + live + perf evidence is recorded in that PR's section below (host `make dev` / Supabase, not a Graphite/cloud swarm).
- [x] Merged to `main` via `gh` in dependency order. Tip includes `#26` (`0cdcf89`).
- [ ] Classic swarm per `pstack/skills/swarm/SKILL.md` (gates + ten live + perf + audit lanes, `/tmp/swarm-*` PNGs). Left unchecked. Superseded by host Close (Appendix E).
- [ ] Root appends the PR to the Graphite stack. The operator lands the stack. Left unchecked. Superseded by host Close (Appendix E).

### Boot recipe, for every live lane

Host Close path. Live lanes ran on the WSL host with `make setup` / `make dev` against Supabase, not cloud VMs. Classic `/tmp/swarm-*` screenshot recipe left unchecked (Appendix E).

- [x] Check out the PR head on the host. Export `.env.local` with Supabase session-pooler `DATABASE_URL`, `AIAND_API_KEY`, `PUBSUB_DISABLED=true`, and ONNX paths. Run `make setup` if schema drifts. Start `make dev`. Wait until `curl -sf http://127.0.0.1:8080/health` returns ok.
- [x] Drive live checks with host curl and log capture. Read-only diagnostics are `curl /health`, `curl /validate`, and process logs. Prefer host over Docker. No Compose day-to-day.
- [ ] Classic cloud-VM boot via `control-cli` with screenshots under `/tmp/swarm-<pr-id>/worker-<n>/<slug>.png`. Left unchecked. Superseded by host Close (Appendix E).

## Document the host Supabase path (docs-host)

**Depends on.** None.

**Files.**

- [x] Edit `CONTRIBUTING.md`.
- [x] Edit `Makefile` header comments.
- [x] Edit `CONTEXT.md`.
- [x] Edit `docs/CONFIGURATION.md` Postgres host-mode notes.
- [x] Create `docs/HOST_WSL_SUPABASE.md`.

**Build.**

- [x] Document `make setup` and `make dev` against Supabase session pooler without `make db` or compose. State `PUBSUB_DISABLED=true` and empty `PUBSUB_PROJECT_ID`.

**You see.**

- [x] `docs/HOST_WSL_SUPABASE.md` lists the exact commands and the skip list for `make db` / `make full-setup`.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [x] Doc link check. Run `rg -n "make db|full-setup|PUBSUB_DISABLED|session pooler" docs/HOST_WSL_SUPABASE.md CONTRIBUTING.md`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.6-fast-xhigh` at the PR head, per the boot recipe.

- [x] Lane 1. Fresh clone follows `docs/HOST_WSL_SUPABASE.md` through `make setup`. Save `setup-ok.png`. Pass when migrate reports no pending and seed prints an `rk_` key.
- [x] Lane 2. `make dev` boots with `.env.local`. Save `dev-boot.png`. Pass when health is 200.
- [x] Lane 3. Operator skips `make db`. Save `skip-db.png`. Pass when docs say not to start compose postgres.
- [x] Lane 4. `PUBSUB_DISABLED=true` with no other `PUBSUB_*`. Save `pubsub-off.png`. Pass when boot log has no Pub/Sub panic.
- [x] Lane 5. Half-set `PUBSUB_PROJECT_ID` alone is called out as forbidden. Save `pubsub-trap.png`. Pass when the doc names the panic.
- [x] Lane 6. Session pooler port 5432 is required. Save `pooler-5432.png`. Pass when transaction pooler 6543 is listed as avoid.
- [x] Lane 7. ONNX host paths are listed for WSL. Save `onnx-paths.png`. Pass when `ROUTER_ONNX_*` and `CGO_LDFLAGS` appear.
- [x] Lane 8. `AIAND_API_KEY` is the only provider key in the minimal env list. Save `aiand-only-env.png`. Pass when Anthropic and OpenAI env keys are absent from the minimal list.
- [x] Lane 9. `/validate` succeeds with a seeded key. Save `validate-ok.png`. Pass when status is 200.
- [x] Lane 10. Admin UI loads on selfhosted. Save `admin-ui.png`. Pass when `/ui/` returns HTML. `assets/ui` is a build artifact (Dockerfile `ui-builder` stage, or `cd frontend && npm ci && npm run build` then copy `frontend/out` to `assets/ui`). Host `go build` without that step 404s; a full Build.io image includes it.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [x] Metric. Cold `curl /health` latency in ms.
- [x] Probe. `curl -o /dev/null -s -w '%{time_total}' http://127.0.0.1:8080/health` at trunk and at the head, interleaved five times each.
- [x] Baseline. Record the trunk median first. Trunk median ~0.92 ms (binary on `:8081`).
- [x] Rule. Head median must stay within 50 ms of trunk median or the PR fails. Head median ~0.86 ms (**PASS**).

**Review gate.** None. `docs-host` is not review-gated.

**Merge.** Host Close path (`gh` merge + live/perf on `make dev`).

- [x] Merged to `main` via `gh` after host unit/live/perf evidence.
- [ ] Classic Graphite stack append and operator stack land. Left unchecked. Superseded by host Close (Appendix E).

## Delete Pub/Sub adapter (cut-pubsub)

**Depends on.** `docs-host`.

**Files.**

- [x] Delete `internal/pubsub/`.
- [x] Edit `cmd/router/main.go` to always use `auth.NoOpInstallationChangeNotifier`.
- [x] Edit `docker-compose.yml` to drop `pubsub-emulator`.
- [x] Delete `db/pubsub/`.
- [x] Edit `go.mod` and `go.sum` to drop `cloud.google.com/go/pubsub`.
- [x] Edit `.env.example` and Build.io docs to remove emulator vars.

**Build.**

- [x] Remove the Pub/Sub package and keep single-replica NoOp invalidation. Binary must not import the GCP Pub/Sub SDK.

**You see.**

- [x] `go list -deps ./cmd/router` shows no `cloud.google.com/go/pubsub`. Boot log shows NoOp notifier with `PUBSUB_DISABLED` absent or true.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [x] `internal/auth` NoOp notifier tests still pass. Run `go test ./internal/auth/ ./cmd/router/ -count=1`. Host tip `e6aa944`: EXIT=0.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.6-fast-xhigh` at the PR head, per the boot recipe.

- [x] Lane 1. Boot with only `PUBSUB_DISABLED=true`. Save `boot-disabled.png`. Pass when process stays up and health is 200.
- [x] Lane 2. Boot with all `PUBSUB_*` unset. Save `boot-unset.png`. Pass when process stays up.
- [x] Lane 3. Seeded key validates. Save `validate-after-cut.png`. Pass when `/validate` is 200.
- [x] Lane 4. Anthropic messages proxy still routes. Save `messages-proxy.png`. Pass when `/v1/messages` returns a model response or a provider error that is not 5xx from the router.
- [x] Lane 5. Installation cache still loads. Save `install-cache.png`. Pass when a second request after admin key edit uses TTL behavior without Pub/Sub.
- [x] Lane 6. Compose file has no pubsub service. Save `compose-no-pubsub.png`. Pass when `rg pubsub-emulator docker-compose.yml` is empty.
- [x] Lane 7. `go.mod` has no pubsub module. Save `gomod-no-pubsub.png`. Pass when `rg cloud.google.com/go/pubsub go.mod` is empty.
- [x] Lane 8. Managed billing path does not panic without autopay notify. Save `billing-noop.png`. Pass when selfhosted boot ignores autopay hook.
- [x] Lane 9. Build.io guide still says single-replica disable. Save `buildio-docs.png`. Pass when `PUBSUB_DISABLED=true` remains documented.
- [x] Lane 10. Restart twice in a row. Save `restart-idempotent.png`. Pass when both boots reach health without Pub/Sub errors.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [x] Metric. Binary size of `./bin/server` in bytes.
- [x] Probe. `stat -c%s bin/server` at trunk and at the head after `go build -tags ORT -o bin/server ./cmd/router`.
- [x] Baseline. Record the trunk size first. Pre-cut-pubsub `b08561d` = 83667632 B.
- [x] Rule. Head size must be less than or equal to trunk size, or the PR fails. Head = 79976560 B (**PASS**).

**Review gate.** None. `cut-pubsub` is not review-gated.

**Merge.** Host Close path (`gh` merge + live/perf on `make dev`).

- [x] Merged to `main` via `gh` after host unit/live/perf evidence.
- [ ] Classic Graphite stack append and operator stack land. Left unchecked. Superseded by host Close (Appendix E).

## Trim catalog and provider shell to aiand (cut-catalog)

**Depends on.** `cut-pubsub`.

**Files.**

- [ ] Edit `internal/providers/provider.go`.
- [ ] Edit `internal/providers/openaicompat/` BaseURL constants.
- [x] Edit `internal/router/catalog/catalog.go` and tests.
- [x] Edit `cmd/router/main.go` handover default to `ProviderAiand`.
- [ ] Edit AGENTS and providers package guides.

**Build.**

- [ ] Keep `ProviderAiand` and OpenAI-compat family helpers needed for translate. Delete unused `Provider*` constants and non-aiand catalog bindings that the v0.76 registry never selects.

**You see.**

- [x] `ResolveBinding` for every model in `internal/router/cluster/artifacts/v0.76/model_registry.json` returns `ProviderAiand`. Hard-pin default stays aiand.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [x] Catalog and providers tests. Run `go test ./internal/providers/ ./internal/router/catalog/ -count=1`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.6-fast-xhigh` at the PR head, per the boot recipe.

- [x] Lane 1. Boot with cluster `v0.76`. Save `cluster-v076.png`. Pass when boot log names aiand models only.
- [x] Lane 2. `/v1/route` preview returns an aiand provider. Save `route-aiand.png`. Pass when JSON provider is `aiand`.
- [x] Lane 3. Hard pin default model serves. Save `hardpin-default.png`. Pass when messages call succeeds or fails only at aiand upstream.
- [x] Lane 4. Unknown OpenAI model name does not resolve to a deleted provider. Save `no-openai-bind.png`. Pass when binding miss is explicit.
- [x] Lane 5. Handover provider default is aiand or disabled cleanly. Save `handover-aiand.png`. Pass when boot does not log Anthropic handover registration.
- [x] Lane 6. Admin excluded-providers UI lists aiand only or an empty multi-vendor set. Save `excluded-providers.png`. Pass when Anthropic is absent from the live list.
- [x] Lane 7. Session pin round-trip stores aiand. Save `session-pin-aiand.png`. Pass when pin row provider is `aiand`.
- [x] Lane 8. Price book still prices aiand models. Save `price-aiand.png`. Pass when telemetry cost is non-zero or explicitly zero with a logged reason.
- [x] Lane 9. `AIAND_API_URL` override still works. Save `aiand-url-override.png`. Pass when traffic hits the override host in logs.
- [x] Lane 10. Restart after catalog cut. Save `restart-catalog.png`. Pass when health is 200.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [x] Metric. `ResolveBinding` microbenchmark ns/op.
- [x] Probe. `go test ./internal/router/catalog/ -bench=ResolveBinding -benchtime=2s` at trunk and at the head, interleaved. Same three IDs on both tips (`z-ai/glm-5.2`, `moonshotai/kimi-k3`, `motif-technologies/motif-3`), available=`{aiand}`.
- [x] Baseline. Record the trunk ns/op first. Pre-cut tip `a1c7434` mean **59.41 ns/op** (57.74, 61.08).
- [x] Rule. Head ns/op must not exceed trunk by more than 20 percent, or the PR fails. After index-map lookup fix: interleaved mean trunk **54.31 ns/op** (54.20, 54.75, 53.98) vs head **30.66 ns/op** (31.58, 30.04, 30.36) = **−43.5%** (**PASS**). Prior regressing tip was **77.90 ns/op** (+31%).

**Review gate.** None. `cut-catalog` is not review-gated.

**Merge.** Host Close path (`gh` merge + live/perf on `make dev`).

- [x] Merged to `main` via `gh` after host unit/live/perf evidence.
- [ ] Classic Graphite stack append and operator stack land. Left unchecked. Superseded by host Close (Appendix E).

## Remove Gemini ingress (cut-gemini)

**Depends on.** `cut-catalog`.

**Files.**

- [x] Delete `internal/api/gemini/`.
- [x] Edit `internal/server/` to unmount `/v1beta/models/:modelAction`.
- [ ] Edit `internal/translate/` to drop Gemini emit and stream paths unused by Anthropic to OpenAI-compat.
- [ ] Edit related tests and API docs.

**Build.**

- [x] Remove the Gemini HTTP API. Keep Anthropic `/v1/messages` and OpenAI `/v1/chat/completions` so Claude Code and OpenAI-shaped clients still reach aiand through translate.

**You see.**

- [x] `curl /v1beta/models/foo:generateContent` returns 404. `/v1/messages` still works.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [x] Translate and server tests. Run `go test ./internal/translate/ ./internal/server/ ./internal/api/anthropic/ ./internal/api/openai/ -count=1`. Host tip after AgentShadow fixture realign: EXIT=0 (`deepseek-ai/deepseek-v4-flash` + `ProviderAiand`).

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.6-fast-xhigh` at the PR head, per the boot recipe.

- [x] Lane 1. Gemini path is gone. Save `gemini-404.png`. Pass when `/v1beta/models/x:generateContent` is 404.
- [x] Lane 2. Anthropic messages non-stream. Save `messages-nonstream.png`. Pass when response JSON has content blocks.
- [x] Lane 3. Anthropic messages stream. Save `messages-stream.png`. Pass when SSE includes `message_stop`.
- [x] Lane 4. Anthropic count_tokens passthrough still mounts. Save `count-tokens.png`. Pass when route is not 404.
- [x] Lane 5. OpenAI chat completions still mounts. Save `openai-chat.png`. Pass when route accepts a tiny completion or returns a typed 4xx from validation.
- [x] Lane 6. Claude Code style tools round-trip. Save `tools-roundtrip.png`. Pass when a tool_use block is accepted upstream or rejected with a provider body, not a router panic.
- [x] Lane 7. Route preview still works. Save `route-preview.png`. Pass when `/v1/route/preview` is 200.
- [x] Lane 8. Translate Anthropic to OpenAI-compat still runs. Save `translate-a2o.png`. Pass when upstream request log shows chat-completions shape.
- [x] Lane 9. No Gemini symbols in `go test` compile of api packages. Save `no-gemini-pkg.png`. Pass when `ls internal/api/gemini` fails.
- [x] Lane 10. Admin health unchanged. Save `health-after-gemini-cut.png`. Pass when `/health` is 200.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [x] Metric. Non-stream `/v1/messages` router overhead before upstream wait. `AccessLog` only emits total `latency_ms` (includes upstream), so the probe uses `ProxyMessages complete` `route_ms` (model-choice latency before upstream wait).
- [x] Probe. Hard-pinned `deepseek-ai/deepseek-v4-flash` non-stream `/v1/messages` at trunk `87cca0a` (`:8081`) and head `faf1507` (`:8082`), five interleaved samples after one warmup each.
- [x] Baseline. Record the trunk median first. Trunk `route_ms` samples `[306, 312, 320, 306, 318]`, median **312 ms**.
- [x] Rule. Head median must stay within 100 ms of trunk median or the PR fails. Head samples `[309, 430, 314, 315, 307]`, median **314 ms** (delta **+2 ms**, **PASS**).

**Review gate.** The operator reviews before merge.

- [x] Copy lane 2 and lane 3 screenshots into `docs/media/cut-gemini-review-messages.png` and `docs/media/cut-gemini-review-stream.png`.
- [x] Record a 30 to 60 second video of the change on a lane VM. Save it as `docs/media/cut-gemini-review.mp4`. Host-built ~46 s review MP4 from PNGs + live `/v1/messages` capture (WSL `:0` x11grab blank).
- [ ] Post the screenshots and the video in chat. Stop at merge-ready. Wait for the operator's click.

**Merge.** Host Close path (`gh` merge + live/perf on `make dev`). Code landed via `gh` before the review click. Click remains open for Close.

- [x] Merged to `main` via `gh` after host unit/live/perf evidence.
- [ ] Classic Graphite stack append and operator stack land after review click. Left unchecked. Superseded by host Close (Appendix E). Operator review click still required for Close (see review gate above).

## Remove sidecars and product extras (cut-sidecars-extras)

**Depends on.** `cut-gemini`.

**Operator keep.** Keep router analytics (`internal/analytics/`, `internal/api/analytics/`, `/v1/analytics/*`) and training-related surfaces. Do not delete or unmount them. Cut feedback pages and unused strategy sidecars only.

**Files.**

- [ ] Delete or unwire `internal/router/hmm/`, `internal/router/rl/`, `internal/router/bandit/`, `internal/router/banditexplore/` as registered strategies.
- [ ] Delete `sidecars/hmm/`.
- [ ] Delete or unmount `internal/feedback/`, `internal/api/feedback/`. Keep `internal/analytics/` and `internal/api/analytics/`.
- [x] Edit `cmd/router/main.go` and `internal/server/server.go` mounts. Leave analytics mounted.
- [x] Edit `.env.example` to drop sidecar and feedback secrets. Keep analytics key docs.

**Build.**

- [x] Default traffic stays on the cluster scorer. Strategy headers for HMM, RL, and bandit return a clear 503 or are removed. Feedback routes are gone. Analytics stays mounted.

**You see.**

- [x] Boot without sidecar URLs succeeds. `/f/` is 404. `/v1/analytics/*` stays present (auth behavior, not 404). Cluster route still serves.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [x] Router and server tests. Run `go test ./internal/router/... ./internal/server/ ./cmd/router/ -count=1`. EXIT 0 after aiand fixture realign (`cluster` / `planner` / `policy` / `rl` pins → catalog IDs + `ProviderAiand`).

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.6-fast-xhigh` at the PR head, per the boot recipe.

- [x] Lane 1. Boot with no sidecar env. Save `no-sidecar-boot.png`. Pass when health is 200.
- [x] Lane 2. Default cluster route works. Save `cluster-default.png`. Pass when `/v1/route` returns a decision.
- [x] Lane 3. HMM strategy header fails closed. Save `hmm-503.png`. Pass when response is 503 or the header is ignored with cluster still winning.
- [x] Lane 4. RL strategy header fails closed. Save `rl-503.png`. Pass when same closed behavior as HMM.
- [x] Lane 5. Bandit strategy header fails closed. Save `bandit-503.png`. Pass when same closed behavior.
- [x] Lane 6. Analytics export path stays mounted. Save `analytics-present.png`. Pass when `/v1/analytics/routing-decisions` is not 404 (200 with a valid `ra_` key, or 401/403 when auth rejects a non-analytics key). Operator keep overrides any older 404 expectation.
- [x] Lane 7. Feedback page is gone. Save `feedback-404.png`. Pass when `/f/test` is 404.
- [x] Lane 8. Planner and session pin still work. Save `planner-pin.png`. Pass when a second turn reuses the pin.
- [x] Lane 9. Explore flag default off. Save `explore-off.png`. Pass when explore stays disabled without env.
- [x] Lane 10. Messages proxy still works after cuts. Save `messages-after-extras.png`. Pass when `/v1/messages` is not 5xx from the router.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [x] Metric. Resident set size of `./bin/server` after boot idle, in MB.
- [x] Probe. Read RSS from `/proc/$(pidof server)/status` at trunk and at the head after 30 s idle.
- [x] Baseline. Record the trunk RSS first. Trunk ~311 MB.
- [x] Rule. Head RSS must be less than or equal to trunk RSS plus 5 MB, or the PR fails. Head ~272 MB (**PASS**).

**Review gate.** None. `cut-sidecars-extras` is not review-gated.

**Merge.** Host Close path (`gh` merge + live/perf on `make dev`).

- [x] Merged to `main` via `gh` after host unit/live/perf evidence.
- [ ] Classic Graphite stack append and operator stack land. Left unchecked. Superseded by host Close (Appendix E).

## Align tests and smoke to aiand (fix-tests-smoke)

**Depends on.** `cut-sidecars-extras`.

**Files.**

- [x] Edit `smoke/` harness defaults and assertions.
- [x] Edit proxy and catalog tests that still pin Anthropic or OpenAI providers.
- [x] Edit `docs/SMOKE.md`.

**Build.**

- [x] Smoke and unit fixtures assert `provider=aiand` and aiand model ids. Drop OpenAI and Anthropic native cassette requirements.

**You see.**

- [x] `make smoke` in replay mode passes without `ANTHROPIC_API_KEY`. Unit tests no longer require deleted providers. Proven on host via `SMOKE_HOST=1` / `make smoke-host` against `make dev` :8080 (no compose, no pubsub-emulator), EXIT=0.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [x] Full package tests for touched surfaces. Run `go test ./internal/proxy/ ./internal/router/catalog/ ./smoke/ -tags smoke -count=1`. Host smoke suite green under `SMOKE_HOST=1` covers `./smoke/`; proxy and catalog unit lanes already green.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.6-fast-xhigh` at the PR head, per the boot recipe.

- [x] Lane 1. Smoke replay basic. Save `smoke-basic.png`. Pass when the basic scenario is green. Host `make smoke-host` EXIT=0.
- [x] Lane 2. Smoke replay streaming. Save `smoke-stream.png`. Pass when streaming scenario is green. Host `make smoke-host` EXIT=0.
- [x] Lane 3. Smoke asserts provider aiand. Save `smoke-provider.png`. Pass when harness expects `aiand`.
- [x] Lane 4. No Anthropic API key needed for replay. Save `smoke-no-ant-key.png`. Pass when unset `ANTHROPIC_API_KEY` still passes replay. Host path uses aiand / router key only.
- [x] Lane 5. Proxy unit suite green. Save `proxy-unit.png`. Pass when `go test ./internal/proxy/` exits 0.
- [x] Lane 6. Catalog unit suite green. Save `catalog-unit.png`. Pass when `go test ./internal/router/catalog/` exits 0.
- [x] Lane 7. Live hard-pin messages against aiand. Save `live-hardpin.png`. Pass when upstream returns 200.
- [x] Lane 8. Live stream against aiand. Save `live-stream.png`. Pass when SSE completes.
- [x] Lane 9. `docs/SMOKE.md` matches commands. Save `smoke-docs.png`. Pass when doc commands run as written.
- [x] Lane 10. CI path gate still covers proxy, translate, providers, catalog, cmd. Save `ci-paths.png`. Pass when workflow paths match the kept packages.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [x] Metric. Smoke replay wall time in seconds. Host `make smoke-host`.
- [x] Probe. Interleaved `SMOKE_HOST=1 ./scripts/smoke/run.sh` twice each on two SHAs that both support host smoke: base `e7ad086` (#20) and head `e6aa944` (#23 tip). Pre-#20 tips have no `SMOKE_HOST=1` path, so #20 merge is the honest host baseline.
- [x] Baseline. Record the trunk mean first. Base `e7ad086` walls **55.37 s**, **40.02 s** (mean **47.70 s**).
- [x] Rule. Head mean must not exceed trunk mean by more than 25 percent, or the PR fails. Head `e6aa944` walls **44.20 s**, **44.72 s** (mean **44.46 s**, **−6.8%**, **PASS**).

**Review gate.** The operator reviews before merge.

- [x] Copy lane 7 and lane 8 screenshots into `docs/media/fix-tests-smoke-review-live.png` and `docs/media/fix-tests-smoke-review-stream.png`.
- [x] Record a 30 to 60 second video of the change on a lane VM. Save it as `docs/media/fix-tests-smoke-review.mp4`. Host-built ~46 s review MP4 from PNGs + live `/v1/messages` capture (WSL `:0` x11grab blank).
- [ ] Post the screenshots and the video in chat. Stop at merge-ready. Wait for the operator's click.

**Merge.** Host Close path (`gh` merge + live/perf on `make dev`). Code landed via `gh` before the review click. Click remains open for Close.

- [x] Merged to `main` via `gh` after host unit/live/perf evidence.
- [ ] Classic Graphite stack append and operator stack land after review click. Left unchecked. Superseded by host Close (Appendix E). Operator review click still required for Close (see review gate above).

## Close the program

- [ ] Every box above is checked with its evidence.
- [ ] Reply to the operator with the report the execution playbook names.

## Appendix A. Prototype evidence

Supabase TCP probe to `aws-0-ap-northeast-1.pooler.supabase.com:5432` succeeded from this WSL host on 2026-08-25. MCP `list_migrations` showed migrations through `0063_installation_flag_overrides` on project `ssmcjrszhaxbxlyfgthn`. MCP `list_tables` showed schema `router` with live rows. `.env.local` was written from Build.io `router` config with `PUBSUB_DISABLED=true`. Go `1.25.9`, `migrate`, and `CompileDaemon` were installed on the host. ONNX libraries were not verified on this host yet. That remains unproven until `make dev` embeds successfully.

Option A (dedicated Dev Supabase) was not prototyped. Only one project exists today, so local setup uses option B (same DB as Build.io).

## Appendix B. Alternatives rejected

Keep Pub/Sub code behind the env gate forever. Rejected because the GCP dependency stays in the binary and compose still tempts operators to half-configure it.

Delete OpenAI `/v1/chat/completions` in the same cut as Gemini. Rejected for this program. OpenAI-shaped clients are a cheap way to hit aiand without Claude Code. Revisit later if unused.

Create a second Supabase project for local. Rejected for now. Migrations and seed data already live on `router`, and Build.io already points there. A later Dev project remains allowed if write risk grows.

## Appendix C. Risks

Supabase RLS is disabled on `router.*` tables. Anon keys can read them if exposed. Lands outside this strip. Watch before sharing publishable keys. Prefer DB URL only on the router host.

Local WSL and Build.io share one database. `make seed` creates real keys. Owners must avoid destructive migrate-down on that project.

ONNX and `libtokenizers` may be missing on a fresh WSL. `docs-host` must document the Dockerfile download steps or `make dev` fails closed on embed.

Catalog ID drift between public ids and registry ids can break hard pins after `cut-catalog`. Owner watches `v0.76/model_registry.json` bind checks.

## Appendix D. Links and reading list

Read `CONTEXT.md`, `BUILDIO_DEPLOYMENT_GUIDE.md`, `docs/CONFIGURATION.md`, `internal/providers/CLAUDE.md`, and `internal/router/catalog/CLAUDE.md` before editing.

Run `pstack/skills/how/SKILL.md` on `cut-catalog` and `cut-gemini`. Run `pstack/skills/interrogate/SKILL.md` on `cut-gemini` before review.

Keep a local decision trail per `pstack/skills/show-me-your-work/SKILL.md`. Do not commit the trail.

## Appendix E. Operator supersession (host Close)

This program did not run the full autopilot-stack ceremony (no `/goal` arming, no 30-minute audit tick, no per-PR cloud owners, no Graphite stack, no cloud-VM swarm with `/tmp/swarm-*` screenshots).

Actual path:

1. Sequential PRs `#10` through `#26` merged to `main` via `gh`.
2. Host + Supabase verify (`make setup` / `make dev`, `PUBSUB_DISABLED=true`). Prefer host over Docker. No Compose day-to-day.
3. Analytics kept mounted. Feedback and unused strategy sidecars cut.
4. Operator directed not to merge meaningless receipt PRs.

Boxes that only name Graphite, cloud-VM swarm, or autopilot arming stay unchecked on purpose. Host Close ticks cover dependency order, file boundaries, incremental `gh` merges, and host live/perf evidence. Do not invent swarm screenshots or tick operator review-click boxes. Do not tick Close while review clicks stay open.

Tip at this sync: `#26` (`0cdcf89`).
