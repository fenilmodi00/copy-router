// Package catalog is the single source of truth for per-model data: tier,
// per-provider upstream IDs, per-provider pricing. Adding a model is one
// struct literal here. Every model carries an ordered Providers list; the
// first binding whose Provider is in the deploy's available set is chosen.
// Pure inner-ring, no I/O.
package catalog

import (
	"weave-os/router/internal/providers"
)

// ModelGPT6Luna is the canonical GPT-6 Luna model ID.
const ModelGPT6Luna = "gpt-6-luna"

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
	// LongContext applies alternate rates above a provider's prompt-size
	// threshold.
	LongContext *LongContextPricing
}

// LongContextPricing holds alternate rates for a provider's long-context tier.
type LongContextPricing struct {
	ThresholdTokens      int
	InputUSDPer1M        float64
	OutputUSDPer1M       float64
	CacheWriteMultiplier float64
	CacheReadMultiplier  float64
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

// ForInputTokens returns the applicable pricing tier for a provider-reported
// prompt token count.
func (p Pricing) ForInputTokens(inputTokens int) Pricing {
	if p.LongContext == nil || inputTokens <= p.LongContext.ThresholdTokens {
		return p
	}
	long := p.LongContext
	return Pricing{
		InputUSDPer1M:        long.InputUSDPer1M,
		OutputUSDPer1M:       long.OutputUSDPer1M,
		CacheWriteMultiplier: long.CacheWriteMultiplier,
		CacheReadMultiplier:  long.CacheReadMultiplier,
	}
}

// ProviderBinding is one (provider, upstream-model-ID, price) tuple for a
// logical model. Ordered list per Model — the scorer picks the first whose
// Provider name is wired in the running deploy.
type ProviderBinding struct {
	// Provider is one of the providers.Provider* constants.
	Provider string
	// UpstreamID is the model ID the upstream API expects. Empty means
	// "same as Model.ID" (no rewrite); non-empty is rewritten at proxy time.
	UpstreamID string
	// Price is the per-provider pricing for this binding.
	Price Pricing
	// FastPrice is the published per-token rate when the binding is dispatched
	// on the provider's fast tier (OpenAI service_tier=priority, Anthropic
	// speed=fast). Zero means the binding has no fast tier. A zero
	// CacheReadMultiplier inherits Price's. Routing always scores on Price;
	// only post-dispatch billing sees the fast rate.
	FastPrice Pricing
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

// ModelID is a catalog model identifier. Production lookup still uses string IDs
// because the table is data, but call sites that name a known catalog model
// should use these constants instead of repeating raw strings.
type ModelID string

const (
	ModelIDClaudeHaiku45  ModelID = "claude-haiku-4-5"
	ModelIDClaudeSonnet46 ModelID = "claude-sonnet-4-6"
	ModelIDClaudeOpus48   ModelID = "claude-opus-4-8"
	ModelIDGPT55          ModelID = "gpt-5.5"
)

func (id ModelID) String() string {
	return string(id)
}

// Model is one logical model — the unit the router decides on.
type Model struct {
	// ID is the public slash-form (or bare) model ID exposed to clients,
	// e.g. "claude-opus-4-7" or "deepseek/deepseek-v4-pro".
	ID string
	// Source is how the model's weights are published. Required on every row:
	// products sold as open-source-only read dispatch eligibility from it, and
	// an unset value is rejected by the catalog tests rather than defaulted.
	Source Source
	// Tier is the coarse capability bucket. TierUnknown excludes the model
	// from generic automatic routing; an HMM-only target may opt in separately.
	Tier Tier
	// HMMTarget allows an otherwise untiered catalog row to be offered to an HMM
	// policy sidecar without adding it to the generic cluster candidate set.
	HMMTarget bool
	// CodexSubscription marks models served by the native ChatGPT/Codex OAuth
	// backend. OpenAI API models leave this false even when they share the same
	// provider binding.
	CodexSubscription bool
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
	// CodexSubscriptionFallback explicitly permits trying a Codex subscription
	// for this model outside the native Codex automatic roster. Unsupported
	// targets must fall back to the selected model's API credential.
	CodexSubscriptionFallback bool
	// Providers is the ordered fallback list. First binding whose
	// Provider name is in the available set wins. Non-empty for every
	// dispatchable row; AIand-only-cut rows pending Phase 3 deletion carry
	// no bindings.
	Providers []ProviderBinding
}

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
	// --- Anthropic ---
	// $1/$5 per the published table (the $0.80/$4 rate that used to sit here
	// is Haiku 3.5's row — Haiku 4.5 never shipped at that price).
	{ID: "claude-haiku-4-5", Source: SourceClosedSource, Tier: TierLow, ContextWindow: 200_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderAnthropic, Price: Pricing{InputUSDPer1M: 1.00, OutputUSDPer1M: 5.00, CacheReadMultiplier: 0.10}},
	}},
	{ID: "claude-sonnet-4-5", Source: SourceClosedSource, Tier: TierMid, ContextWindow: 200_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderAnthropic, Price: Pricing{InputUSDPer1M: 3.00, OutputUSDPer1M: 15.00, CacheReadMultiplier: 0.10}},
	}},
	{ID: "claude-sonnet-4-6", Source: SourceClosedSource, Tier: TierMid, ContextWindow: 200_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderAnthropic, Price: Pricing{InputUSDPer1M: 3.00, OutputUSDPer1M: 15.00, CacheReadMultiplier: 0.10}},
	}},
	// 1M context is behind the context-1m beta (catalog carries 200K like the
	// rest of Sonnet). Priced at standard $3/$15, not the $2/$10 introductory
	// rate (through 2026-08-31) — avoids a compile-time price going stale.
	{ID: "claude-sonnet-5", Source: SourceClosedSource, Tier: TierMid, ContextWindow: 200_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderAnthropic, Price: Pricing{InputUSDPer1M: 3.00, OutputUSDPer1M: 15.00, CacheReadMultiplier: 0.10}},
	}},
	// Sonnet 5.5: $2/$10, cache reads at $0.20/MTok (0.1x), no fast tier.
	// Native 1M context; thinking cannot be disabled (between_tools is the
	// floor) and forced tool_choice is rejected, like Opus 5.5.
	{ID: "claude-sonnet-5-5", Source: SourceClosedSource, Tier: TierMid, ContextWindow: 1_000_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderAnthropic, Price: Pricing{InputUSDPer1M: 2.00, OutputUSDPer1M: 10.00, CacheReadMultiplier: 0.10}},
	}},
	// Legacy Opus IDs kept passthrough-priced (no Tier — not a routing
	// target; see gpt-4o below for the same pattern) so BYOK/direct-model
	// requests billing-debit at real cost instead of catalog.PrimaryPriceFor
	// silently returning $0. Prices per the opus-4-6 comment below: 4.1 and
	// earlier were $15/$75; 4.5 is the first $5/$25 release.
	{ID: "claude-opus-4-0", Source: SourceClosedSource, ContextWindow: 200_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderAnthropic, Price: Pricing{InputUSDPer1M: 15.00, OutputUSDPer1M: 75.00, CacheReadMultiplier: 0.10}},
	}},
	{ID: "claude-opus-4-1", Source: SourceClosedSource, ContextWindow: 200_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderAnthropic, Price: Pricing{InputUSDPer1M: 15.00, OutputUSDPer1M: 75.00, CacheReadMultiplier: 0.10}},
	}},
	{ID: "claude-opus-4-5", Source: SourceClosedSource, ContextWindow: 200_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderAnthropic, Price: Pricing{InputUSDPer1M: 5.00, OutputUSDPer1M: 25.00, CacheReadMultiplier: 0.10}},
	}},
	// Opus 4.5+ is $5/$25 per MTok (down from $15/$75 on 4.1 and earlier).
	// 4.6+/4.7+/4.8 support 1M context via the context-1m-2025-08-07 beta; the
	// catalog reports 200K and the pre-filter expands to 1M when the beta
	// header is present (contextWindowForRequest in proxy/service.go).
	{ID: "claude-opus-4-6", Source: SourceClosedSource, Tier: TierHigh, ContextWindow: 200_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderAnthropic, Price: Pricing{InputUSDPer1M: 5.00, OutputUSDPer1M: 25.00, CacheReadMultiplier: 0.10}},
	}},
	{ID: "claude-opus-4-7", Source: SourceClosedSource, Tier: TierHigh, ContextWindow: 200_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderAnthropic, Price: Pricing{InputUSDPer1M: 5.00, OutputUSDPer1M: 25.00, CacheReadMultiplier: 0.10}},
	}},
	// Opus 4.8 retired from routing; kept as priced passthrough so lingering BYOK/direct pins bill at real cost.
	{ID: "claude-opus-4-8", Source: SourceClosedSource, ContextWindow: 200_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderAnthropic, Price: Pricing{InputUSDPer1M: 5.00, OutputUSDPer1M: 25.00, CacheReadMultiplier: 0.10}, FastPrice: Pricing{InputUSDPer1M: 10.00, OutputUSDPer1M: 50.00}},
	}},
	// 1M context natively (no context-1m beta header), same $5/$25 as opus-4-8.
	{ID: "claude-opus-5", Source: SourceClosedSource, Tier: TierHigh, ContextWindow: 1_000_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderAnthropic, Price: Pricing{InputUSDPer1M: 5.00, OutputUSDPer1M: 25.00, CacheReadMultiplier: 0.10}, FastPrice: Pricing{InputUSDPer1M: 10.00, OutputUSDPer1M: 50.00}},
	}},
	// Opus 5.5: $4/$20, cache reads at $0.20/MTok (0.05x), fast mode $8/$40.
	// Native 1M context, adaptive thinking always on, forced tool_choice
	// rejected like Fable 5.1. Not a cluster roster member until it has
	// quality labels.
	{ID: "claude-opus-5-5", Source: SourceClosedSource, Tier: TierHigh, ContextWindow: 1_000_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderAnthropic, Price: Pricing{InputUSDPer1M: 4.00, OutputUSDPer1M: 20.00, CacheReadMultiplier: 0.05}, FastPrice: Pricing{InputUSDPer1M: 8.00, OutputUSDPer1M: 40.00}},
	}},
	// Fable 5 retired from routing; kept as priced passthrough so lingering
	// BYOK/direct pins and the compaction summarizer bill at real cost.
	// Safety classifiers can return stop_reason "refusal" (HTTP 200); see
	// mapStopReason in translate.
	{ID: "claude-fable-5", Source: SourceClosedSource, ContextWindow: 1_000_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderAnthropic, Price: Pricing{InputUSDPer1M: 10.00, OutputUSDPer1M: 50.00, CacheReadMultiplier: 0.10}},
	}},
	// Fable 5.1: same $10/$50 as Fable 5, cache reads at $0.25/MTok (0.025x).
	// Native 1M context, adaptive thinking always on, stop_reason "refusal"
	// like Fable 5. Not a cluster roster member until it has quality labels.
	{ID: "claude-fable-5-1", Source: SourceClosedSource, Tier: TierHigh, ContextWindow: 1_000_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderAnthropic, Price: Pricing{InputUSDPer1M: 10.00, OutputUSDPer1M: 50.00, CacheReadMultiplier: 0.025}},
	}},

	// --- OpenAI GPT-4.x (legacy) ---
	{ID: "gpt-4.1-nano", Source: SourceClosedSource, Tier: TierLow, ContextWindow: 1_047_576, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{InputUSDPer1M: 0.10, OutputUSDPer1M: 0.40, CacheReadMultiplier: 0.25}, FastPrice: Pricing{InputUSDPer1M: 0.20, OutputUSDPer1M: 0.80}},
	}},
	{ID: "gpt-4.1-mini", Source: SourceClosedSource, Tier: TierLow, ContextWindow: 1_047_576, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{InputUSDPer1M: 0.40, OutputUSDPer1M: 1.60, CacheReadMultiplier: 0.25}, FastPrice: Pricing{InputUSDPer1M: 0.70, OutputUSDPer1M: 2.80}},
	}},
	{ID: "gpt-4.1", Source: SourceClosedSource, Tier: TierMid, ContextWindow: 1_047_576, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{InputUSDPer1M: 2.00, OutputUSDPer1M: 8.00, CacheReadMultiplier: 0.25}, FastPrice: Pricing{InputUSDPer1M: 3.50, OutputUSDPer1M: 14.00}},
	}},
	// gpt-4o family: priced for passthrough, not a routing target.
	{ID: "gpt-4o-mini", Source: SourceClosedSource, ContextWindow: 128_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{InputUSDPer1M: 0.15, OutputUSDPer1M: 0.60, CacheReadMultiplier: 0.50}, FastPrice: Pricing{InputUSDPer1M: 0.25, OutputUSDPer1M: 1.00}},
	}},
	{ID: "gpt-4o", Source: SourceClosedSource, ContextWindow: 128_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{InputUSDPer1M: 2.50, OutputUSDPer1M: 10.00, CacheReadMultiplier: 0.50}, FastPrice: Pricing{InputUSDPer1M: 4.25, OutputUSDPer1M: 17.00}},
	}},

	// --- OpenAI GPT-5 ---
	{ID: "gpt-5-nano", Source: SourceClosedSource, ContextWindow: 400_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{InputUSDPer1M: 0.10, OutputUSDPer1M: 0.40, CacheReadMultiplier: 0.10}},
	}},
	{ID: "gpt-5-mini", Source: SourceClosedSource, ContextWindow: 400_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{InputUSDPer1M: 0.50, OutputUSDPer1M: 2.00, CacheReadMultiplier: 0.10}, FastPrice: Pricing{InputUSDPer1M: 0.45, OutputUSDPer1M: 3.60}},
	}},
	{ID: "gpt-5", Source: SourceClosedSource, Tier: TierHigh, ContextWindow: 400_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{InputUSDPer1M: 2.50, OutputUSDPer1M: 10.00, CacheReadMultiplier: 0.10}, FastPrice: Pricing{InputUSDPer1M: 2.50, OutputUSDPer1M: 20.00}},
	}},
	{ID: "gpt-5-chat", Source: SourceClosedSource, ContextWindow: 400_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{InputUSDPer1M: 2.50, OutputUSDPer1M: 10.00, CacheReadMultiplier: 0.10}},
	}},

	// --- OpenAI GPT-5.4 ---
	// gpt-5.4-nano serves a 400K window (OpenRouter listing + direct-OpenAI
	// capacity), not 1M — overstating lets the overflow pre-filter route a
	// >400K request onto a model that hard-400s upstream. gpt-5.4/-mini share
	// the family's 400K ceiling.
	{ID: "gpt-5.4-nano", Source: SourceClosedSource, Tier: TierMid, ContextWindow: 400_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{InputUSDPer1M: 0.20, OutputUSDPer1M: 1.25, CacheReadMultiplier: 0.10}},
	}},
	{ID: "gpt-5.4-mini", Source: SourceClosedSource, Tier: TierMid, ContextWindow: 400_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{InputUSDPer1M: 0.75, OutputUSDPer1M: 4.50, CacheReadMultiplier: 0.10}, FastPrice: Pricing{InputUSDPer1M: 1.50, OutputUSDPer1M: 9.00}},
	}},
	{ID: "gpt-5.4", Source: SourceClosedSource, Tier: TierHigh, ContextWindow: 1_000_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{InputUSDPer1M: 2.50, OutputUSDPer1M: 15.00, CacheReadMultiplier: 0.10}, FastPrice: Pricing{InputUSDPer1M: 5.00, OutputUSDPer1M: 30.00}},
	}},
	{ID: "gpt-5.4-pro", Source: SourceClosedSource, Tier: TierHigh, ContextWindow: 1_000_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{InputUSDPer1M: 30.00, OutputUSDPer1M: 180.00, CacheReadMultiplier: 1.0}},
	}},

	// --- OpenAI GPT-5.5 ---
	{ID: "gpt-5.5-nano", Source: SourceClosedSource, Tier: TierMid, ContextWindow: 1_000_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{InputUSDPer1M: 0.15, OutputUSDPer1M: 0.60, CacheReadMultiplier: 0.10}},
	}},
	{ID: "gpt-5.5-mini", Source: SourceClosedSource, Tier: TierMid, ContextWindow: 1_000_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{InputUSDPer1M: 0.50, OutputUSDPer1M: 2.50, CacheReadMultiplier: 0.10}},
	}},
	// gpt-5.5 retired from routing; kept as priced passthrough so lingering
	// session pins and direct requests still bill at real cost. The HMM rosters
	// dropped it, but an untiered row is what also removes it from cluster
	// candidates — leaving it tiered kept the legacy strategy selecting it.
	{ID: "gpt-5.5", Source: SourceClosedSource, ContextWindow: 1_050_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{InputUSDPer1M: 5.00, OutputUSDPer1M: 30.00, CacheReadMultiplier: 0.10}, FastPrice: Pricing{InputUSDPer1M: 12.50, OutputUSDPer1M: 75.00}},
	}},
	{ID: "gpt-5.5-pro", Source: SourceClosedSource, Tier: TierHigh, ContextWindow: 1_000_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{InputUSDPer1M: 30.00, OutputUSDPer1M: 180.00, CacheReadMultiplier: 1.0}},
	}},

	// --- OpenAI GPT-5.6 --- Sol/Terra/Luna family, GA 2026-07-09.
	{ID: "gpt-5.6-luna", Source: SourceClosedSource, Tier: TierMid, CodexSubscription: true, ContextWindow: 1_050_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{
			InputUSDPer1M: 0.20, OutputUSDPer1M: 1.20, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10,
			LongContext: &LongContextPricing{ThresholdTokens: 272_000, InputUSDPer1M: 0.40, OutputUSDPer1M: 1.80, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10},
		}, FastPrice: Pricing{
			InputUSDPer1M: 0.40, OutputUSDPer1M: 2.40, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10,
			LongContext: &LongContextPricing{ThresholdTokens: 272_000, InputUSDPer1M: 0.80, OutputUSDPer1M: 3.60, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10},
		}},
	}},
	// Pi roster aliases dispatch to their native OpenAI model IDs.
	{ID: "gpt-5.6-luna-pro", Source: SourceClosedSource, HMMTarget: true, ContextWindow: 1_050_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, UpstreamID: "gpt-5.6-luna", Price: Pricing{
			InputUSDPer1M: 0.20, OutputUSDPer1M: 1.20, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10,
			LongContext: &LongContextPricing{ThresholdTokens: 272_000, InputUSDPer1M: 0.40, OutputUSDPer1M: 1.80, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10},
		}, FastPrice: Pricing{
			InputUSDPer1M: 0.40, OutputUSDPer1M: 2.40, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10,
			LongContext: &LongContextPricing{ThresholdTokens: 272_000, InputUSDPer1M: 0.80, OutputUSDPer1M: 3.60, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10},
		}},
	}},
	{ID: "gpt-5.6-terra", Source: SourceClosedSource, Tier: TierHigh, CodexSubscription: true, ContextWindow: 1_050_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{
			InputUSDPer1M: 2.00, OutputUSDPer1M: 12.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10,
			LongContext: &LongContextPricing{ThresholdTokens: 272_000, InputUSDPer1M: 4.00, OutputUSDPer1M: 18.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10},
		}, FastPrice: Pricing{
			InputUSDPer1M: 4.00, OutputUSDPer1M: 24.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10,
			LongContext: &LongContextPricing{ThresholdTokens: 272_000, InputUSDPer1M: 8.00, OutputUSDPer1M: 36.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10},
		}},
	}},
	{ID: "gpt-5.6-sol", Source: SourceClosedSource, Tier: TierHigh, CodexSubscription: true, ContextWindow: 1_050_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{
			InputUSDPer1M: 4.00, OutputUSDPer1M: 20.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10,
			LongContext: &LongContextPricing{ThresholdTokens: 272_000, InputUSDPer1M: 8.00, OutputUSDPer1M: 30.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10},
		}, FastPrice: Pricing{
			InputUSDPer1M: 8.00, OutputUSDPer1M: 40.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10,
			LongContext: &LongContextPricing{ThresholdTokens: 272_000, InputUSDPer1M: 16.00, OutputUSDPer1M: 60.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10},
		}},
	}},
	{ID: "gpt-5.6-sol-pro", Source: SourceClosedSource, HMMTarget: true, ContextWindow: 1_050_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, UpstreamID: "gpt-5.6-sol", Price: Pricing{
			InputUSDPer1M: 4.00, OutputUSDPer1M: 20.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10,
			LongContext: &LongContextPricing{ThresholdTokens: 272_000, InputUSDPer1M: 8.00, OutputUSDPer1M: 30.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10},
		}, FastPrice: Pricing{
			InputUSDPer1M: 8.00, OutputUSDPer1M: 40.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10,
			LongContext: &LongContextPricing{ThresholdTokens: 272_000, InputUSDPer1M: 16.00, OutputUSDPer1M: 60.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10},
		}},
	}},
	{ID: "gpt-6-astra", Source: SourceClosedSource, Tier: TierHigh, ContextWindow: 1_050_000, CodexSubscriptionFallback: true, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{
			InputUSDPer1M: 10.00, OutputUSDPer1M: 50.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10,
			LongContext: &LongContextPricing{ThresholdTokens: 272_000, InputUSDPer1M: 20.00, OutputUSDPer1M: 75.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10},
		}, FastPrice: Pricing{
			InputUSDPer1M: 20.00, OutputUSDPer1M: 100.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10,
			LongContext: &LongContextPricing{ThresholdTokens: 272_000, InputUSDPer1M: 40.00, OutputUSDPer1M: 150.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10},
		}},
	}},
	// --- OpenAI GPT-6 Sol/Luna --- same >272K (2x in, 1.5x out) and 2x fast
	// schedule as Astra.
	{ID: "gpt-6-sol", Source: SourceClosedSource, Tier: TierHigh, CodexSubscription: true, ContextWindow: 1_050_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{
			InputUSDPer1M: 2.00, OutputUSDPer1M: 10.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10,
			LongContext: &LongContextPricing{ThresholdTokens: 272_000, InputUSDPer1M: 4.00, OutputUSDPer1M: 15.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10},
		}, FastPrice: Pricing{
			InputUSDPer1M: 4.00, OutputUSDPer1M: 20.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10,
			LongContext: &LongContextPricing{ThresholdTokens: 272_000, InputUSDPer1M: 8.00, OutputUSDPer1M: 30.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10},
		}},
	}},
	{ID: "gpt-6.1-sol", Source: SourceClosedSource, Tier: TierHigh, CodexSubscription: true, ContextWindow: 1_050_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{
			InputUSDPer1M: 2.00, OutputUSDPer1M: 10.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.05,
			LongContext: &LongContextPricing{ThresholdTokens: 272_000, InputUSDPer1M: 4.00, OutputUSDPer1M: 15.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.05},
		}, FastPrice: Pricing{
			InputUSDPer1M: 4.00, OutputUSDPer1M: 20.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.05,
			LongContext: &LongContextPricing{ThresholdTokens: 272_000, InputUSDPer1M: 8.00, OutputUSDPer1M: 30.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.05},
		}},
	}},
	{ID: ModelGPT6Luna, Source: SourceClosedSource, Tier: TierMid, CodexSubscription: true, ContextWindow: 1_050_000, Providers: []ProviderBinding{
		{Provider: providers.ProviderOpenAI, Price: Pricing{
			InputUSDPer1M: 0.10, OutputUSDPer1M: 0.50, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10,
			LongContext: &LongContextPricing{ThresholdTokens: 272_000, InputUSDPer1M: 0.20, OutputUSDPer1M: 0.75, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10},
		}, FastPrice: Pricing{
			InputUSDPer1M: 0.20, OutputUSDPer1M: 1.00, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10,
			LongContext: &LongContextPricing{ThresholdTokens: 272_000, InputUSDPer1M: 0.40, OutputUSDPer1M: 1.50, CacheWriteMultiplier: 1.25, CacheReadMultiplier: 0.10},
		}},
	}},

	// --- OSS pool ---
	//
	// Only kimi-k3 survives the AIand-only cut here; every other OSS row was
	// deleted with its provider registrations. First multimodal Kimi, so unlike
	// the retired k2.x rows it carries no ImageInputUnsupported.
	{ID: "moonshotai/kimi-k3", Source: SourceOpenSource, Tier: TierHigh, ContextWindow: 1_048_576, Providers: []ProviderBinding{
		// AIand-only product binding (user decision 2026-10-04): the router
		// serves kimi-k3 exclusively via AIand at its lower output rate.
		{Provider: providers.ProviderAIAND, Price: Pricing{InputUSDPer1M: 3.000, OutputUSDPer1M: 12.500, CacheReadMultiplier: 0.50 / 3.000}},
	}},
	// --- AIand (api.aiand.com) ---
	//
	// AIand serves this curated 8-model roster natively under slash-form IDs
	// (empty UpstreamID, like OpenRouter). Rates/context/caps from the live
	// GET /v1/models probe (2026-10-04). moonshotai/kimi-k3 is served via its
	// AIand-only binding in the moonshot section above (2026-10-04 roster
	// decision: dropped glm-5.2, kimi-k2.7-code, qwen3.6-27b, gemma-4, gpt-oss).
	{ID: "deepseek-ai/deepseek-v4-flash", Source: SourceOpenSource, Tier: TierLow, ContextWindow: 1_048_576, ImageInput: ImageInputUnsupported, Providers: []ProviderBinding{
		{Provider: providers.ProviderAIAND, Price: Pricing{InputUSDPer1M: 0.150, OutputUSDPer1M: 0.250, CacheReadMultiplier: 0.08 / 0.150}},
	}},
	{ID: "deepseek-ai/deepseek-v4.1-flash", Source: SourceOpenSource, Tier: TierMid, ContextWindow: 1_048_576, Providers: []ProviderBinding{
		{Provider: providers.ProviderAIAND, Price: Pricing{InputUSDPer1M: 0.300, OutputUSDPer1M: 0.600, CacheReadMultiplier: 0.02 / 0.300}},
	}},
	{ID: "deepseek-ai/deepseek-v4-pro", Source: SourceOpenSource, Tier: TierHigh, ContextWindow: 1_048_576, ImageInput: ImageInputUnsupported, Providers: []ProviderBinding{
		{Provider: providers.ProviderAIAND, Price: Pricing{InputUSDPer1M: 1.000, OutputUSDPer1M: 2.500, CacheReadMultiplier: 0.25}},
	}},
	{ID: "zai-org/glm-5.3", Source: SourceOpenSource, Tier: TierHigh, ContextWindow: 1_048_576, ImageInput: ImageInputUnsupported, Providers: []ProviderBinding{
		{Provider: providers.ProviderAIAND, Price: Pricing{InputUSDPer1M: 1.000, OutputUSDPer1M: 4.000, CacheReadMultiplier: 0.30}},
	}},
	{ID: "zai-org/glm-5.3-flash", Source: SourceOpenSource, Tier: TierLow, ContextWindow: 1_048_576, Providers: []ProviderBinding{
		{Provider: providers.ProviderAIAND, Price: Pricing{InputUSDPer1M: 0.150, OutputUSDPer1M: 0.500, CacheReadMultiplier: 0.03 / 0.150}},
	}},
	{ID: "qwen/qwen3.8-27b", Source: SourceOpenSource, Tier: TierMid, ContextWindow: 262_144, Providers: []ProviderBinding{
		{Provider: providers.ProviderAIAND, Price: Pricing{InputUSDPer1M: 0.400, OutputUSDPer1M: 3.000, CacheReadMultiplier: 0.20 / 0.400}},
	}},
	{ID: "motif-technologies/motif-3", Source: SourceOpenSource, Tier: TierMid, ContextWindow: 262_144, ImageInput: ImageInputUnsupported, Providers: []ProviderBinding{
		{Provider: providers.ProviderAIAND, Price: Pricing{InputUSDPer1M: 0.500, OutputUSDPer1M: 2.000, CacheReadMultiplier: 0.20 / 0.500}},
	}},
}
