# ai& (aiand.com) — Live model catalog

> Snapshot pulled from the live `GET /v1/models` endpoint
> (`https://api.aiand.com/v1/models`) on **2026-08-26**, using the
> `AIAND_API_KEY` from `.env`. This is the authoritative per-org model list —
> it supersedes the static catalog table in
> `docs/aiand-provider-research.md`. Raw payload preserved verbatim in
> `docs/aiand-model-catalog.json`.

> The list is **dynamic and per-org** — re-run the endpoint before adding
> catalog bindings. Any org gets only what it has access to.

## Models (9 current)

| Model ID                        | provider           | CTX  | Capabilities                                                   | reasoning_efforts | default | In/1M   | Out/1M   | Cached/1M |
| ------------------------------- | ------------------ | ---- | ------------------------------------------------------------ | ----------------- | ------- | ----- | ------ | --------- |
| `deepseek-ai/deepseek-v4-flash` | deepseek-ai        | 1M   | reasoning, tool_calling                                      | none/high/max     | none    | $0.15 | $0.25  | $0.08     |
| `openai/gpt-oss-120b`           | openai             | 131K | reasoning, tool_calling                                      | low/medium/high   | medium  | $0.15 | $0.60  | $0.08     |
| `google/gemma-4-31b-it`         | google             | 262K | reasoning, tool_calling, **vision**, **video**, document       | none/high         | none    | $0.20 | $0.50  | $0.05     |
| `qwen/qwen3.6-27b`              | qwen               | 262K | reasoning, tool_calling, **vision**, **video**, document       | none/high         | high    | $0.32 | $3.20  | $0.20     |
| `motif-technologies/motif-3`    | Motif-Technologies | 262K | chat, reasoning, tool_calling                                | low/medium/high   | medium  | $0.50 | $2.00  | $0.20     |
| `moonshotai/kimi-k2.7-code`     | moonshotai         | 262K | reasoning, tool_calling, **vision**, document                 | high              | high    | $0.75 | $3.50  | $0.20     |
| `deepseek-ai/deepseek-v4-pro`   | deepseek-ai        | 1M   | reasoning, tool_calling                                      | none/high/max     | none    | $1.00 | $2.50  | $0.25     |
| `zai-org/glm-5.2`               | zai-org            | 1M   | reasoning, tool_calling                                      | none/high/max     | max     | $1.00 | $4.00  | $0.30     |
| `moonshotai/kimi-k3`            | moonshotai         | 1M   | reasoning, tool_calling, **vision**, document                 | low/high/max      | max     | $3.00 | $12.50 | $0.50     |

All pricing USD (`currency: "usd"`), per-1M-token. `Cached/1M` = `cached_input_per_1m`
applies to prompt-cache hits.

### Notes

- **Model namespaces** are `<lab>/<model>` (e.g. `zai-org/glm-5.2`). Send the
**exact** `id` — ai&'s `model` param does case-insensitive exact-then-prefix
resolution, so a bare alias like `glm` is ambiguous across versions.
- `id` and `name` may differ in casing (`deepseek-ai/DeepSeek-V4-Pro` vs id
`deepseek-ai/deepseek-v4-pro`). Use `id` as the routing key.
- `reasoning_effort_default` gives each model's default; `reasoning_efforts`
lists valid values (unsupported → `400`).
- Capabilities vary (e.g. `motif-3` is the only non-vision current model).
- `description` may be null (GLM-5.2, kimi-k3) — don't rely on it.

## Changes vs the earlier static research doc

Compared against the catalog table in `docs/aiand-provider-research.md`
(which came from the marketing catalog page, not the API):

| Model                         | Status vs doc        | Note                                                                                        |
| ----------------------------- | -------------------- | ------------------------------------------------------------------------------------------- |
| `moonshotai/kimi-k3`          | **ADDED**            | 1M ctx, $3.00/$12.50 — new frontier model                                                   |
| `motif-technologies/motif-3`  | **ADDED**            | 262K, 314B MoE (13.2B active), $0.50/$2.00                                                  |
| `moonshotai/kimi-k2.6`        | **REMOVED**          | no longer in per-org list (deprecated)                                                      |
| `zai-org/glm-5.1`             | **REMOVED**          | no longer in per-org list (deprecated)                                                      |
| `qwen/qwen3.6-27b`            | price changed        | doc said Free/Free → API says **$0.32/$3.20** (cached $0.20)                                |
| `google/gemma-4-31b-it`       | capabilities changed | doc: reasoning, tool_calling → API: + vision, video, document (price unchanged $0.20/$0.50) |
| `deepseek-ai/deepseek-v4-pro` | price updated        | doc $1.00/$2.50 — confirmed, now cached $0.25                                               |
| other models                  | unchanged            | glm-5.2, kimi-k2.7-code, deepseek-v4-flash, gpt-oss-120b match                              |

> The research doc's GLM-5.2 price discrepancy (marketing $0.80 vs doc $1.00)
> is resolved: the **API returns $1.00 input / $4.00 output / $0.30 cached**.

## Deprecated / delisted

Not returned by `/v1/models` (still may route until fully removed — the API
docs' prefix-resolution 400 references `GET /v1/models` for valid names):

- `moonshotai/kimi-k2.6`
- `zai-org/glm-5.1`

## Router integration guidance

For the ai&-only "core balancing" router:

1. Add `providers.ProviderAiand` ("aiand") to `internal/providers/provider.go`.
2. Add `AiandBaseURL = "https://api.aiand.com/v1"` to
   `internal/providers/openaicompat/client.go` + `AIAND_API_KEY` env entry.
3. Register in `cmd/router/main.go` via the `openaicompat` client.
4. In `internal/router/catalog/`, add one `Model` per row above (ordered
   `Providers` list = ai& as first/only binding), tier + pricing keyed off
   the fields here. Use the **exact** `id`.
5. `reasoning_effort_default` → set the model's default effort in the binding
   if we forward one; otherwise let the model default apply.

Source of truth: `docs/aiand-model-catalog.json` (verbatim endpoint dump) +
this file. Refresh both whenever you reconfigure the router.