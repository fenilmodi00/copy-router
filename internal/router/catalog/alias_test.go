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

// TestCanonicalModel_CanonicalPassesThrough verifies that a canonical catalog
// ID (zai-org/glm-5.2) is returned unchanged by CanonicalModel.
func TestCanonicalModel_CanonicalPassesThrough(t *testing.T) {
	const canonical = "zai-org/glm-5.2"
	if got := CanonicalModel(canonical); got != canonical {
		t.Fatalf("CanonicalModel(%q) = %q, want %q", canonical, got, canonical)
	}
}

// TestCanonicalModel_LegacyAliasResolves verifies that the legacy alias
// z-ai/glm-5.2 resolves to the canonical zai-org/glm-5.2 via CanonicalModel.
func TestCanonicalModel_LegacyAliasResolves(t *testing.T) {
	const legacy = "z-ai/glm-5.2"
	const want = "zai-org/glm-5.2"
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
// catalog ID's UpstreamID) is returned unchanged. The legacy alias for the
// GLM-5.2 binding's upstream name is checked too: since zai-org/glm-5.2's
// UpstreamID equals its catalog ID, the binding id resolves through as-is.
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
	// The legacy alias itself resolves to canonical (covered by LegacyAliasResolves),
	// but Confirm the canonical wire name is unchanged.
	const canonicalWire = "zai-org/glm-5.2"
	if got := CanonicalModel(canonicalWire); got != canonicalWire {
		t.Fatalf("CanonicalModel(%q) = %q, want %q", canonicalWire, got, canonicalWire)
	}
}
