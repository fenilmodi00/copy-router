package catalog

import (
	"testing"

	"workweave/router/internal/providers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGLM52CanonicalAlias pins the canonicalization contract: zai-org/glm-5.2
// is the catalog ID ai& serves, and z-ai/glm-5.2 remains a recognized
// backward-compat alias so frozen training artifacts (v0.69–v0.74,
// candidate-k12) and stored session pins keep resolving to the same row.
//
// The legacy name must NOT become a dispatch target: byUpstreamID must not
// contain z-ai/glm-5.2, or AvailableBindings/EnumerateBindings would treat it
// as a real upstream wire name and dispatch the legacy string to ai&, which
// rejects it.
func TestGLM52CanonicalAlias(t *testing.T) {
	const canonical = "zai-org/glm-5.2"
	const legacy = "z-ai/glm-5.2"

	avail := map[string]struct{}{providers.ProviderAiand: {}}

	m, ok := ByID(canonical)
	require.True(t, ok, "canonical id %q must be a catalog ID", canonical)
	assert.Equal(t, canonical, m.ID)

	legacyM, ok := ByID(legacy)
	require.True(t, ok, "legacy id %q must still resolve as a catalog alias", legacy)
	assert.Equal(t, canonical, legacyM.ID, "legacy alias must resolve to the canonical row")

	byUp, ok := ByIDOrUpstream(canonical)
	require.True(t, ok)
	assert.Equal(t, canonical, byUp.ID)

	byLeg, ok := ByIDOrUpstream(legacy)
	require.True(t, ok)
	assert.Equal(t, canonical, byLeg.ID, "ByIDOrUpstream must resolve the legacy name to canonical")

	assert.Equal(t, TierHigh, TierFor(canonical))
	assert.Equal(t, TierHigh, TierFor(legacy), "TierFor must resolve the legacy alias")

	bindings := AvailableBindings(canonical, avail)
	require.Len(t, bindings, 1, "canonical model has exactly one aiand binding")
	assert.Equal(t, "zai-org/glm-5.2", bindings[0].UpstreamID,
		"binding UpstreamID is the ai& wire name")
	assert.NotEqual(t, legacy, bindings[0].UpstreamID,
		"the legacy name must never be the dispatched upstream wire id")

	legacyBindings := AvailableBindings(legacy, avail)
	require.Len(t, legacyBindings, 1, "legacy alias must resolve to the same single binding")
	assert.Equal(t, "zai-org/glm-5.2", legacyBindings[0].UpstreamID,
		"looking bindings up through the legacy name must still dispatch the canonical wire id")

	// Dispatch-invisibility: the legacy name must never be the upstream wire id
	// any binding carries. AvailableBindings(legacy) returns the canonical
	// binding (whose UpstreamID is zai-org/glm-5.2), never a binding whose
	// UpstreamID is z-ai/glm-5.2 — that would make dispatch send the legacy
	// string to ai&, which rejects it.
	for _, b := range AvailableBindings(legacy, avail) {
		assert.NotEqual(t, legacy, b.UpstreamID,
			"legacy alias %q must never be a dispatch UpstreamID", legacy)
	}

	assert.Equal(t, m.ID, legacyM.ID)
	assert.Equal(t, m.Tier, legacyM.Tier)
	assert.Equal(t, m.ContextWindow, legacyM.ContextWindow)
}
