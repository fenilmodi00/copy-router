package proxy

import (
	"strings"
	"testing"

	"workweave/router/internal/providers"
	"workweave/router/internal/router/catalog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveForceModel(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantID       string
		wantProvider string
		wantKnown    bool
	}{
		// Catalog matches: provider comes from the primary binding (aiand).
		{
			name:         "catalog kimi-k3",
			input:        "moonshotai/kimi-k3",
			wantID:       "moonshotai/kimi-k3",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "catalog gemma",
			input:        "google/gemma-4-31b-it",
			wantID:       "google/gemma-4-31b-it",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "catalog qwen",
			input:        "qwen/qwen3.6-27b",
			wantID:       "qwen/qwen3.6-27b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "catalog qwen — bare suffix match",
			input:        "qwen3.6-27b",
			wantID:       "qwen/qwen3.6-27b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "alias gpt",
			input:        "gpt",
			wantID:       "openai/gpt-oss-120b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "alias gpt hyphen minor version",
			input:        "gpt-5-5",
			wantID:       "openai/gpt-oss-120b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "native openai prefix",
			input:        "openai/gpt-oss-120b",
			wantID:       "openai/gpt-oss-120b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "native openai prefix with version alias",
			input:        "openai/gpt-5.6",
			wantID:       "openai/gpt-oss-120b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "native openai prefix with model alias",
			input:        "openai/luna",
			wantID:       "openai/gpt-oss-120b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "native openai prefix rejects cross-provider alias",
			input:        "openai/claude",
			wantID:       "z-ai/glm-5.2",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true, // alias retargets onto aiand catalog
		},
		{
			name:         "alias claude",
			input:        "claude",
			wantID:       "z-ai/glm-5.2",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "alias opus",
			input:        "opus",
			wantID:       "z-ai/glm-5.2",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "alias opus dotted version",
			input:        "opus-4.8",
			wantID:       "moonshotai/kimi-k3",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "alias mixed case and whitespace",
			input:        "  Gemini  ",
			wantID:       "google/gemma-4-31b-it",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "alias qwen",
			input:        "qwen",
			wantID:       "qwen/qwen3.6-27b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "canonical qwen/qwen3.6-27b with vendor prefix",
			input:        "qwen/qwen3.6-27b",
			wantID:       "qwen/qwen3.6-27b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "dash spelling qwen/qwen-3.8-max",
			input:        "qwen/qwen-3.8-max",
			wantID:       "qwen/qwen3.6-27b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "dash spelling qwen-3.8-max",
			input:        "qwen-3.8-max",
			wantID:       "qwen/qwen3.6-27b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "dash spelling qwen-3.8",
			input:        "qwen-3.8",
			wantID:       "qwen/qwen3.6-27b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		// Heuristic fallback: not in the catalog, so known is false.
		{
			name:         "heuristic openai — gpt-6 not in catalog",
			input:        "gpt-6",
			wantID:       "gpt-6",
			wantProvider: providers.ProviderOpenAI,
			wantKnown:    false,
		},
		{
			name:         "heuristic openai — o3",
			input:        "o3",
			wantID:       "o3",
			wantProvider: providers.ProviderOpenAI,
			wantKnown:    false,
		},
		{
			name:         "heuristic openrouter — unknown slash model",
			input:        "mistral/mistral-small-2603",
			wantID:       "mistral/mistral-small-2603",
			wantProvider: providers.ProviderOpenRouter,
			wantKnown:    false,
		},
		{
			name:         "unknown native openai prefix",
			input:        "openai/gpt-6",
			wantID:       "gpt-6",
			wantProvider: providers.ProviderOpenAI,
			wantKnown:    false,
		},
		{
			name:         "heuristic anthropic — unknown bareword",
			input:        "totally-not-a-model",
			wantID:       "totally-not-a-model",
			wantProvider: providers.ProviderAnthropic,
			wantKnown:    false,
		},
		{
			name:         "truncated gpt- is not known",
			input:        "gpt-",
			wantID:       "gpt-",
			wantProvider: providers.ProviderOpenAI,
			wantKnown:    false,
		},
		{
			name:         "spaced model name is not known",
			input:        "qwen 3.8",
			wantID:       "qwen 3.8",
			wantProvider: providers.ProviderAnthropic,
			wantKnown:    false,
		},
		{
			name:         "spaced alias is not known",
			input:        "qwen max",
			wantID:       "qwen max",
			wantProvider: providers.ProviderAnthropic,
			wantKnown:    false,
		},
		{
			name:         "model name with a trailing prompt is not known",
			input:        "gpt-5 help me debug this",
			wantID:       "gpt-5 help me debug this",
			wantProvider: providers.ProviderOpenAI,
			wantKnown:    false,
		},
		{
			name:         "prefix of a real id is not known",
			input:        "claude-sonnet-4",
			wantID:       "claude-sonnet-4",
			wantProvider: providers.ProviderAnthropic,
			wantKnown:    false,
		},
		{
			// bare mimo is unknown (xiaomi/mimo* retired from aiand catalog).
			name:         "fragment of a bare name is not known",
			input:        "mimo",
			wantID:       "mimo",
			wantProvider: providers.ProviderAnthropic,
			wantKnown:    false,
		},
		{
			name:         "xhigh-style unknown model",
			input:        "claude-opus-5:xhigh",
			wantID:       "claude-opus-5",
			wantProvider: providers.ProviderAnthropic,
			wantKnown:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotProvider, gotKnown := resolveForceModel(tt.input)
			assert.Equal(t, tt.wantID, gotID, "canonical id")
			assert.Equal(t, tt.wantProvider, gotProvider, "provider")
			assert.Equal(t, tt.wantKnown, gotKnown, "known")
		})
	}
}

// The bare-name table must stay unambiguous as models are added: a tail shared
// by two models, or one that collides with a full ID or an alias, would make a
// bare name resolve to an arbitrary winner — the silent-wrong-model failure
// exact matching exists to prevent. Such tails are dropped, not guessed.
func TestBareCatalogNames_Unambiguous(t *testing.T) {
	tails := make(map[string][]string)
	for _, m := range catalog.Models {
		if _, tail, ok := strings.Cut(m.ID, "/"); ok && len(m.Providers) > 0 {
			tails[tail] = append(tails[tail], m.ID)
		}
	}

	for tail, owners := range tails {
		mapped, listed := bareCatalogNames[tail]
		_, isFullID := catalog.ByID(tail)
		_, aliased := forceModelAliases[tail]

		if len(owners) > 1 || isFullID || aliased {
			assert.False(t, listed,
				"ambiguous tail %q (owners=%v full_id=%v aliased=%v) must not be a bare name",
				tail, owners, isFullID, aliased)
			continue
		}
		require.True(t, listed, "unambiguous tail %q must be reachable without its vendor prefix", tail)
		assert.Equal(t, owners[0], mapped)
	}

	// Every entry must name a real, servable model.
	for tail, id := range bareCatalogNames {
		m, ok := catalog.ByID(id)
		require.True(t, ok, "bare name %q maps to unknown model %q", tail, id)
		assert.NotEmpty(t, m.Providers, "bare name %q maps to unservable model %q", tail, id)
	}
}

// An alias must win over a bare catalog name, so a deliberate alias can never
// be shadowed by an incidental tail collision.
func TestBareCatalogNames_AliasesTakePrecedence(t *testing.T) {
	for alias := range forceModelAliases {
		_, shadowed := bareCatalogNames[alias]
		assert.False(t, shadowed, "alias %q must not also be a bare-name entry", alias)
	}
}

// Family aliases (grok, xai) retarget to kimi-k2.7 on aiand.
func TestResolveForceModel_GrokFamilyAlias(t *testing.T) {
	for _, input := range []string{"grok", "xai"} {
		t.Run(input, func(t *testing.T) {
			gotID, gotProvider, gotKnown := resolveForceModel(input)
			assert.Equal(t, "moonshotai/kimi-k2.7", gotID, "canonical id")
			assert.Equal(t, providers.ProviderAiand, gotProvider, "provider")
			assert.True(t, gotKnown, "known")
		})
	}
}

// An explicit :level suffix must survive resolution to its catalog model.
func TestResolveForceModel_EffortSuffixPreserved(t *testing.T) {
	gotID, _, gotKnown, gotEffort := resolveForceModelWithEffort("moonshotai/kimi-k2.7:high")
	assert.Equal(t, "moonshotai/kimi-k2.7", gotID, "canonical id")
	assert.True(t, gotKnown, "known")
	assert.Equal(t, "high", gotEffort, "effort")

	_, _, _, noneEffort := resolveForceModelWithEffort("deepseek/deepseek-v4-flash:none")
	assert.Equal(t, "none", noneEffort, ":none suffix must strip as effort none")

	_, _, _, maxEffort := resolveForceModelWithEffort("z-ai/glm-5.2:max")
	assert.Equal(t, "max", maxEffort)
}

// /v1/router/models on ai& deploys lists UpstreamIDs. Force-model must accept
// those exact strings and pin the catalog row — without renaming UpstreamID.
func TestResolveForceModel_AcceptsUpstreamRegistryIDs(t *testing.T) {
	tests := []struct {
		input  string
		wantID string
	}{
		{"deepseek-ai/deepseek-v4-flash", "deepseek/deepseek-v4-flash"},
		{"zai-org/glm-5.2", "z-ai/glm-5.2"},
		{"moonshotai/kimi-k2.7-code", "moonshotai/kimi-k2.7"},
		{"moonshotai/kimi-k3", "moonshotai/kimi-k3"},
		{"motif-technologies/motif-3", "motif-technologies/motif-3"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotID, gotProvider, gotKnown := resolveForceModel(tt.input)
			assert.True(t, gotKnown, "known")
			assert.Equal(t, tt.wantID, gotID, "canonical catalog id")
			assert.Equal(t, providers.ProviderAiand, gotProvider, "primary binding")
		})
	}
}
