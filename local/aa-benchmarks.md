# AA Benchmarks — 6-Model Roster Research

Retrieved 2026-08-29. Primary source: artificialanalysis.ai (Intelligence Index v4.1.1: GDPval-AA v2, τ³-Banking, Terminal-Bench v2.1, SciCode, HLE, GPQA Diamond, CritPt, AA-Omniscience, AA-LCR).

AA rescaled the index between June and Aug 2026 (launch posts: GLM-5.2=51, Kimi K3=57; current pages: GLM-5.2=53, Kimi K3=60). All six models HAVE direct AA entries — including Motif-3 and Qwen3.8-27B.

## Summary Table

| Model (router ID) | AA Index | Terminal-Bench | DeepSWE v1.1 | SWE-bench Verified | Price (in/out per 1M) | Confidence |
|---|---|---|---|---|---|---|
| zai-org/glm-5.3 | 60 (max) | 28.3 (TB 3.0, vendor) / 88.2 (TB 2.1, provider) | 66.9 (vendor) | ~97% (vals.ai run) | flagship tier [INFERENCE] | Direct AA; TB/DeepSWE vendor |
| qwen/qwen3.8-27b | 52 (xhigh) | 73.0 (TB 2.1, vendor) | 42.2 (vendor) | 90% (vals.ai run) | Apache 2.0, self-hostable | Direct AA |
| moonshotai/kimi-k3 | 60 (max) | 88.3 (TB 2.1, vendor) / 84% (TB v2, AA) | 67.5 (vendor) / 64 (AA Coding Index) | 93.4% (vals.ai) / 76.8% (vendor) | $3.00/$15.00 — expensive | Direct AA |
| motif-technologies/motif-3 | 47 | not published | not published | not published | open weights, $0 listed | Direct AA |
| moonshotai/kimi-k2.7-code | 43 | no completed runs (vals.ai 0%) | not published | not published | $0.95/$4.00 | Direct AA |
| deepseek-ai/deepseek-v4-flash | 52 (Flash 0731, max) | 82.7 (TB 2.1, vendor) | 54.4 (vendor) | 91% (vals.ai 0731) / 79.0% (llm-stats) | $0.14/$0.28 ($0.003 cache-hit) | Direct AA |

Reference — GLM-5.2 (max) (being replaced): AA Index 53 (current page; 51 at June-2026 launch), GDPval-AA v2 1524, TB v2.1 ~78–81, 43k output tokens/AA task, $1.40/$4.40 per 1M, ~$0.19/task. Source: https://artificialanalysis.ai/models/glm-5-2 (retrieved 2026-08-29).

## Per-Model Detail

### zai-org/glm-5.3
- AA Index: 60 (max) — direct; on par with Kimi K3; +7 over GLM-5.2. 170M tokens on AA eval (very verbose); 59.3 t/s. Source: https://artificialanalysis.ai/models/glm-5-3 (2026-08-29); corroborated by AA changelog (https://artificialanalysis.ai/changelog).
- Terminal-Bench 3.0: 28.3 (from GLM-5.2's 4.6) — vendor (Z.AI docs). TB 2.1: 88.2 (provider-run via benchlm.ai; best verified TB 2.1 row there).
- DeepSWE v1.1: 66.9 (from GLM-5.2's 46.2) — vendor. Agents' Last Exam: 28.5 (from 23.8) — vendor.
- SWE-bench Verified: ~97% (vals.ai leaderboard run).
- Price/quality: verbose, slow-ish; GLM-5.3-Flash sibling scores 57 at $0.15/$0.50 if cost matters.
- Sources: https://docs.z.ai/guides/llm/glm-5.3 ; https://benchlm.ai/models/deepseek-v4-flash-0731 ; https://vals.ai/benchmarks/swebench

### qwen/qwen3.8-27b
- AA Index: 52 (xhigh) — direct (added 17 Aug 2026 per AA changelog; the expected 'no AA entry' is outdated). 160M tokens on eval (very verbose); 52.2 t/s. AA Agentic Index: 51. Source: https://artificialanalysis.ai/models/qwen3-8-27b (2026-08-29).
- TB 2.1: 73.0 (from Qwen3.6's 63.4) — vendor card. No TB 3.0 figure. DeepSWE v1.1: 42.2 (from 13.3) — vendor. SWE-bench Pro: 61.7; SWE-bench Verified: 90% (vals.ai).
- 27.78B dense, Apache 2.0, 262K context, self-hostable — frontier-class at open-weights price.
- Sources: https://kingy.ai/blog/qwen3-8-27b-specs-benchmarks-local-hardware ; https://www.orcarouter.ai/blog/qwen-3-8-27b ; https://venturebeat.com/technology/qwen3-8-27b-runs-frontier-class-coding-agents-and-reasoning-locally-no-cloud-api-required

### moonshotai/kimi-k3
- AA Index: 60 (max) — direct. 2.8T/104B active MoE; 37.2 t/s; $3/$15 per 1M ($2425 AA eval cost); 1M context; highest-ranked open weights model. (Launch coverage reported 57 pre-rescale.) Source: https://artificialanalysis.ai/models/kimi-k3 (2026-08-29).
- AA Coding Agent Index: 57 (#5) — direct: TB v2 84%, DeepSWE 64%, SWE-Atlas-QnA 23%; $3.18/task.
- Vendor tables: TB 2.1 88.3, DeepSWE 67.5, FrontierSWE 81.2, SWE Marathon 42.0, SWE-bench Verified 76.8% (vendor) vs 93.4% (vals.ai independent — prefer). GDPval v2 Elo 1668 (vs GLM-5.2 1514).
- Price/quality: good agentic value per task ($3.18) but premium token pricing.
- Sources: https://x.com/ArtificialAnlys/status/2078230240766345330 ; https://wan27.org/blog/kimi-k3-benchmarks ; https://vals.ai/benchmarks/swebench

### motif-technologies/motif-3
- AA Index: 47 — direct (final Aug 2026 release HAS an AA entry). 314B/13.2B active MoE, open weights, 256K context, reasoning model; very verbose (260M tokens). Speed/price N/A on AA page. Source: https://artificialanalysis.ai/models/motif-3 (2026-08-29).
- Predecessor context: Motif 3 Beta AA 45 (estimated, Jul 2026); HF model card 44; Motif-2-12.7B-Reasoning AA 45.
- Terminal-Bench / DeepSWE / SWE-bench Verified: NOT PUBLISHED for Motif 3 — explicit no-entry markers.
- Price/quality: budget/open slot, well below the other five roster models on intelligence.
- Sources: https://artificialanalysis.ai/models/motif-0714 ; https://huggingface.co/Motif-Technologies/Motif-3-Beta ; https://www.orcarouter.ai/blog/motif-3

### moonshotai/kimi-k2.7-code
- AA Index: 43 — direct. 49.3 t/s; fairly concise (100M tokens); $0.95/$4.00; $544.58 AA eval cost; Modified MIT; 10 providers. Source: https://artificialanalysis.ai/models/kimi-k2-7-code (2026-08-29).
- Terminal-Bench / DeepSWE / SWE-bench Verified: NO published scores — vals.ai tracks rows but no completed runs. Vendor launch reported coding suites instead: Kimi Code Bench v2 62.0 (vs K2.6 50.9), MCP Mark Verified 81.1, Program Bench 53.6, MCP Atlas 76.0 — all vendor-run.
- Price/quality: cheaper and more concise than K3; purpose-built coding model; ~30% fewer reasoning tokens than K2.6.
- Sources: https://www.marktechpost.com/2026/06/12/moonshot-ai-releases-kimi-k2-7-code-a-coding-model-reporting-21-8-on-kimi-code-bench-v2-over-k2-6 ; https://www.vals.ai/models/kimi_kimi-k2.7-code

### deepseek-ai/deepseek-v4-flash
- AA Index: 52 — direct, current DeepSeek V4 Flash 0731 (Reasoning, Max Effort) checkpoint (endpoint silently upgraded 31 Jul 2026, same name/key). 128 t/s; very verbose (210M tokens); $0.14/$0.28, cache-hit $0.003 (−98%). Source: https://artificialanalysis.ai/models/deepseek-v4-flash (2026-08-29). Predecessors: original V4 Flash AA 47; Flash Vision 51.
- TB 2.1: 82.7 (vendor; above GLM-5.2's 81.0, below Opus 4.8's 85.0). No TB 3.0. DeepSWE: 54.4 (vendor). SWE-bench Verified: 91% (vals.ai 0731) / 79.0% (llm-stats Flash-Max) / 73.7% (tech report). Also: Toolathlon-Verified 70.3, Cybergym 76.7, NL2Repo 54.2, ALE 25.2 (vendor).
- Price/quality: best cost-per-task in the roster; on AA Pareto frontier; ~60% cheaper per task than GPT-5.6 Luna at comparable intelligence.
- Sources: https://huggingface.co/deepseek-ai/DeepSeek-V4-Flash-0731 ; https://x.com/ArtificialAnlys/status/2083123180869496865 ; https://www.morphllm.com/deepseek-v4 ; https://www.techtimes.com/articles/322513/20260731/deepseek-retrained-v4-flash-beats-its-flagship-pro-nine-agent-benchmarks.htm

## GLM-5.2 → GLM-5.3 Replacement Delta
AA Index 53→60 (+7); TB 3.0 4.6→28.3; DeepSWE v1.1 46.2→66.9; Agents' Last Exam 23.8→28.5; GDPval-AA v2 1524 (GLM-5.3 not yet captured there). Both 744B/40B MoE; GLM-5.3 keeps SAO with compaction.

## Caveats for bundle quality factors
1. AA rescaled the Intelligence Index Jun–Aug 2026; use current-page values (done above).
2. TB 3.0 exists only for GLM-5.3 (vendor); ecosystem mostly reports TB 2.1 — don't mix scales.
3. DeepSWE v1.1 figures for DeepSeek/Qwen/GLM are vendor-run; AA Coding Agent Index runs (Kimi K3: DeepSWE 64) are the independent check where available.
4. SWE-bench Verified is saturated (top-5 span ~4 pts); prefer vals.ai independent runs.
5. Verbose models (Qwen3.8 160M tokens, GLM-5.3 170M, Motif-3 260M, DS V4 Flash 210M) inflate real cost-per-task vs headline token prices — use AA Cost-per-Task in quality factors.
