# internal/translate — AGENTS

> **Mirror notice.** Verbatim sync with [CLAUDE.md](CLAUDE.md). **Update both together** — divergence = bug.

Cross-format wire-format conversion. Pure functions, no I/O, no provider knowledge, no domain types. Read [root CLAUDE.md](../../CLAUDE.md) first.

## Scope

Anthropic ⇄ OpenAI via a `RequestEnvelope` intermediate + per-target `emit_*.go` files. Native Gemini ingress and emit were removed; Anthropic-path defensive strips for Gemini-only fields (e.g. `thought_signature`) remain.

**Only [`../proxy`](../proxy) calls this package.** Providers stay ignorant of cross-format concerns.


## Adding a wire-format pair

When a new inbound format needs to talk to an existing upstream provider with a different wire format:

1. **Add conversion functions to this package.** Pure functions only — no I/O, no provider knowledge, no domain types.
2. **If response streaming, adapt [`stream.go`](stream.go)** or add a sibling decorator. Decorators wrap `http.ResponseWriter` and translate on the fly so we never buffer entire responses. Use [`../sse`](../sse) for zero-alloc SSE framing. A decorator that only prepends synthetic content to a stream (like the `*RoutingMarkerWriter` types) should embed `sse.ChunkedWriter` for the shared `Header`/`WriteHeader`/`Flush`/`FlushEvent` + streaming-detection bookkeeping, and add only its format-specific `Write`/`emit*` methods. A full response translator (buffers to translate one wire format into another, e.g. `AnthropicSSETranslator`) has enough divergent `WriteHeader`/streaming logic that it should NOT embed `ChunkedWriter` — reuse only `sse.FlushWriter(bw, flusher)` for its `flushEvent` helper.
3. **Compose the new translation in `proxy.Service.Proxy*`.** Proxy is the only caller of `translate`.


## Anthropic-specific stripping (load-bearing)

Anthropic-only fields (`thinking`, `cache_control`, `metadata`, Anthropic beta headers) are stripped at translation time **and again defensively in the OpenAI / openaicompat adapters**. Keep both checks — belt-and-suspenders is intentional because the field set drifts as Anthropic adds beta features.

## Prefix-stable system handling (load-bearing)

Anthropic 400s on `role:"system"` inside `messages`, so `hoistAnthropicSystemMessages` clears them — but only the **leading** run is hoisted into `system`. A mid-conversation system message is demoted to `user` **in place**. Hoisting it instead would move its text in front of the whole history, so a client that emits a system reminder per turn (Claude Code) shifts the cached prefix on every turn and re-writes the entire prompt; prod traffic showed ~890k cache-creation tokens per turn against a flat 17.5k read.

## `<think>` content-channel extraction (gated)

Some OpenAI-compat upstreams (today `xiaomi/mimo-v2.5-pro`) stream chain-of-thought as inline `<think>…</think>` in the **`content`** channel rather than `reasoning_content`/`reasoning`. Left alone, Claude Code renders the raw tags as prose. When the catalog model carries `ThinkTagReasoning: true` (plumbed to the translator via `WithThinkTagReasoning`), [`think_tag.go`](think_tag.go)'s `thinkTagSplitter` reroutes a **leading** `<think>` block into an Anthropic thinking block; everything else passes through as text. Anchored to the start (after leading whitespace) — a mid-prose `<think>` mention stays text, mirroring `leadsWithToolishMarkup`. The splitter is streaming-safe: it buffers at most `len("</think>")-1` bytes (no whole-response buffering), so a tag split across SSE deltas is still caught. Off by default; only `xiaomi/mimo-v2.5-pro` enables it, and only on the OpenAI-compat chat-completions chain.


## `thought_signature` strip on Anthropic emit (load-bearing)

Inbound history can still carry Gemini-origin `thought_signature` fields (echoed by clients after a prior Gemini turn). Targeting Anthropic, `resolveAnthropicOverrides` strips the raw field from **all** blocks (`StripThoughtSignature`) — Anthropic 400s on unknown block fields. For tool calls the signature may also live in a smuggled id carrier ([`thought_signature_id.go`](thought_signature_id.go)); OpenAI emit paths clamp oversized ids under the 64-char `call_id`/`tool_calls[].id` limit (`clampOpenAIToolCallID`).


## Cross-format reasoning signatures on Anthropic `thinking` blocks (load-bearing)

The Responses→Anthropic writers smuggle an OpenAI reasoning item (`id` + `encrypted_content`) into the Anthropic `signature` field ([`openai_reasoning_signature.go`](openai_reasoning_signature.go)) so the reasoning can be replayed to OpenAI next turn. Anthropic validates that opaque field and answers `Invalid signature in thinking block`, so `resolveAnthropicOverrides` drops those blocks unconditionally (`StripForeignSignedThinkingBlocks`) alongside unsigned ones — not only when `ModelSwitched` is set. The switch guard is not sufficient: a client-side compaction rewrites the first user message, which re-keys the session, so the pin (and with it the prior served model) is gone on exactly the turn that re-routes an OpenAI-served history to Anthropic.

## Tool-call validation + strict decoding (load-bearing)

Model-emitted tool_use arguments are validated against the inbound request's `tools[].input_schema` by [`toolcheck`](toolcheck/) at every response-translation point (OpenAI-compat chains, streaming + non-streaming, and both Responses paths). The pipeline: normalize (drop empty-string/null OPTIONAL params) -> minimal JSON repair -> Draft-7 validation -> safe deterministic repair (drop unknown keys, lossless coercions), re-validated. Unrepairable schema mismatches forward as-emitted (the client's own tool error is the feedback loop); only unparseable JSON degrades to `{}`. Every finding surfaces via `ResponseSummary.ToolCallIssues`, which the proxy logs as `router.tool_call_invalid`. **Everything is fail-open** — a schema that won't compile must never fail a request.

On the emit side the failure class is prevented at decode time where the upstream exposes a knob: OpenAI Responses tools go out with `strict:true` + a strictified schema ([`strictify_openai.go`](strictify_openai.go) — additionalProperties:false, all-required, optionals as null unions; non-strictifiable schemas fall back to non-strict). Proxy-side validation always checks against the ORIGINAL schema — the explicit nulls strict mode induces are dropped by toolcheck's normalize pass.


## Invariants

- **No I/O.** No HTTP, no DB, no filesystem.
- **No domain types.** Don't import `auth`, `proxy`, or anything from `internal/router/*`.
- **No provider package imports.** Translation must be addressable without pulling in `internal/providers/<name>`.
