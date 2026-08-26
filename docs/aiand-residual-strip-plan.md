# aiand residual strip plan

Make the router an OpenAI-compatible product whose first-party path is `ProviderAiand` and the live catalog IDs. Remap Claude-era model strings onto those catalog IDs. Leave Anthropic and Claude Code ingress peripheral. Finish residual docs, UI, handover OpenAI-compat, and obsolete fixtures. PR ids in order are `model-remap`, `docs-residual`, `ui-copy`, `handover-fix`, `fixtures-cleanup`, then optional `glossary`. **Operator skip (2026-08-26): `ui-copy` cancelled; stack is `model-remap` → `docs-residual` → `handover-fix` → `fixtures-cleanup` → optional `glossary`.**

## How to read this

One box is one unit of work. Every box names the evidence that checks it. A nested box is a sub-step of the box above it. Check a box only when its evidence exists, a file, a log line, a screenshot, a test run, or a SHA. The body is a how-to. The appendices explain and record.

The program runs `pstack/skills/poteto-mode/playbooks/autopilot-stack.md`. Cloud owners build and verify each PR. The operator reviews the Graphite stack and lands it. PR ids `ui-copy` and `handover-fix` are review-gated.

Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

## Program checklist

### Arm the program

- [ ] State the protocol and this plan to the operator, then stop. Start execution only on her explicit go.
- [ ] On her go, arm a `/goal` with this exact text. "`docs/aiand-residual-strip-plan.md`. PR ids `model-remap`, `docs-residual`, `ui-copy`, `handover-fix`, `fixtures-cleanup`, optional `glossary` in that stack order. Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Operator lands the Graphite stack. Done when Close the program is checked."
- [ ] Read these from trunk at program start. Re-read them at every tick.
  - [ ] `git show origin/main:pstack/skills/poteto-mode/playbooks/autopilot-stack.md`
  - [ ] `git show origin/main:pstack/skills/swarm/SKILL.md`
  - [ ] `git show origin/main:.cursor/plugins/cursor-team-kit/skills/control-ui/SKILL.md`
  - [ ] `git show origin/main:.cursor/plugins/cursor-team-kit/skills/control-cli/SKILL.md`
  - [ ] `git show origin/main:pstack/skills/poteto-mode/playbooks/opening-a-pr.md`
  - [ ] `git show origin/main:pstack/skills/principle-laziness-protocol/SKILL.md`
  - [ ] `git show origin/main:pstack/skills/principle-sequence-verifiable-units/SKILL.md`
  - [ ] `git show origin/main:pstack/skills/principle-model-the-domain/SKILL.md`
  - [ ] `git show origin/main:pstack/skills/principle-outcome-oriented-execution/SKILL.md`
  - [ ] `git show origin/main:docs/aiand-residual-strip-plan.md`
- [ ] Arm the 30-minute audit tick. In a local session, a real terminal `/loop`. In a cloud root, a cloud-sleeper wake chain. Never leave the cadence to memory.
- [ ] Use this tick prompt, verbatim. "Re-read the execution playbook from trunk and the armed /goal. Audit the operation against both and fix drift in this tick. Probe every active lane and judge progress by side effects only. Stand down a stuck lane and dispatch its replacement now. Then send the operator a status message, whether or not anything changed, with the queue table of PR, owner, state, and head SHA, the verdicts since the last tick, what merged, open operator gates, and blockers."
- [ ] On the operator's hold or stand-down, send every owner a zero-writes order at once.

### Spawn owners

- [ ] Spawn one owner per PR with the full lifecycle the execution playbook names.
- [ ] Follow this dependency graph. Start dependent work only after its parent merges, or base it on the parent branch when the execution playbook stacks.
  - [ ] `model-remap` is first. Branch from `main`.
  - [ ] `docs-residual` and `ui-copy` after `model-remap`. Both may stack in parallel after that parent for Graphite order.
  - [ ] `handover-fix` after `model-remap`. Independent of docs and UI for files. Stack after `ui-copy` only for linear Graphite order.
  - [ ] `fixtures-cleanup` after `handover-fix` and `model-remap` (shared `internal/proxy/` boundary plus remapped force targets).
  - [ ] `glossary` after `docs-residual` (optional).
- [ ] Hold the file boundaries. `model-remap` touches force aliases, live Claude deploy constants, and their unit tests only. `docs-residual` touches only docs and env-example comments. `ui-copy` touches only `frontend/src/**` plus rebuilt `assets/ui`. `handover-fix` touches only `internal/proxy/handover.go` and its summarizer tests. `fixtures-cleanup` touches obsolete skipped tests and Claude-as-deploy fixtures, never historical cluster artifacts. `glossary` creates or edits root `CONTEXT.md` only.
- [ ] Hold the review gate. `ui-copy` and `handover-fix` change an interaction. They wait for the operator's review in chat with screenshots and a video before merge.

### PR mechanics, for every PR

- [ ] Open the PR ready, never draft, with `gh pr create` and `draft: false`, or with Graphite `gt` for a stack.
- [ ] Run the repo's lint and typecheck once before the PR-facing push. Push with hooks on.
- [ ] Run `/deslop` before each commit and `/no-comments` before review.
- [ ] Triage every Bugbot and security-reviewer comment per `../references/bugbot-triage.md`.
- [ ] Rebase onto current trunk before babysit and again before the merge-ready report.

### Verdict and merge, for every PR

- [ ] At the merge-ready head SHA, run the swarm per `pstack/skills/swarm/SKILL.md`. One gates lane. The ten live lanes from the PR's **Verify, live** block. The perf lane from its **Verify, perf** block. One audit lane that reads the diff and the receipts and distrusts the PR body.
- [ ] Clean only when every lane is `PASS`. Findings go back to the owner. A new head gets a fresh swarm and a fresh verdict.
- [ ] Root appends the PR to the Graphite stack. The operator lands the stack.

### Boot recipe, for every live lane

Each live lane runs on its own cloud VM at the PR head. Drive through `control-ui` or `control-cli` from `cursor-team-kit`.

- [ ] `git fetch origin <head-branch> && git checkout <head SHA>`.
- [ ] Export `.env.local` with Supabase session-pooler `DATABASE_URL`, `AIAND_API_KEY`, and ONNX paths. Run `make setup` if schema drifts. Start `make dev`. Wait until `curl -sf http://127.0.0.1:8080/health` returns ok. For docs-only lanes that never boot the binary, skip `make dev` and drive `rg` plus file reads instead.
- [ ] Deliver input only through the control skill's commands. Name the read-only diagnostics.
- [ ] Save every screenshot to `/tmp/swarm-<pr-id>/worker-<n>/<slug>.png` and return the paths with the report.

## Remap Claude model identity to aiand (`model-remap`)

**Depends on.** None.

**Files.**

- [ ] Edit `internal/proxy/force_model.go` (`forceModelAliases` full Claude IDs and values per Appendix F).
- [ ] Edit `internal/proxy/force_model_internal_test.go` (and peers that assert old alias targets).
- [ ] Edit `internal/proxy/loop_detection.go` (`escalateModel`).
- [ ] Edit `internal/proxy/loop_detection_internal_test.go`.
- [ ] Edit `internal/proxy/service.go` constructor default `cyberRefusalFallbackModel` (main already defaults env to kimi-k2.7).
- [ ] Edit related unit tests that pin the old Claude escalate or cyber defaults.

**Build.**

- [ ] Encode Appendix F in `forceModelAliases`. Prefer exact full IDs clients still send (`claude-fable-5`, `claude-opus-4-8`, `claude-opus-5`, `claude-sonnet-5`, `claude-haiku-4-5`, and peers). Align short keys with the same targets. Set `escalateModel` to `z-ai/glm-5.2`. Set the Service constructor cyber fallback default to `moonshotai/kimi-k2.7`. Do not invent new catalog rows. Do not rewrite `artifacts/**`. Do not treat Claude Code as a reason to keep Claude catalog IDs. Use the cloud-agent prompt in Appendix D under `model-remap`.

**You see.**

- [ ] `resolveForceModel("claude-fable-5")` returns catalog id `moonshotai/kimi-k3` and `ProviderAiand`. `resolveForceModel("claude-opus-4-8")` returns `z-ai/glm-5.2`. Loop escalation pins `z-ai/glm-5.2`. Unconfigured cyber fallback default is `moonshotai/kimi-k2.7`.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Force and loop tests. Run `go test ./internal/proxy/ -run 'ForceModel|LoopEscalation|CyberRefusal' -count=1`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.6-fast-xhigh` at the PR head, per the boot recipe.

- [ ] Lane 1. Resolve `claude-fable-5` through the force path. Save `force-fable.png`. Pass when canonical id is `moonshotai/kimi-k3` and provider is `aiand`.
- [ ] Lane 2. Resolve `claude-opus-4-8`. Save `force-opus48.png`. Pass when canonical id is `z-ai/glm-5.2`.
- [ ] Lane 3. Resolve `claude-opus-5` and `opus`. Save `force-opus5.png`. Pass when both land on `z-ai/glm-5.2`.
- [ ] Lane 4. Resolve `claude-sonnet-5` and `claude-haiku-4-5`. Save `force-sonnet-haiku.png`. Pass when targets match Appendix F.
- [ ] Lane 5. Confirm `escalateModel` constant. Save `escalate-const.png`. Pass when it equals `z-ai/glm-5.2`.
- [ ] Lane 6. Confirm Service cyber default. Save `cyber-default.png`. Pass when constructor default is `moonshotai/kimi-k2.7`.
- [ ] Lane 7. Confirm main env default still `ROUTER_CYBER_REFUSAL_FALLBACK_MODEL` → kimi-k2.7. Save `main-cyber.png`. Pass when env default matches.
- [ ] Lane 8. Confirm v0.76 registry and `artifacts/latest` untouched. Save `artifacts-frozen.png`. Pass when `git diff --name-only` excludes `internal/router/cluster/artifacts/`.
- [ ] Lane 9. Boot `make dev` and `/health`. Save `remap-health.png`. Pass when health is 200.
- [ ] Lane 10. Spot-check OpenAI chat completions still the product lead path in README only if already true, else note docs follow in `docs-residual`. Save `openai-lead-note.png`. Pass when this PR did not expand Anthropic as deploy brand.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. `resolveForceModel` microbench or force-model unit suite wall time in ms.
- [ ] Probe. `go test ./internal/proxy/ -run 'ForceModel' -count=20` at trunk and head. Record mean.
- [ ] Baseline. Record the trunk mean first.
- [ ] Rule. Head mean must stay within 2x of trunk mean or the PR fails.

**Review gate.** None. `model-remap` is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, patch-id unchanged.
- [ ] Root appends the PR to the Graphite stack. The operator lands the stack.

## Lead docs with OpenAI-compat and aiand (`docs-residual`)

**Depends on.** `model-remap`.

**Files.**

- [ ] Edit `docs/CONFIGURATION.md`.
- [ ] Edit `README.md`.
- [ ] Edit `.env.example` (handover and cyber comments still naming Claude models).
- [ ] Edit `CONTRIBUTING.md` (adapter path list that still names deleted `providers/{anthropic,openai,google}` packages).
- [ ] Edit `docs/SMOKE.md` and `smoke/mitmproxy/cassettes/README.md` where record path still says Anthropic deploy key.
- [ ] Edit `docs/HOST_WSL_SUPABASE.md` only if primary deploy wording still drifts. Keep the "do not add Anthropic or OpenAI provider keys" stay rule for deploy keys.

**Build.**

- [ ] Rewrite the product lead so OpenAI-compatible `/v1/chat/completions` and `AIAND_*` come first. Put Claude model remap (Appendix F summary) in Configuration. Move Anthropic env, OpenRouter, OpenAI, Google, and gateway vars into a peripheral gateway or BYOK appendix. Keep gateway curls. Do not expand Claude Code as a primary goal. Leaving `/v1/messages` docs is fine as secondary ingress. Fix `.env.example` comments that still say `claude-haiku-4-5` or `claude-sonnet-5`. Use the cloud-agent prompt in Appendix D under `docs-residual`.

**You see.**

- [ ] `docs/CONFIGURATION.md` opens with OpenAI-compat plus aiand. Claude IDs appear only as alias inputs that remap to catalog IDs. `.env.example` handover and cyber comments name aiand catalog models.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Doc drift grep. Run `rg -n 'Recommended baseline|OpenRouter is the recommended|drop-in proxy for Anthropic|ROUTER_HANDOVER_PROVIDER=anthropic|ROUTER_HANDOVER_MODEL=claude|ROUTER_CYBER_REFUSAL_FALLBACK_MODEL=claude' README.md docs/CONFIGURATION.md .env.example CONTRIBUTING.md` and expect no deploy-baseline hits.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.6-fast-xhigh` at the PR head, per the boot recipe.

- [ ] Lane 1. Open `docs/CONFIGURATION.md` product lead. Save `config-openai-aiand.png`. Pass when OpenAI-compat and `AIAND_*` lead before any Anthropic deploy framing.
- [ ] Lane 2. Open the Claude remap section or table. Save `config-remap-table.png`. Pass when `claude-fable-5` and `claude-opus-4-8` map to Appendix F targets.
- [ ] Lane 3. Open the gateway or BYOK appendix. Save `config-gateway-appendix.png`. Pass when `ANTHROPIC_GATEWAY_*` and `anthropic_gateway` live only there.
- [ ] Lane 4. Open `README.md` tagline and architecture. Save `readme-openai.png`. Pass when the product sells OpenAI-compat plus aiand, not Anthropic · OpenAI · Gemini as deploy baseline.
- [ ] Lane 5. Diff `.env.example` handover and cyber comments. Save `env-handover.png`. Pass when comments match `ProviderAiand` and aiand catalog ids, not Claude defaults.
- [ ] Lane 6. Read `CONTRIBUTING.md` adapter list. Save `contributing-adapters.png`. Pass when it no longer lists deleted `internal/providers/{anthropic,openai,google}` packages as present adapters.
- [ ] Lane 7. Read `docs/SMOKE.md` record path. Save `smoke-aiand-key.png`. Pass when record uses `AIAND_API_KEY` and does not require deploy `ANTHROPIC_API_KEY`.
- [ ] Lane 8. Read `smoke/mitmproxy/cassettes/README.md`. Save `cassette-readme.png`. Pass when record wording is router to aiand, not router to Anthropic.
- [ ] Lane 9. Confirm `/v1/messages` may still appear as secondary ingress, not as the product lead. Save `messages-peripheral.png`. Pass when primary install or quickstart is OpenAI-compat or aiand, not Claude Code-first.
- [ ] Lane 10. Follow `docs/HOST_WSL_SUPABASE.md` minimal env list. Save `host-minimal-env.png`. Pass when minimal env is aiand-only and still forbids Anthropic or OpenAI deploy keys.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Cold `curl /health` latency in ms on an unchanged binary boot (docs-only PR must not regress runtime).
- [ ] Probe. `curl -o /dev/null -s -w '%{time_total}' http://127.0.0.1:8080/health` at trunk and at the head, interleaved five times each.
- [ ] Baseline. Record the trunk median first.
- [ ] Rule. Head median must stay within 50 ms of trunk median or the PR fails.

**Review gate.** None. `docs-residual` is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, patch-id unchanged.
- [ ] Root appends the PR to the Graphite stack. The operator lands the stack.

## Strip UI Anthropic deploy copy (`ui-copy`)

**Depends on.** `model-remap`.

**Files.**

- [ ] Edit `frontend/src/app/(app)/settings/providers/page.tsx`.
- [ ] Edit `frontend/src/components/settings/ProviderKeysPanel.tsx`.
- [ ] Edit `frontend/src/components/charts/DrillDownModal.tsx`.
- [ ] Rebuild `assets/ui` from `frontend` (do not hand-edit the build artifact).

**Build.**

- [ ] Replace settings description and `My Anthropic key` placeholder with aiand or BYOK-accurate copy. Add an `aiand` entry to `PROVIDER_BADGE` and demote anthropic badge from brand-primary. Claude Code in the install picker may stay as a peripheral harness label. Do not expand Claude Code UX. Use the cloud-agent prompt in Appendix D under `ui-copy`.

**You see.**

- [ ] `/ui/settings/providers` no longer claims Anthropic or OpenRouter as the BYOK set. Name placeholder is not Anthropic. Drill-down shows a branded `aiand` badge for live `decision_provider=aiand` rows.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Frontend drift grep. Run `rg -n 'Bring your own keys for Anthropic|My Anthropic key' frontend/src` and expect zero hits.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.6-fast-xhigh` at the PR head, per the boot recipe.

- [ ] Lane 1. Open `/ui/settings/providers`. Save `providers-description.png`. Pass when the section blurb matches aiand-only or accurate BYOK wording, not Anthropic-first multi-vendor.
- [ ] Lane 2. Focus the optional key name field. Save `providers-placeholder.png`. Pass when placeholder is not `My Anthropic key`.
- [ ] Lane 3. Confirm provider picker still lists `ai&` / `aiand`. Save `providers-picker.png`. Pass when picker is aiand-only.
- [ ] Lane 4. Open install command picker. Save `install-picker.png`. Pass when OpenAI-compat or curl install is available. Claude Code label may remain without becoming the hero.
- [ ] Lane 5. Open a drill-down row with `decision_provider=aiand` (seed or fixture). Save `badge-aiand.png`. Pass when badge label is `ai&` or `aiand`, not gray raw fallback only.
- [ ] Lane 6. Confirm anthropic badge still exists for BYOK or historical rows if present. Save `badge-anthropic-stay.png`. Pass when anthropic is not using the sole brand-primary treatment as deploy identity.
- [ ] Lane 7. Confirm client-app label `claude-code` still renders if the harness entry stays. Save `client-app-claude.png`. Pass when harness product name is optional stay, not a blocker for remap.
- [ ] Lane 8. Grep built `assets/ui` for `My Anthropic` and `Bring your own keys for Anthropic`. Save `assets-ui-clean.png`. Pass when those strings are absent after rebuild.
- [ ] Lane 9. Reload `/ui/` after rebuild. Save `ui-boot.png`. Pass when HTML loads and logo remains `aiand-auto`.
- [ ] Lane 10. Mobile width on settings providers. Save `providers-mobile.png`. Pass when copy and picker remain readable without Anthropic deploy brand.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Time to first `/ui/settings/providers` paint in ms.
- [ ] Probe. Hard-reload the page at trunk and at the head five times each. Record navigation timing or stopwatch from address commit to visible section title.
- [ ] Baseline. Record the trunk median first.
- [ ] Rule. Head median must stay within 200 ms of trunk median or the PR fails.

**Review gate.** The operator reviews before merge.

- [ ] Copy lane 1, lane 4, and lane 5 screenshots into `docs/media/ui-copy-review-<slug>.png`.
- [ ] Record a 30 to 60 second video of the change on a lane VM. Save it as `docs/media/ui-copy-review.mp4`.
- [ ] Post the screenshots and the video in chat. Stop at merge-ready. Wait for the operator's click.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, patch-id unchanged.
- [ ] Root appends the PR to the Graphite stack. The operator lands the stack.

## Fix handover summarizer for aiand (`handover-fix`)

**Depends on.** `model-remap`. Stack after `ui-copy` for Graphite order only.

**Files.**

- [ ] Edit `internal/proxy/handover.go`.
- [ ] Edit `internal/proxy/handover_internal_test.go`.
- [ ] Edit `cmd/router/main.go` only if `NewProviderSummarizer` must take the provider name (prefer smallest change).

**Build.**

- [ ] Stop hardcoding Anthropic Messages emit, `/v1/messages`, `anthropic-version`, `ProviderAnthropic` labels, and Anthropic response extract for the default aiand OpenAI-compat client. Emit chat completions via `PrepareOpenAI`, parse OpenAI content and usage, and label `ProviderAiand` (or the configured handover provider). Keep `ProviderAnthropic` constant for BYOK. Use the cloud-agent prompt in Appendix D under `handover-fix`.

**You see.**

- [ ] Boot log still wires handover to `aiand`. A unit test captures an OpenAI-shaped body and `ProviderAiand` on the fake client. Switch or compaction summarizer no longer posts Messages JSON to `/chat/completions`.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Summarizer tests. Run `go test ./internal/proxy/ -run 'ProviderSummarizer' -count=1` and `go test ./internal/router/handover/ -count=1`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.6-fast-xhigh` at the PR head, per the boot recipe.

- [ ] Lane 1. Boot `make dev` and read handover wire log. Save `handover-boot.png`. Pass when log shows provider `aiand` and model an aiand catalog id.
- [ ] Lane 2. Unit test proves OpenAI body shape. Save `summarizer-openai-body.png`. Pass when fake client sees chat-completions fields, not Anthropic-only `content` blocks as the sole shape.
- [ ] Lane 3. Unit test proves `Provider()` is `aiand`. Save `summarizer-provider-label.png`. Pass when `Provider()` equals `providers.ProviderAiand`.
- [ ] Lane 4. Unit test proves usage extract from OpenAI `prompt_tokens` / `completion_tokens`. Save `summarizer-usage.png`. Pass when usage fields populate from OpenAI JSON.
- [ ] Lane 5. Confirm `ProviderAnthropic` still exists in `internal/providers/provider.go`. Save `provider-anthropic-stay.png`. Pass when constant remains for BYOK or gateway.
- [ ] Lane 6. Confirm main default remains `ROUTER_HANDOVER_PROVIDER` → `ProviderAiand`. Save `main-default.png`. Pass when default is unchanged.
- [ ] Lane 7. Force a switch path in a controlled test or local session with summarizer enabled. Save `switch-handover.png`. Pass when summarizer call does not fail solely from Messages-on-chat-completions mismatch (success or intentional timeout or empty, not format reject).
- [ ] Lane 8. Compaction path reuses the same adapter. Save `compaction-inherit.png`. Pass when compaction summarizer uses the same OpenAI-compat path under aiand.
- [ ] Lane 9. `go test ./internal/proxy/ -run 'Handover|Compaction' -count=1`. Save `proxy-handover-suite.png`. Pass when EXIT=0.
- [ ] Lane 10. Health still 200 after boot. Save `health-ok.png`. Pass when `/health` is ok.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. `ProviderSummarizer` unit success path wall time in ms.
- [ ] Probe. `go test ./internal/proxy/ -run 'TestProviderSummarizer_Success' -count=20` at trunk and head. Record mean.
- [ ] Baseline. Record the trunk mean first.
- [ ] Rule. Head mean must stay within 2x of trunk mean or the PR fails.

**Review gate.** The operator reviews before merge.

- [ ] Copy lane 1, lane 2, and lane 7 screenshots into `docs/media/handover-fix-review-<slug>.png`.
- [ ] Record a 30 to 60 second video of the change on a lane VM. Save it as `docs/media/handover-fix-review.mp4`.
- [ ] Post the screenshots and the video in chat. Stop at merge-ready. Wait for the operator's click.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, patch-id unchanged.
- [ ] Root appends the PR to the Graphite stack. The operator lands the stack.

## Delete obsolete Anthropic and Claude deploy fixtures (`fixtures-cleanup`)

**Depends on.** `handover-fix`.

**Files.**

- [ ] Delete whole-file obsolete suites first (`internal/proxy/hmm_outcome_internal_test.go`, `sibling_failover_internal_test.go`, `subscription_exhaustion_internal_test.go`, `subscription_oauth_reject_internal_test.go`, `subscription_only_openai_test.go`, `subsidy_test.go`, `internal/router/hmm/e2e_test.go`) when every `Test*` is obsolete-skipped.
- [ ] Delete obsolete-skipped funcs in high-density partial files (`usage_bypass_test.go`, `force_model_exclusions_internal_test.go`, `subscription_internal_test.go`, `turnloop_internal_test.go`, `usage_bypass_internal_test.go`, and peers matching `t.Skip("obsolete on aiand-only catalog")`).
- [ ] Migrate live deploy helpers starting at `internal/proxy/service_test.go` `makeProxyService` (hard-pin still `ProviderAnthropic` + Claude or mismatched catalog model). Also `upstream_models_test.go` and `internal/api/admin/upstream_models_test.go`.
- [ ] Migrate or delete fixtures that still assert Claude catalog IDs as deploy hard-pins (`claude-fable-5`, `claude-opus-4-8`, `claude-haiku-4-5` as expected served models) outside Messages wire tests.
- [ ] Scrub deploy-key Anthropic from `scripts/smoke/run.sh` and `.github/workflows/smoke.yml` (record or compose must use `AIAND_API_KEY`). Cassette README may already be fixed in `docs-residual`.
- [ ] Do not expand work in `internal/api/anthropic/**` beyond leaving peripheral Messages ingress tests.
- [ ] Do not edit historical cluster artifacts under `internal/router/cluster/artifacts/`.

**Build.**

- [ ] Order is delete whole-file dead suites, delete obsolete-skipped funcs, migrate `makeProxyService` and remaining live hard-pins to `ProviderAiand` plus Appendix F catalog IDs, then scrub smoke scripts or workflow off deploy `ANTHROPIC_API_KEY`. Keep BYOK `ProviderAnthropic` fixtures and gateway fixtures. Smoke may keep Messages-shaped client bodies as peripheral ingress. Use the cloud-agent prompt in Appendix D under `fixtures-cleanup`.

**You see.**

- [ ] `rg 'obsolete on aiand-only catalog' --glob '*_test.go'` count drops for cases deleted. Remaining live proxy helpers default to `ProviderAiand` for deploy-shaped construction. Force-alias tests expect Appendix F targets. `go test` packages listed below stay green.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Proxy and related packages. Run `go test ./internal/proxy/ ./internal/proxy/usage/ ./internal/router/hmm/ ./cmd/router/ -count=1`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.6-fast-xhigh` at the PR head, per the boot recipe.

- [ ] Lane 1. Count obsolete skips before and after. Save `skip-count.png`. Pass when deleted cases are gone and surviving skips have an explicit stay reason (BYOK, compose-only, release gate).
- [ ] Lane 2. `go test ./internal/proxy/ -count=1`. Save `proxy-tests.png`. Pass when EXIT=0.
- [ ] Lane 3. `go test ./internal/proxy/usage/ -count=1`. Save `usage-tests.png`. Pass when EXIT=0.
- [ ] Lane 4. `go test ./cmd/router/ -count=1`. Save `cmd-tests.png`. Pass when EXIT=0.
- [ ] Lane 5. Spot-check a migrated helper uses `ProviderAiand` and an aiand catalog id. Save `helper-aiand.png`. Pass when deploy-shaped constructor no longer hardcodes Anthropic or Claude as deploy.
- [ ] Lane 6. Spot-check a BYOK fixture still uses `ProviderAnthropic`. Save `byok-stay.png`. Pass when BYOK cases remain.
- [ ] Lane 7. Confirm force aliases resolve Claude inputs to Appendix F targets. Save `force-alias-remap.png`. Pass when aliases match `model-remap`, not Claude catalog IDs as outputs.
- [ ] Lane 8. Confirm smoke OpenAI or Messages harness types still compile. Save `smoke-build.png`. Pass when `go test ./smoke/ -tags smoke -c` succeeds or package builds.
- [ ] Lane 9. Confirm no edits under historical cluster artifact paths. Save `artifacts-untouched.png`. Pass when `git diff --name-only` excludes frozen artifact blobs.
- [ ] Lane 10. Boot `make dev` and `/health`. Save `fixtures-health.png`. Pass when health is 200.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. `go test ./internal/proxy/ -count=1` wall time in seconds.
- [ ] Probe. Run the package suite at trunk and head three times each.
- [ ] Baseline. Record the trunk median first.
- [ ] Rule. Head median must stay within 30 percent of trunk median or the PR fails.

**Review gate.** None. `fixtures-cleanup` is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, patch-id unchanged.
- [ ] Root appends the PR to the Graphite stack. The operator lands the stack.

## Add optional provider glossary (`glossary`)

**Depends on.** `docs-residual`.

**Files.**

- [ ] Create `CONTEXT.md` at repo root (file is currently missing while `docs/HOST_WSL_SUPABASE.md` and `docs/strip-to-aiand-plan.md` still link it).
- [ ] Edit `docs/HOST_WSL_SUPABASE.md` link text only if needed after create.

**Build.**

- [ ] Write a glossary-only `CONTEXT.md` per `.agents/skills/domain-modeling/CONTEXT-FORMAT.md`. Define Provider, TranslationFamily, CatalogModelID, UpstreamID, ClientModelAlias, ClientHarness, IngressSurface. No implementation detail. Name OpenAI-compat as the primary ingress surface and `/v1/messages` as peripheral. Use the cloud-agent prompt in Appendix D under `glossary`.

**You see.**

- [ ] Root `CONTEXT.md` exists. Terms separate deploy provider `aiand` from Anthropic wire family and optional Claude Code harness. Broken `CONTEXT.md` links resolve.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Glossary shape check. Run `test -f CONTEXT.md` and `rg -n '^\*\*(Provider|TranslationFamily|CatalogModelID|UpstreamID|ClientModelAlias|ClientHarness|IngressSurface)\*\*' CONTEXT.md`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.6-fast-xhigh` at the PR head, per the boot recipe.

- [ ] Lane 1. Open `CONTEXT.md`. Save `glossary-exists.png`. Pass when file opens and has a Language section.
- [ ] Lane 2. Read Provider term. Save `term-provider.png`. Pass when Provider means deploy or BYOK upstream name, not wire format.
- [ ] Lane 3. Read TranslationFamily. Save `term-family.png`. Pass when FamilyAnthropic is wire family, not deploy brand.
- [ ] Lane 4. Read CatalogModelID vs UpstreamID. Save `term-ids.png`. Pass when catalog id and upstream id are distinct.
- [ ] Lane 5. Read ClientModelAlias. Save `term-alias.png`. Pass when Claude strings are aliases onto aiand catalog IDs, not first-party catalog rows.
- [ ] Lane 6. Read ClientHarness. Save `term-harness.png`. Pass when Claude Code is an optional harness, not a provider and not a reason to keep Claude catalog IDs.
- [ ] Lane 7. Read IngressSurface. Save `term-ingress.png`. Pass when `/v1/chat/completions` is primary and `/v1/messages` is peripheral.
- [ ] Lane 8. Confirm no code paths or env recipes inside glossary. Save `glossary-no-impl.png`. Pass when file has no function names or step-by-step setup.
- [ ] Lane 9. Follow link from `docs/HOST_WSL_SUPABASE.md` if present. Save `host-link.png`. Pass when link resolves.
- [ ] Lane 10. Confirm Stay or Avoid lines discourage renaming `ProviderAnthropic` to `ProviderAiand`. Save `avoid-rename.png`. Pass when Avoid covers that collapse.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Time to open and search `CONTEXT.md` with `rg` in ms.
- [ ] Probe. `rg -n 'Provider' CONTEXT.md` at trunk (missing file counts as fail baseline setup) and head, five times.
- [ ] Baseline. Record a trivial `rg` on `README.md` trunk median as the compare baseline if trunk lacks `CONTEXT.md`.
- [ ] Rule. Head `rg` on `CONTEXT.md` must complete under 100 ms median or the PR fails.

**Review gate.** None. `glossary` is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, patch-id unchanged.
- [ ] Root appends the PR to the Graphite stack. The operator lands the stack.

## Close the program

- [ ] Every box above is checked with its evidence.
- [ ] Reply to the operator with the report the execution playbook names.

## Appendix A. Prototype evidence

Product course-correction locked OpenAI-first plus Claude-to-aiand remap. No new prototype branch this planning pass.

Evidence already in tree.

1. Catalog (`internal/router/catalog/catalog.go`) is aiand-only. Exact ids include `moonshotai/kimi-k3` and `z-ai/glm-5.2`.
2. Deploy pointer `artifacts/latest` is `v0.76`. Registry models are already aiand upstream ids. Leave frozen history alone.
3. `forceModelAliases` already remaps short Claude keys. Full ids like `claude-fable-5` and `claude-opus-4-8` still miss exact entries and fall through to `ProviderAnthropic` unknown.
4. Live bug. `escalateModel` is still `claude-opus-5`. Service constructor cyber default is still `claude-sonnet-5` (main env default is already kimi-k2.7).
5. Prior force map sent `opus-4-8` / `claude-4-8` to `moonshotai/kimi-k3`. Operator override maps `claude-opus-4-8` to `z-ai/glm-5.2`. Appendix F follows the operator.

Unproven until owners run live lanes.

1. Whether `ProviderSummarizer` should branch on translation family vs store an explicit provider name from main. Live unit capture in `handover-fix` decides.
2. How many obsolete skips delete cleanly vs need tiny aiand migrations. `fixtures-cleanup` owner counts and decides per file.
3. Whether coral chart tokens in `globals.css` count as Anthropic residue. Locked Change named chart colors in DrillDown badges. Soft coral theme stays unless the operator expands scope.

## Appendix B. Alternatives rejected

Full string-replace of Anthropic to aiand. Rejected. Wire protocol names and BYOK provider constants still need Anthropic as a word.

Public catalog ids like `aiand-sonnet`. Rejected. Keep open-weight catalog ids.

Rename `ProviderAnthropic` to `ProviderAiand`. Rejected. BYOK and gateway still need Anthropic as a provider name.

Rewrite historical cluster artifacts. Rejected. Frozen. `v0.76` already routes aiand registry models.

Preserve Claude catalog IDs so Claude Code keeps working. Rejected by operator. Remap models. Leave `/v1/messages` peripheral if needed. Do not block OpenAI-first.

Keep prior residual-only five-PR stack without remap. Rejected. Live Claude constants and incomplete aliases are product bugs under the new intent.

## Appendix C. Risks

Force aliases change client-visible pins for `claude-opus-4-8` (was kimi-k3, now glm-5.2). Lands in `model-remap`. Owner watches eval or pin tests that assumed kimi.

Loop escalation and cyber defaults leave Claude ids and fail catalog resolve on aiand-only deploys. Lands in `model-remap`. Owner watches `known=false` and Anthropic fallback.

Handover still emits Anthropic Messages while the client is OpenAI-compat. Lands in `handover-fix`. Owner watches format reject and wrong BYOK lookup via `Provider()`.

Docs lead with OpenAI-compat while some install READMEs still feature Claude Code. Lands in `docs-residual`. Owner demotes Claude Code without deleting secondary ingress notes if useful.

UI source fixed but `assets/ui` stale. Lands in `ui-copy`. Owner must rebuild the artifact.

Obsolete skips deleted while a case still encodes a live invariant. Lands in `fixtures-cleanup`. Prefer migrate over delete when the assertion still maps to aiand.

Frozen cluster artifacts touched by accident. Lands in `model-remap` and `fixtures-cleanup`. Diff must exclude those blobs.

Glossary turns into a setup guide. Lands in `glossary`. Keep glossary-only per domain-modeling skill.

## Appendix D. Links, reading list, and cloud-agent prompts

Read before editing.

- `docs/strip-to-aiand-plan.md` (prior strip program, mostly closed)
- `docs/CONFIGURATION.md`
- `cmd/router/main.go` (aiand register, handover default, cyber env default, hard-pin defaults)
- `internal/providers/provider.go`
- `internal/router/catalog/catalog.go`
- `internal/proxy/force_model.go`
- `internal/proxy/loop_detection.go`
- `internal/proxy/handover.go`
- `internal/router/cluster/artifacts/latest` (must stay `v0.76` pointer, no blob edits)
- `frontend/src/app/(app)/settings/providers/page.tsx`
- `frontend/src/components/settings/ProviderKeysPanel.tsx`
- `frontend/src/components/charts/DrillDownModal.tsx`
- `.agents/skills/domain-modeling/SKILL.md` and `CONTEXT-FORMAT.md` for `glossary`

`model-remap` and `handover-fix` get `pstack/skills/how/SKILL.md` before coding. Contested body-shape design may take `pstack/skills/interrogate/SKILL.md`. Every owner keeps a local `decisions.tsv` trail per `pstack/skills/show-me-your-work/SKILL.md` (do not commit).

### Throughput checkpoint

Blocking first steps. Owners read Stay or Change below and Appendix F before any edit. `model-remap` lands first. `handover-fix` runs `how` on `internal/proxy/handover.go` before coding.

Independent workstreams. After `model-remap`, `docs-residual` and `ui-copy` parallel. `handover-fix` also after `model-remap` for correctness of shared proxy package ordering.

Shared mutable state. `internal/proxy/` is shared by `model-remap`, `handover-fix`, and `fixtures-cleanup`. Serialize those three. `assets/ui` is written only by `ui-copy` rebuild.

Smallest safe decomposition. Six PRs as above. Do not merge remap into docs. Do not merge handover and fixture deletion.

### Stay or Change (paste into every owner)

```
STAY
- ProviderAiand, AIAND_*, catalog open-weight IDs
- OpenAI-compat /v1/chat/completions as product lead
- /v1/messages package path as peripheral ingress (leave, do not expand)
- FamilyAnthropic wire helpers where translation still needs them
- ProviderAnthropic for BYOK and gateway only
- Historical cluster artifacts (v0.76 and older blobs)
- CLAUDE.md / AGENTS.md agent mirror filenames
- Optional Claude Code install label as peripheral harness

CHANGE (your PR only)
- model-remap: Appendix F aliases + escalateModel + cyber constructor default
- docs-residual: OpenAI-first docs; Claude IDs only as remap inputs; env comments off Claude
- ui-copy: settings description, placeholders, DrillDown provider badges; rebuild assets/ui
- handover-fix: summarizer body + provider label for aiand OpenAI-compat
- fixtures-cleanup: obsolete Anthropic/Claude-as-deploy fixtures; expect Appendix F outputs
- glossary: CONTEXT.md terms only (OpenAI primary ingress)

REJECT
- Preserving Claude catalog IDs "for Claude Code"
- Full string-replace
- Public ids like aiand-sonnet
- Renaming ProviderAnthropic to ProviderAiand
- Rewriting historical artifacts
- Expanding Anthropic or Claude Code as primary product surface
```

### Cloud-agent prompt (`model-remap`)

```
Workspace /root/copy-router. Implement PR model-remap only from docs/aiand-residual-strip-plan.md.

Product intent. OpenAI-compatible router. First-party path is ProviderAiand and catalog Models. Claude-era IDs are client aliases onto those catalog IDs. Do not preserve Claude models for Claude Code.

Appendix F is authoritative. Encode full IDs and align short keys.
- claude-fable-5 (and fable short keys) -> moonshotai/kimi-k3
- claude-opus-4-8 (and opus-4-8 / claude-4-8 keys) -> z-ai/glm-5.2  [operator override; old map used kimi-k3]
- claude-opus-5 / claude-opus / opus / claude-5 -> z-ai/glm-5.2
- claude-sonnet-5 / claude-sonnet / sonnet -> moonshotai/kimi-k2.7
- claude-sonnet-4-6 / sonnet-4-6 -> deepseek-ai/deepseek-v4-pro
- claude-haiku-4-5 / claude-haiku / haiku -> deepseek-ai/deepseek-v4-flash
Also set escalateModel in loop_detection.go to z-ai/glm-5.2.
Set Service constructor cyberRefusalFallbackModel default to moonshotai/kimi-k2.7 (match main env default).

Stay. Do not edit artifacts/. Do not invent catalog rows. Do not rename ProviderAnthropic. Do not edit handover.go, docs, or frontend.

Verify. go test ./internal/proxy/ -run 'ForceModel|LoopEscalation|CyberRefusal' -count=1. Diff must exclude frozen artifact blobs. Open ready PR. Stop at merge-ready.
```

### Cloud-agent prompt (`docs-residual`)

```
Workspace /root/copy-router. Implement PR docs-residual only from docs/aiand-residual-strip-plan.md. Depends on model-remap.

Goal. Lead README + docs/CONFIGURATION.md with OpenAI-compatible router and aiand-only deploy. Include a short Claude-to-catalog remap summary from Appendix F. Demote Anthropic/OpenRouter/OpenAI/Google env to a peripheral gateway/BYOK appendix. Fix .env.example comments that still name claude-haiku-4-5 or claude-sonnet-5. Fix CONTRIBUTING.md adapter list. Align smoke cassette README record path to AIAND_API_KEY.

Do not expand Claude Code as the primary install story. Leaving secondary /v1/messages notes is fine.

Stay/Change/Reject. Paste the Stay or Change block from the plan. Do not touch frontend, handover.go, force_model.go, tests, or artifacts.

Evidence to cite before editing. docs/CONFIGURATION.md provider table, README tagline, .env.example ROUTER_HANDOVER_* and cyber comments, cmd/router/main.go ProviderAiand register and handover default, Appendix F.

Verify. Run the unit rg from the plan. Boot make dev only if needed for perf health probe. Do not open a draft PR. Open ready PR. Stop at merge-ready for Graphite append.
```

### Cloud-agent prompt (`ui-copy`)

```
Workspace /root/copy-router. Implement PR ui-copy only from docs/aiand-residual-strip-plan.md. Depends on model-remap.

Goal. Fix leftover Anthropic deploy-brand UI copy so the dashboard matches an OpenAI-compat + aiand product.
Files. frontend/src/app/(app)/settings/providers/page.tsx description, ProviderKeysPanel.tsx placeholder "My Anthropic key", DrillDownModal.tsx PROVIDER_BADGE (add aiand, demote anthropic from sole brand-primary). Rebuild assets/ui from frontend. Do not hand-edit assets/ui source maps by text replace alone without rebuild.

Stay. Optional Claude Code harness label if already present. Do not make Claude Code the hero. Wire path hints. Logo aiand-auto already correct.

Reject. Preserving Anthropic as deploy brand. Full repo string-replace.

Verify. rg frontend/src for the forbidden strings. Manual /ui/settings/providers and install picker. Review-gated. Prepare screenshots and a short video, then stop for operator click.
```

### Cloud-agent prompt (`handover-fix`)

```
Workspace /root/copy-router. Implement PR handover-fix only from docs/aiand-residual-strip-plan.md. Depends on model-remap.

Bug. main defaults ROUTER_HANDOVER_PROVIDER=aiand and wires an openaicompat client, but internal/proxy/handover.go ProviderSummarizer still PrepareAnthropic, posts /v1/messages, sets anthropic-version, hardcodes ProviderAnthropic in Provider()/decision/usage, and extracts Anthropic content[] / input_tokens. openaicompat.Client.Proxy always posts to /chat/completions, so aiand gets Messages JSON and summarizer fails or mis-attributes BYOK.

Fix. Emit OpenAI chat completions (PrepareOpenAI), parse OpenAI assistant text and usage, label provider from configured handover provider (aiand). Update handover_internal_test.go fakes and assertions. Smallest change. Do not rename ProviderAnthropic. Do not retarget force aliases (owned by model-remap). Do not edit docs/UI.

Verify. go test ./internal/proxy/ -run 'ProviderSummarizer' -count=1 and related Handover|Compaction tests. Review-gated with boot log + unit proof screenshots and a short video.
```

### Cloud-agent prompt (`fixtures-cleanup`)

```
Workspace /root/copy-router. Implement PR fixtures-cleanup only from docs/aiand-residual-strip-plan.md. Depends on handover-fix and model-remap already stacked or merged.

Goal. Dead obsolete skips. Live tests that still treat ProviderAnthropic or Claude IDs as deploy hard-pins while catalog is aiand-only. Delete dead skips. Migrate live deploy fixtures to ProviderAiand and Appendix F catalog IDs.

Order.
1. Delete whole-file obsolete suites (hmm_outcome_internal_test, sibling_failover_internal_test, subscription_exhaustion_internal_test, subscription_oauth_reject_internal_test, subscription_only_openai_test, subsidy_test, router/hmm/e2e_test) when every Test* is obsolete-skipped.
2. Delete obsolete-skipped funcs in partial files (usage_bypass_test, force_model_exclusions_internal_test, subscription_internal_test, turnloop_internal_test, usage_bypass_internal_test, and peers).
3. Migrate makeProxyService in service_test.go first (central hard-pin). Then upstream_models helpers and remaining live NewService(..., ProviderAnthropic, ...) deploy-shaped call sites. Replace expected Claude served models with Appendix F aiand ids where the assertion is deploy-shaped.
4. Scrub scripts/smoke/run.sh and .github/workflows/smoke.yml off deploy ANTHROPIC_API_KEY. Use AIAND_API_KEY for record or compose.

Stay. BYOK ProviderAnthropic fixtures, gateway fixtures, peripheral Messages ingress tests under internal/api/anthropic if left alone, historical cluster artifacts.

Reject. Mass rewrite of every ProviderAnthropic string. Do not touch handover.go (owned by handover-fix). Do not re-open force alias design (owned by model-remap). Do not preserve Claude catalog IDs as deploy targets.

Verify. go test ./internal/proxy/ ./internal/proxy/usage/ ./internal/router/hmm/ ./cmd/router/ ./internal/api/admin/ -count=1. Optional go test ./smoke/ -tags smoke -count=1. Diff must not include frozen artifact blobs.
```

### Cloud-agent prompt (`glossary`)

```
Workspace /root/copy-router. Implement optional PR glossary only from docs/aiand-residual-strip-plan.md. Depends on docs-residual.

Create root CONTEXT.md using .agents/skills/domain-modeling/CONTEXT-FORMAT.md. Glossary only. Terms. Provider, TranslationFamily, CatalogModelID, UpstreamID, ClientModelAlias, ClientHarness, IngressSurface. Encode OpenAI-compat as primary ingress. Encode /v1/messages as peripheral. Encode Claude strings as ClientModelAlias onto aiand CatalogModelID. Claude Code is optional ClientHarness, not a reason to keep Claude catalog rows. Avoid collapsing ProviderAnthropic into ProviderAiand. No setup recipes. No function names.

Verify. File exists. rg for each term heading. HOST_WSL link resolves if present.
```

## Appendix E. Principles cited

Redesign from First Principles. OpenAI-compat plus aiand models is treated as if it had always been the product base. Claude IDs become aliases, not parallel catalog.

Outcome-Oriented Execution. Converge on OpenAI-first plus remapped identity. Do not keep a Claude-preserving intermediate for Claude Code.

Laziness Protocol. Prefer alias table edits and constant swaps over a new catalog family or second rename program.

Subtract Before You Add. Remap and delete Claude-as-deploy fixtures before adding glossary terms.

Sequence Work into Verifiable Units. Six independently shippable PRs with unit, live, and perf boxes. `model-remap` proves identity before docs and fixtures claim it.

Model the Domain. Keep Provider, TranslationFamily, CatalogModelID, UpstreamID, ClientModelAlias, ClientHarness, and IngressSurface distinct. Claude strings live only in ClientModelAlias.

Boundary Discipline. Handover adapter emits the body the registered client family accepts. Force resolution maps external names at the boundary onto catalog IDs.

Prove It Works. Each PR requires live lanes on the real surface (force resolve, docs read, UI, or `make dev`), not compile-only.

Guard the Context Window. Plan keeps prompts and Appendix F self-contained for cloud owners.

Never Block on the Human. Operator course-correction applied. This plan does not reopen the Claude Code product fork.

Encode Lessons in Structure. Plan is validated by `check-plan.mjs`. Mapping table is Appendix F so owners do not invent targets.

## Appendix F. Claude to aiand catalog mapping

Source of truth for `model-remap` and for docs. Prefer existing `catalog.Models` IDs. Deploy default cluster is `artifacts/latest` → `v0.76` (already aiand registry; do not rewrite blobs).

| Client or force input | aiand catalog ID | Notes |
| --- | --- | --- |
| `claude-fable-5`, `claude-fable`, `fable`, `fable-5`, `fable5` | `moonshotai/kimi-k3` | Operator example. High-tier long-context peer. |
| `claude-opus-4-8`, `opus-4-8`, `opus-4.8`, `claude-4-8`, `claude-4.8` | `z-ai/glm-5.2` | Operator example. Overrides older force map that used kimi-k3. |
| `claude-opus-5`, `claude-opus`, `opus`, `claude`, `anthropic`, `claude-5`, `opus-5`, `opus-5.0`, `opus5` | `z-ai/glm-5.2` | Flagship high tier. Also `escalateModel` target. |
| `claude-opus-4-7`, `claude-opus-4-6` | `z-ai/glm-5.2` | Same opus family. Closest live high-tier peer. Mark if evals assumed kimi. |
| `claude-sonnet-5`, `claude-sonnet`, `sonnet`, `sonnet-5` | `moonshotai/kimi-k2.7` | Existing sonnet aliases. Cyber fallback peer. |
| `claude-sonnet-4-6`, `sonnet-4-6`, `sonnet-4.6` | `deepseek-ai/deepseek-v4-pro` | Existing mid-tier alias. |
| `claude-haiku-4-5`, `claude-haiku`, `haiku`, `haiku-4-5`, `haiku-4.5` | `deepseek-ai/deepseek-v4-flash` | Existing low-tier alias. Hard-pin default peer. |

No clear peer found beyond the table. Prefer nearest existing catalog row by tier. Do not add brand-new catalog models in this program.

Already on aiand (no remap). Handover default `deepseek-ai/deepseek-v4-flash`. Baseline default `moonshotai/kimi-k3`. Compaction large-window `moonshotai/kimi-k3`. Hard-pin fallback `deepseek-ai/deepseek-v4-flash`. Main cyber env default `moonshotai/kimi-k2.7`.
