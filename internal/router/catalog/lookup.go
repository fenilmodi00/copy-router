package catalog

import (
	"fmt"
	"sort"
	"strings"

	"workweave/router/internal/router"
)

var byID map[string]int
var byUpstreamID map[string]int

func init() {
	byID = make(map[string]int, len(Models))
	byUpstreamID = make(map[string]int)
	for i := range Models {
		m := Models[i]
		byID[m.ID] = i
		for _, b := range m.Providers {
			if b.UpstreamID == "" || b.UpstreamID == m.ID {
				continue
			}
			indexUpstreamID(b.UpstreamID, i)
		}
	}
	for alias, canonical := range aliases {
		i, ok := byID[canonical]
		if !ok {
			continue
		}
		if _, exists := byID[alias]; !exists {
			byID[alias] = i
		}
	}
}

func indexUpstreamID(upstreamID string, i int) {
	if _, exists := byUpstreamID[upstreamID]; !exists {
		byUpstreamID[upstreamID] = i
	}
	if lower := strings.ToLower(upstreamID); lower != upstreamID {
		if _, exists := byUpstreamID[lower]; !exists {
			byUpstreamID[lower] = i
		}
	}
}

func modelIndex(id string) (int, bool) {
	if i, ok := byID[id]; ok {
		return i, true
	}
	if base := router.StripDateSuffix(id); base != id {
		if i, ok := byID[base]; ok {
			return i, true
		}
	}
	if i, ok := byUpstreamID[id]; ok {
		return i, true
	}
	if lower := strings.ToLower(id); lower != id {
		if i, ok := byUpstreamID[lower]; ok {
			return i, true
		}
	}
	return 0, false
}

// ByID returns the model with the given ID. If the exact ID isn't found,
// retries after stripping a trailing Anthropic (-20251001) or OpenAI (-2024-08-06) date suffix.
func ByID(id string) (Model, bool) {
	if i, ok := byID[id]; ok {
		return Models[i], true
	}
	if base := router.StripDateSuffix(id); base != id {
		if i, ok := byID[base]; ok {
			return Models[i], true
		}
	}
	return Model{}, false
}

// ByIDOrUpstream returns the model for a catalog ID or any binding UpstreamID
// (e.g. "deepseek-ai/deepseek-v4-flash" → deepseek-ai/deepseek-v4-flash). Catalog
// IDs win first so a catalog ID that equals some other model's UpstreamID is
// never rerouted. Upstream bindings themselves are unchanged — this only
// resolves names clients see on /v1/router/models back to the catalog row.
func ByIDOrUpstream(id string) (Model, bool) {
	if i, ok := modelIndex(id); ok {
		return Models[i], true
	}
	return Model{}, false
}

func resolveModelForWindow(id string) (Model, bool) {
	return ByIDOrUpstream(id)
}

// ResolveBinding returns the first ProviderBinding whose Provider is in
// `available`. Accepts catalog IDs or binding UpstreamIDs so registry fields
// resolve without renaming rows.
func ResolveBinding(id string, available map[string]struct{}) (ProviderBinding, bool) {
	i, ok := modelIndex(id)
	if !ok {
		return ProviderBinding{}, false
	}
	for _, b := range Models[i].Providers {
		if _, ok := available[b.Provider]; ok {
			return b, true
		}
	}
	return ProviderBinding{}, false
}

// AvailableBindings returns every ProviderBinding for the model whose Provider
// is in `available`, in catalog order. Used by the proxy's failover loop:
// index 0 is primary, indexes >0 are ordered fallbacks.
func AvailableBindings(id string, available map[string]struct{}) []ProviderBinding {
	indexed := EnumerateBindings(id, available)
	out := make([]ProviderBinding, 0, len(indexed))
	for _, binding := range indexed {
		out = append(out, binding.ProviderBinding)
	}
	return out
}

// IndexedBinding identifies a provider binding by its stable catalog order.
type IndexedBinding struct {
	Index int
	ProviderBinding
}

// EnumerateBindings returns every enabled binding in stable catalog order.
func EnumerateBindings(id string, available map[string]struct{}) []IndexedBinding {
	m, ok := ByID(id)
	if !ok {
		return nil
	}
	out := make([]IndexedBinding, 0, len(m.Providers))
	for index, b := range m.Providers {
		if _, ok := available[b.Provider]; ok {
			out = append(out, IndexedBinding{Index: index, ProviderBinding: b})
		}
	}
	return out
}

// RoutingTargetSet returns the automatic routing targets that have at least
// one binding backed by a provider registered in this deployment. Untiered
// catalog rows are passthrough-only and must never become policy candidates.
func RoutingTargetSet(availableProviders map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(Models))
	for _, model := range Models {
		if model.Tier == TierUnknown {
			continue
		}
		if len(EnumerateBindings(model.ID, availableProviders)) == 0 {
			continue
		}
		out[model.ID] = struct{}{}
	}
	return out
}

// HMMRoutingTargetSet returns the HMM policy candidate universe. It includes
// generic routing targets plus catalog rows explicitly reserved for HMM policy
// sidecars, without making those rows eligible for generic cluster routing.
func HMMRoutingTargetSet(availableProviders map[string]struct{}) map[string]struct{} {
	out := RoutingTargetSet(availableProviders)
	for _, model := range Models {
		if !model.HMMTarget || len(EnumerateBindings(model.ID, availableProviders)) == 0 {
			continue
		}
		out[model.ID] = struct{}{}
	}
	return out
}

// UpstreamIDFor returns the upstream model ID for a catalog binding.
func UpstreamIDFor(catalogID, bindingID string) string {
	if bindingID != "" {
		return bindingID
	}
	return catalogID
}

// PriceFor returns the per-(provider, model) pricing.
func PriceFor(provider, id string) (Pricing, bool) {
	m, ok := ByIDOrUpstream(id)
	if !ok {
		return Pricing{}, false
	}
	for _, b := range m.Providers {
		if b.Provider == provider {
			return b.Price, true
		}
	}
	return Pricing{}, false
}

// PrimaryPriceFor returns the pricing of the model's first (primary) binding,
// for call sites that don't thread a specific provider through.
func PrimaryPriceFor(id string) (Pricing, bool) {
	m, ok := ByIDOrUpstream(id)
	if !ok {
		return Pricing{}, false
	}
	if len(m.Providers) == 0 {
		return Pricing{}, false
	}
	return m.Providers[0].Price, true
}

// TierFor returns the tier of the model, or TierUnknown if absent.
func TierFor(id string) Tier {
	m, ok := ByID(id)
	if !ok {
		return TierUnknown
	}
	return m.Tier
}

// ThinkTagReasoningFor reports whether the model streams chain-of-thought as
// inline <think>…</think> in content (the Anthropic translator reroutes it
// into thinking). Unknown models return false.
func ThinkTagReasoningFor(id string) bool {
	m, ok := ByID(id)
	if !ok {
		return false
	}
	return m.ThinkTagReasoning
}

// CapabilitiesFor returns the wire-format ModelSpec for a catalog model.
// When ReasoningEfforts is set, the spec advertises CapReasoning with those
// levels so emit/force-effort clamp against the live ai& menu. Otherwise falls
// back to router.Lookup (Anthropic/OpenAI/Gemini registry entries).
func CapabilitiesFor(id string) router.ModelSpec {
	if m, ok := ByIDOrUpstream(id); ok && len(m.ReasoningEfforts) > 0 {
		levels := append([]string(nil), m.ReasoningEfforts...)
		return router.NewSpecWithReasoning(router.ReasoningCapabilities{Levels: levels}, router.CapReasoning)
	}
	return router.Lookup(id)
}

// ReasoningEffortsFor returns a copy of the model's accepted effort levels, or
// nil when the model takes no effort parameter / is unknown.
func ReasoningEffortsFor(id string) []string {
	m, ok := ByIDOrUpstream(id)
	if !ok || len(m.ReasoningEfforts) == 0 {
		return nil
	}
	return append([]string(nil), m.ReasoningEfforts...)
}

// IsAtOrBelow reports whether the model has a known tier at or below the
// ceiling. Unknown-tier models return false.
func IsAtOrBelow(id string, ceiling Tier) bool {
	t := TierFor(id)
	if t == TierUnknown {
		return false
	}
	return t <= ceiling
}

// AllowedAtOrBelow returns the set of known model IDs whose tier is at or
// below the ceiling.
func AllowedAtOrBelow(ceiling Tier) map[string]struct{} {
	out := make(map[string]struct{}, len(Models))
	for _, m := range Models {
		if m.Tier != TierUnknown && m.Tier <= ceiling {
			out[m.ID] = struct{}{}
		}
	}
	return out
}

// DefaultContextWindow is the fallback context window in tokens for models
// with no ContextWindow set in the catalog.
const DefaultContextWindow = 128_000

// ContextWindowFor returns the context window in tokens for the given model.
// Returns DefaultContextWindow when the model is absent or has no ContextWindow set.
// Accepts either a catalog ID (e.g. "zai-org/glm-5.2") or an upstream API ID stored
// as a registry model field (e.g. "zai-org/glm-5.2"), so the context-window
// scoring term sees the true window regardless of which ID a strategy artifact
// recorded.
func ContextWindowFor(id string) int {
	m, ok := resolveModelForWindow(id)
	if !ok || m.ContextWindow <= 0 {
		return DefaultContextWindow
	}
	return m.ContextWindow
}

// ContextWindowForBinding returns the context window for modelID on provider,
// preferring a non-zero ProviderBinding.ContextWindow over the model-level value.
// Returns DefaultContextWindow for an unknown model. Accepts catalog IDs or
// upstream API IDs (resolved via resolveModelForWindow).
func ContextWindowForBinding(modelID, provider string) int {
	m, ok := resolveModelForWindow(modelID)
	if !ok {
		return DefaultContextWindow
	}
	cw := m.ContextWindow
	if cw <= 0 {
		cw = DefaultContextWindow
	}
	for _, b := range m.Providers {
		if b.Provider == provider && b.ContextWindow > 0 {
			return b.ContextWindow
		}
	}
	return cw
}

// ToolUseLowSet returns model IDs with ToolUseLow quality. The cluster scorer
// excludes these when req.HasTools, falling back to the unfiltered pool if
// that would empty the eligible set. Each set entry's aliases (catalog.aliases)
// are folded in too, so a frozen training artifact whose model field carries
// a legacy alias name (e.g. z-ai/glm-5.2 instead of the canonical zai-org/glm-5.2)
// still matches the exclusion set the scorer compares against.
func ToolUseLowSet() map[string]struct{} {
	out := make(map[string]struct{}, len(Models))
	for _, m := range Models {
		if m.ToolUseQuality == ToolUseLow {
			out[m.ID] = struct{}{}
		}
	}
	foldAliasesInto(out)
	return out
}

// AgenticLowSet returns model IDs whose AgenticUse is AgenticLow — models that
// emit valid tool calls but can't sustain an agentic harness loop. Excluded
// alongside ToolUseLowSet so a cost demotion lands on a harness-capable model,
// not just the cheapest one. Mirrors ToolUseLowSet's fallback behavior.
func AgenticLowSet() map[string]struct{} {
	out := make(map[string]struct{}, len(Models))
	for _, m := range Models {
		if m.AgenticUse == AgenticLow {
			out[m.ID] = struct{}{}
		}
	}
	foldAliasesInto(out)
	return out
}

// ImageUnsupportedSet returns model IDs flagged ImageInputUnsupported,
// excluded when the request carries image content. Mirrors ToolUseLowSet's
// fallback behavior.
func ImageUnsupportedSet() map[string]struct{} {
	out := make(map[string]struct{}, len(Models))
	for _, m := range Models {
		if m.ImageInput == ImageInputUnsupported {
			out[m.ID] = struct{}{}
		}
	}
	foldAliasesInto(out)
	return out
}

// foldAliasesInto adds each alias in catalog.aliases whose canonical id is
// already a key in out, so derived exclusion sets (ToolUseLowSet / AgenticLowSet
// / ImageUnsupportedSet) match legacy-named frozen-artifact candidates too.
func foldAliasesInto(out map[string]struct{}) {
	for alias, canonical := range aliases {
		if _, ok := out[canonical]; !ok {
			continue
		}
		if _, exists := out[alias]; !exists {
			out[alias] = struct{}{}
		}
	}
}

// AcceptsImages reports whether the model accepts image content. Unknown
// models default to true; only an explicit ImageInputUnsupported flag
// returns false.
func AcceptsImages(id string) bool {
	m, ok := ByID(id)
	if !ok {
		return true
	}
	return m.ImageInput != ImageInputUnsupported
}

// AllPrimaryPricing returns primary-binding pricing for every known model,
// keyed by model ID.
func AllPrimaryPricing() map[string]Pricing {
	out := make(map[string]Pricing, len(Models))
	for _, m := range Models {
		if len(m.Providers) == 0 {
			continue
		}
		out[m.ID] = m.Providers[0].Price
	}
	return out
}

// ValidateDeployed returns an error naming any deployed model missing from
// the catalog or lacking a tier. Accepts catalog IDs or binding UpstreamIDs
// (via ByIDOrUpstream) so registry upstream IDs validate without renaming.
func ValidateDeployed(deployed []string) error {
	var missing []string
	for _, id := range deployed {
		m, ok := ByIDOrUpstream(id)
		if !ok {
			missing = append(missing, id+" (not in catalog)")
			continue
		}
		if m.Tier == TierUnknown {
			missing = append(missing, id+" (no tier set)")
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("catalog: deployed models missing or unconfigured — add or fix them in internal/router/catalog/catalog.go: %s", strings.Join(missing, ", "))
}

// CanonicalModel normalizes a model ID to its canonical form.
//
// If the id is a canonical catalog ID (zai-org/glm-5.2), it passes through
// unchanged. If the id is a known legacy alias (z-ai/glm-5.2), it returns the
// canonical catalog ID. If the id is a catalog ID's upstream binding value, it
// also passes through unchanged (it's already a valid wire name). For any
// unrecognized id, it passes through unchanged — no DB backfill, no error.
//
// Legacy aliases are registered in catalog.go so frozen training artifacts,
// stored session pins, and archived client integrations stay resolvable to the
// same Model row without ever leaking into dispatch wire names (byUpstreamID
// never gains aliases, so AvailableBindings/EnumerateBindings never hand a
// legacy name to dispatch).
func CanonicalModel(id string) string {
	// Fast path: id resolves to a catalog model (canonical or alias). Return the
	// model's actual ID, which is the canonical name for both.
	if i, ok := byID[id]; ok {
		return Models[i].ID
	}
	// If id matches an upstream binding, it's a valid wire name as-is.
	if _, ok := byUpstreamID[id]; ok {
		return id
	}
	// Lowercase fallback for byUpstreamID (e.g. "ZAI-ORG/GLM-5.2").
	lower := strings.ToLower(id)
	if lower != id {
		if _, ok := byUpstreamID[lower]; ok {
			return id
		}
		if canonical, ok := aliases[lower]; ok {
			return canonical
		}
	}
	// Alias lookup: legacy z-ai/glm-5.2 -> zai-org/glm-5.2.
	if canonical, ok := aliases[id]; ok {
		return canonical
	}
	// Unknown id passes through unchanged — read-time canonicalization only,
	// never rewrite or error.
	return id
}
