// Package catalog is the single source of truth for per-model data: tier,
// per-provider upstream IDs, per-provider pricing. Adding a model is one
// struct literal here. Every model carries an ordered Providers list; the
// first binding whose Provider is in the deploy's available set is chosen,
// letting SOC-2 direct-provider rows append an OpenRouter fallback. Pure
// inner-ring, no I/O.
package catalog

import (
	"workweave/router/internal/providers"
)

// Tier is the coarse capability bucket. Higher is stronger; integer
// ordering is load-bearing (planner compares freshTier > pinTier).
type Tier int

const (
	TierUnknown Tier = iota // Zero value; absent from table.
	TierLow
	TierMid
	TierHigh
)

// String returns a snake_case label for logs and OTel attrs.
func (t Tier) String() string {
	switch t {
	case TierLow:
		return "low"
	case TierMid:
		return "mid"
	case TierHigh:
		return "high"
	default:
		return "unknown"
	}
}

// Pricing holds the per-1M-token USD costs for a single (provider, model)
// binding.
type Pricing struct {
	InputUSDPer1M  float64
	OutputUSDPer1M float64
	// CacheWriteMultiplier is the cache-creation price relative to base input price.
	// Zero means unspecified — existing production pricing is preserved.
	CacheWriteMultiplier float64
	// CacheReadMultiplier is the cost of a cache hit relative to the base
	// input price (e.g. 0.10 for Anthropic, 0.50 for OpenAI). Zero means
	// "unspecified — use DefaultCacheReadMultiplier".
	CacheReadMultiplier float64
}

// DefaultCacheReadMultiplier is the fallback multiplier for bindings
// without published cache pricing. 0.5 is conservative: high enough to not
// treat unknown providers as free caching, low enough to not block switches.
const DefaultCacheReadMultiplier = 0.5

// DefaultCacheWriteMultiplier preserves the legacy production cache-creation
// calculation until provider/model/retention-specific data is verified.
const DefaultCacheWriteMultiplier = 1.25

// EffectiveCacheReadMultiplier returns CacheReadMultiplier if set, else
// DefaultCacheReadMultiplier.
func (p Pricing) EffectiveCacheReadMultiplier() float64 {
	if p.CacheReadMultiplier > 0 {
		return p.CacheReadMultiplier
	}
	return DefaultCacheReadMultiplier
}

// EffectiveCacheWriteMultiplier returns the binding-specific cache-write
// multiplier when known, otherwise the legacy production multiplier.
func (p Pricing) EffectiveCacheWriteMultiplier() float64 {
	if p.CacheWriteMultiplier > 0 {
		return p.CacheWriteMultiplier
	}
	return DefaultCacheWriteMultiplier
}

// ProviderBinding is one (provider, upstream-model-ID, price) tuple for a
// logical model. Ordered list per Model — the scorer picks the first whose
// Provider name is wired in the running deploy.
type ProviderBinding struct {
	// Provider is one of the providers.Provider* constants.
	Provider string
	// UpstreamID is the model ID the upstream API expects. Empty means
	// "same as Model.ID" (no rewrite). Non-empty is fed to the
	// openaicompat client's modelIDMap so the body's "model" field is
	// rewritten at proxy time (e.g. Bedrock's dot-form, Makora's
	// HuggingFace form).
	UpstreamID string
	// Price is the per-provider pricing for this binding.
	Price Pricing
	// ContextWindow overrides the model-level ContextWindow for this binding.
	// Zero means inherit. Use when the served window differs by provider.
	ContextWindow int
}

// ToolUseQuality marks a model's reliability under has_tools=true turns.
// ToolUseUnknown (zero value) = no concerns recorded; ToolUseLow flags models
// that hallucinate tool calls, emit malformed tool_use blocks, or loop on the
// same tool. The scorer excludes ToolUseLow models from argmax on tool-bearing
// requests, falling back to the unfiltered pool only if that would empty it.
type ToolUseQuality int

const (
	ToolUseUnknown ToolUseQuality = iota
	ToolUseLow
)

// AgenticUse marks whether a model can reliably drive an agentic harness (the
// multi-step skill/tool-orchestration loop in Claude Code, opencode, etc).
// Stricter than ToolUseQuality: a model can emit well-formed tool calls yet
// still fail to run the harness (e.g. minimax-m3 grepped the filesystem for a
// skill instead of invoking it). AgenticLow flags models the scorer drops
// from has_tools turns, so the price dial can demote Opus to a cheaper
// harness-capable model instead of stranding the turn on the cheapest one.
type AgenticUse int

const (
	AgenticUnknown AgenticUse = iota
	AgenticLow
)

// ImageInput marks whether a model accepts image content parts.
// ImageInputUnknown (zero value) = no restriction; first-party models default
// here since they're all multimodal. ImageInputUnsupported flags text-only
// models that 4xx on image parts (e.g. GLM-5.1). The scorer excludes the
// ImageInputUnsupported set from image-bearing requests.
type ImageInput int

const (
	ImageInputUnknown ImageInput = iota
	ImageInputUnsupported
)

// Model is one logical model — the unit the router decides on.
type Model struct {
	// ID is the public slash-form (or bare) model ID exposed to clients,
	// e.g. "claude-opus-4-7" or "deepseek-ai/deepseek-v4-pro".
	ID string
	// Tier is the coarse capability bucket. TierUnknown excludes the model
	// from generic automatic routing; an HMM-only target may opt in separately.
	Tier Tier
	// HMMTarget allows an otherwise untiered catalog row to be offered to an HMM
	// policy sidecar without adding it to the generic cluster candidate set.
	HMMTarget bool
	// ContextWindow is the model's total input+output token budget in tokens.
	// 0 means use catalog.DefaultContextWindow.
	ContextWindow int
	// ToolUseQuality: default ToolUseUnknown; set ToolUseLow to remove the
	// model from agentic argmax pools.
	ToolUseQuality ToolUseQuality
	// AgenticUse: default AgenticUnknown; set AgenticLow to keep the model
	// out of the price dial's agentic demotion ladder.
	AgenticUse AgenticUse
	// ImageInput: default ImageInputUnknown; set ImageInputUnsupported on
	// text-only models so the scorer keeps image-bearing turns off them.
	ImageInput ImageInput
	// ThinkTagReasoning marks a model that streams inline <think>...</think>
	// instead of reasoning_content; the Anthropic translator reroutes a
	// leading <think> block into Anthropic thinking. Default false.
	ThinkTagReasoning bool
	// ReasoningEfforts is the ordered (least→most expensive) set of
	// reasoning_effort values this model accepts on the ai& wire. Empty means
	// the model takes no effort parameter. The ai& vocabulary is
	// none/low/high/max; a few OpenAI-compat rows also list medium.
	ReasoningEfforts []string
	// Providers is the ordered fallback list. First binding whose
	// Provider name is in the available set wins. Must be non-empty.
	Providers []ProviderBinding
}

// Aiand effort vocabulary. Per-model ReasoningEfforts is a subset (or, for
// OpenAI-compat rows, may also include "medium").
const (
	EffortNone   = "none"
	EffortLow    = "low"
	EffortMedium = "medium"
	EffortHigh   = "high"
	EffortMax    = "max"
)

// PrimaryProvider returns the first binding's provider name. Callers that
// don't yet thread provider through (OTel emitter, billing debit hook)
// look up pricing by this.
func (m Model) PrimaryProvider() string {
	if len(m.Providers) == 0 {
		return ""
	}
	return m.Providers[0].Provider
}

// Models is the source of truth, one struct literal per model, grouped by
// family and tier. Tiered models with a registered provider are automatic
// routing targets. Strategy artifacts own selection membership: legacy cluster
// versions use their model_registry.json, while policy sidecars such as HMM
// intersect catalog targets with their own roster. This catalog controls
// pricing and dispatch for every strategy.
var Models = []Model{
	// aiand-only catalog for Build.io / v0.76 registry.
	// Non-aiand bindings removed; registry upstream IDs resolve via ByIDOrUpstream.
	// ReasoningEfforts mirrors live GET /v1/models reasoning_efforts (2026-08-25).
	{ID: "deepseek-ai/deepseek-v4-flash", Tier: TierLow, ContextWindow: 1_048_576, ImageInput: ImageInputUnsupported, AgenticUse: AgenticLow,
		ReasoningEfforts: []string{EffortNone, EffortHigh, EffortMax},
		Providers: []ProviderBinding{
			{Provider: providers.ProviderAiand, UpstreamID: "deepseek-ai/deepseek-v4-flash",
				Price: Pricing{InputUSDPer1M: 0.150, OutputUSDPer1M: 0.250, CacheReadMultiplier: 0.08 / 0.150}},
		}},
	{ID: "deepseek-ai/deepseek-v4-pro", Tier: TierMid, ContextWindow: 1_048_576, ImageInput: ImageInputUnsupported,
		ReasoningEfforts: []string{EffortNone, EffortHigh, EffortMax},
		Providers: []ProviderBinding{
			{Provider: providers.ProviderAiand, UpstreamID: "deepseek-ai/deepseek-v4-pro",
				Price: Pricing{InputUSDPer1M: 1.000, OutputUSDPer1M: 2.500, CacheReadMultiplier: 0.25 / 1.000}},
		}},
	{ID: "moonshotai/kimi-k2.7", Tier: TierHigh, ContextWindow: 262_144, ImageInput: ImageInputUnsupported,
		ReasoningEfforts: []string{EffortHigh},
		Providers: []ProviderBinding{
			{Provider: providers.ProviderAiand, UpstreamID: "moonshotai/kimi-k2.7-code",
				Price: Pricing{InputUSDPer1M: 0.750, OutputUSDPer1M: 3.500, CacheReadMultiplier: 0.20 / 0.750}},
		}},
	{ID: "moonshotai/kimi-k3", Tier: TierHigh, ContextWindow: 1_048_576,
		ReasoningEfforts: []string{EffortLow, EffortHigh, EffortMax},
		Providers: []ProviderBinding{
			{Provider: providers.ProviderAiand, UpstreamID: "moonshotai/kimi-k3",
				Price: Pricing{InputUSDPer1M: 3.000, OutputUSDPer1M: 12.500, CacheReadMultiplier: 0.50 / 3.000}},
		}},
	{ID: "openai/gpt-oss-120b", Tier: TierLow, ContextWindow: 131_072,
		ReasoningEfforts: []string{EffortLow, EffortMedium, EffortHigh},
		Providers: []ProviderBinding{
			{Provider: providers.ProviderAiand, UpstreamID: "openai/gpt-oss-120b",
				Price: Pricing{InputUSDPer1M: 0.150, OutputUSDPer1M: 0.600, CacheReadMultiplier: 0.08 / 0.150}},
		}},
	{ID: "qwen/qwen3.6-27b", Tier: TierLow, ContextWindow: 262_144,
		ReasoningEfforts: []string{EffortNone, EffortHigh},
		Providers: []ProviderBinding{
			{Provider: providers.ProviderAiand, UpstreamID: "qwen/qwen3.6-27b",
				Price: Pricing{InputUSDPer1M: 0.320, OutputUSDPer1M: 3.200, CacheReadMultiplier: 0.20 / 0.320}},
		}},
	{ID: "google/gemma-4-31b-it", Tier: TierLow, ContextWindow: 262_144,
		ReasoningEfforts: []string{EffortNone, EffortHigh},
		Providers: []ProviderBinding{
			{Provider: providers.ProviderAiand, UpstreamID: "google/gemma-4-31b-it",
				Price: Pricing{InputUSDPer1M: 0.200, OutputUSDPer1M: 0.500, CacheReadMultiplier: 0.05 / 0.200}},
		}},
	{ID: "motif-technologies/motif-3", Tier: TierMid, ContextWindow: 262_144,
		ReasoningEfforts: []string{EffortLow, EffortMedium, EffortHigh},
		Providers: []ProviderBinding{
			{Provider: providers.ProviderAiand, UpstreamID: "motif-technologies/motif-3",
				Price: Pricing{InputUSDPer1M: 0.500, OutputUSDPer1M: 2.000, CacheReadMultiplier: 0.20 / 0.500}},
		}},
	// GLM-5.2: canonical id is zai-org/glm-5.2, ai&'s served model name. The
	// legacy z-ai/glm-5.2 string (frozen v0.69–v0.74 + candidate-k12 training
	// artifacts, stored session pins, archived client integrations) stays
	// resolvable via the aliases table in lookup.go. UpstreamID equals the
	// catalog ID, so indexUpstreamID skips this binding (no rewrite needed;
	// the catalog id is the wire id).
	{ID: "zai-org/glm-5.2", Tier: TierHigh, ContextWindow: 1_048_576, ImageInput: ImageInputUnsupported,
		ReasoningEfforts: []string{EffortNone, EffortHigh, EffortMax},
		Providers: []ProviderBinding{
			{Provider: providers.ProviderAiand, UpstreamID: "zai-org/glm-5.2",
				Price: Pricing{InputUSDPer1M: 1.000, OutputUSDPer1M: 4.000, CacheReadMultiplier: 0.30 / 1.000}},
		}},
}

// aliases maps retired catalog IDs onto their current canonical rows. The
// canonical ID is the one the live cluster scorer and /v1/router/models
// surface; an alias entry keeps an older name resolvable so frozen training
// artifacts (whose model_registry.json ranks the legacy name), stored
// session pins, and archived client integrations keep landing on the same
// Model row. Aliases are dispatch-invisible by construction: byID holds the
// legacy→canonical index, but byUpstreamID never gains the alias, so
// AvailableBindings/EnumerateBindings never hand dispatch a legacy upstream
// wire name. Add aliases here only when a model is renamed and the old name
// still appears in committed frozen artifacts or live client state.
var aliases = map[string]string{
	"z-ai/glm-5.2": "zai-org/glm-5.2",
}
