package router

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// rosterIDs is the served AIand model set. The registry describes exactly these
// and nothing else: the Anthropic/OpenAI/Gemini rows were cut with the
// AIand-only provider surface, so no other ID may resolve to a spec.
var rosterIDs = []string{
	"zai-org/glm-5.3",
	"zai-org/glm-5.3-flash",
	"moonshotai/kimi-k3",
	"deepseek-ai/deepseek-v4-flash",
	"deepseek-ai/deepseek-v4.1-flash",
	"deepseek-ai/deepseek-v4-pro",
	"qwen/qwen3.8-27b",
	"motif-technologies/motif-3",
}

func TestRegistry_RosterOnly(t *testing.T) {
	want := make(map[string]struct{}, len(rosterIDs))
	for _, id := range rosterIDs {
		want[id] = struct{}{}
	}
	assert.Len(t, registry, len(rosterIDs))
	for id := range registry {
		_, ok := want[id]
		assert.Truef(t, ok, "registry carries non-roster model %q", id)
	}
}

func TestLookup_RosterReasoningSpec(t *testing.T) {
	for _, id := range rosterIDs {
		t.Run(id, func(t *testing.T) {
			spec := Lookup(id)
			assert.True(t, spec.Supports(CapReasoning))
			assert.Equal(t, []string{"low", "medium", "high"}, spec.Reasoning().Levels)
			assert.True(t, spec.Reasoning().AlwaysOn)
		})
	}
}

func TestLookup_UnknownModel(t *testing.T) {
	spec := Lookup("unknown-model-99")
	assert.False(t, spec.Supports(CapAdaptiveThinking))
	assert.False(t, spec.Supports(CapExtendedThinking))
	assert.False(t, spec.Supports(CapReasoning))
	assert.Empty(t, spec.Reasoning().Levels)
}

// TestLookup_PrunedFamiliesReturnZero pins that the Anthropic/OpenAI/Gemini
// registry rows removed by the AIand-only cut are gone: nothing serves those
// models, so Lookup must return the zero value (as it always did for an
// unregistered ID).
func TestLookup_PrunedFamiliesReturnZero(t *testing.T) {
	for _, model := range []string{
		"claude-opus-4-7", "claude-fable-5-1", "claude-haiku-4-5", "claude-sonnet-4-6",
		"gpt-6-astra", "gpt-5.6-sol", "gpt-5.5", "gpt-4o", "o3",
		"gemini-2.5-flash", "gemini-3.7-flash",
		"grok-4.5", "grok-4.7", "muse-spark-1.3", "qwen/qwen3.8-max",
	} {
		t.Run(model, func(t *testing.T) {
			spec := Lookup(model)
			assert.False(t, spec.Supports(CapReasoning))
			assert.False(t, spec.Supports(CapXhighEffort))
			assert.False(t, spec.Supports(CapExtendedContext))
			assert.Empty(t, spec.Reasoning().Levels)
		})
	}
}

func TestLookup_DateSuffixFallsBackToBase(t *testing.T) {
	// A dated variant of a registered row normalizes to the base spec.
	assert.True(t, Lookup("zai-org/glm-5.3-20251001").Supports(CapReasoning))
	// Dated IDs with no registered base stay unknown.
	assert.Empty(t, Lookup("mystery-model-20250101").Reasoning().Levels)
	assert.Empty(t, Lookup("claude-opus-4-7-20260301").Reasoning().Levels)
}

func TestValidateCatalogReasoningCapabilities(t *testing.T) {
	assert.NoError(t, ValidateCatalogReasoningCapabilities())
}
