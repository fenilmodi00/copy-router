package catalog

import (
	"testing"

	"workweave/router/internal/providers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGLMSuccessorAlias pins the canonicalization contract for the GLM
// succession: zai-org/glm-5.3 is the catalog ID ai& serves, and the retired
// names z-ai/glm-5.2, zai-org/glm-5.2, z-ai/glm-5.3 remain recognized
// backward-compat aliases so frozen training artifacts (v0.69–v0.76,
// candidate-k12) and stored session pins keep resolving to a live row.
//
// The legacy names must NOT become dispatch targets: byUpstreamID must not
// contain them, or AvailableBindings/EnumerateBindings would treat them as
// real upstream wire names and dispatch the legacy string to ai&, which
// rejects it.
func TestGLMSuccessorAlias(t *testing.T) {
	const canonical = "zai-org/glm-5.3"
	legacies := []string{"z-ai/glm-5.2", "zai-org/glm-5.2", "z-ai/glm-5.3"}

	avail := map[string]struct{}{providers.ProviderAiand: {}}

	m, ok := ByID(canonical)
	require.True(t, ok, "canonical id %q must be a catalog ID", canonical)
	assert.Equal(t, canonical, m.ID)

	for _, legacy := range legacies {
		legacyM, ok := ByID(legacy)
		require.True(t, ok, "legacy id %q must still resolve as a catalog alias", legacy)
		assert.Equal(t, canonical, legacyM.ID, "legacy alias must resolve to the canonical row")

		byLeg, ok := ByIDOrUpstream(legacy)
		require.True(t, ok)
		assert.Equal(t, canonical, byLeg.ID, "ByIDOrUpstream must resolve the legacy name to canonical")
	}

	assert.Equal(t, TierHigh, TierFor(canonical))
	for _, legacy := range legacies {
		assert.Equal(t, TierHigh, TierFor(legacy), "TierFor must resolve the legacy alias")
	}

	bindings := AvailableBindings(canonical, avail)
	require.Len(t, bindings, 1, "canonical model has exactly one aiand binding")
	assert.Equal(t, "zai-org/glm-5.3", bindings[0].UpstreamID,
		"binding UpstreamID is the ai& wire name")

	for _, legacy := range legacies {
		legacyBindings := AvailableBindings(legacy, avail)
		require.Len(t, legacyBindings, 1, "legacy alias must resolve to the same single binding")
		assert.Equal(t, "zai-org/glm-5.3", legacyBindings[0].UpstreamID,
			"looking bindings up through the legacy name must still dispatch the canonical wire id")
		// Dispatch-invisibility: the legacy name must never be the upstream
		// wire id any binding carries.
		for _, b := range legacyBindings {
			assert.NotEqual(t, legacy, b.UpstreamID,
				"legacy alias %q must never be a dispatch UpstreamID", legacy)
		}
		legacyM, _ := ByID(legacy)
		assert.Equal(t, m.Tier, legacyM.Tier)
		assert.Equal(t, m.ContextWindow, legacyM.ContextWindow)
	}
}

// TestQwenSuccessorAlias pins the qwen3.6-27b → qwen3.8-27b succession: the
// retired name resolves to the live row for tier, window, and dispatch, and
// never dispatches the retired wire name.
func TestQwenSuccessorAlias(t *testing.T) {
	const canonical = "qwen/qwen3.8-27b"
	const legacy = "qwen/qwen3.6-27b"

	avail := map[string]struct{}{providers.ProviderAiand: {}}

	m, ok := ByID(canonical)
	require.True(t, ok, "canonical id %q must be a catalog ID", canonical)

	legacyM, ok := ByID(legacy)
	require.True(t, ok, "legacy id %q must still resolve as a catalog alias", legacy)
	assert.Equal(t, canonical, legacyM.ID, "legacy alias must resolve to the canonical row")

	assert.Equal(t, TierLow, TierFor(legacy), "TierFor must resolve the legacy alias")
	assert.Equal(t, m.ContextWindow, ContextWindowFor(legacy),
		"legacy alias must resolve to the canonical context window")

	legacyBindings := AvailableBindings(legacy, avail)
	require.Len(t, legacyBindings, 1, "legacy alias must resolve to the same single binding")
	assert.Equal(t, canonical, legacyBindings[0].UpstreamID,
		"looking bindings up through the legacy name must still dispatch the canonical wire id")
}

// TestCanonicalModel_CanonicalPassesThrough verifies that a canonical catalog
// ID (zai-org/glm-5.3) is returned unchanged by CanonicalModel.
func TestCanonicalModel_CanonicalPassesThrough(t *testing.T) {
	const canonical = "zai-org/glm-5.3"
	if got := CanonicalModel(canonical); got != canonical {
		t.Fatalf("CanonicalModel(%q) = %q, want %q", canonical, got, canonical)
	}
}

// TestCanonicalModel_LegacyAliasResolves verifies that the retired alias
// z-ai/glm-5.2 resolves to the canonical zai-org/glm-5.3 via CanonicalModel.
func TestCanonicalModel_LegacyAliasResolves(t *testing.T) {
	const legacy = "z-ai/glm-5.2"
	const want = "zai-org/glm-5.3"
	if got := CanonicalModel(legacy); got != want {
		t.Fatalf("CanonicalModel(%q) = %q, want %q", legacy, got, want)
	}
}

// TestCanonicalModel_UnknownPassesThrough verifies that an unknown model ID is
// returned unchanged by CanonicalModel (read-time only, no error).
func TestCanonicalModel_UnknownPassesThrough(t *testing.T) {
	const unknown = "definitely-not-a-model"
	if got := CanonicalModel(unknown); got != unknown {
		t.Fatalf("CanonicalModel(%q) = %q, want %q", unknown, got, unknown)
	}
}

// TestCanonicalModel_UpstreamIDPassesThrough verifies that a model ID matching
// an upstream binding (e.g. a deepseek-ai/... registry id that equals its
// catalog ID's UpstreamID) is returned unchanged. The canonical wire name for
// the GLM successor is checked too: since zai-org/glm-5.3's UpstreamID equals
// its catalog ID, the binding id resolves through as-is.
func TestCanonicalModel_UpstreamIDPassesThrough(t *testing.T) {
	// Models whose UpstreamID equals their catalog ID (no rewrite needed) pass
	// through unchanged.
	for _, m := range Models {
		if m.Providers[0].UpstreamID == m.ID {
			if got := CanonicalModel(m.ID); got != m.ID {
				t.Fatalf("CanonicalModel(%q) = %q, want %q", m.ID, got, m.ID)
			}
		}
	}
	const canonicalWire = "zai-org/glm-5.3"
	if got := CanonicalModel(canonicalWire); got != canonicalWire {
		t.Fatalf("CanonicalModel(%q) = %q, want %q", canonicalWire, got, canonicalWire)
	}
}
