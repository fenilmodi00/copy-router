package admin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type fakeRoutableModels struct {
	models map[string]struct{}
}

func (f fakeRoutableModels) RoutableModels() map[string]struct{} { return f.models }

func routable(ids ...string) fakeRoutableModels {
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return fakeRoutableModels{models: out}
}

// known builds the catalog-membership set the guard checks IDs against.
func known(ids ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out
}

// An empty allowlist clears the restriction and must never be blocked.
func TestAllowlistLosesRoutability_EmptyAllowlistAlwaysPasses(t *testing.T) {
	assert.False(t, allowlistLosesRoutability(nil, known("a"), routable("a")))
	assert.False(t, allowlistLosesRoutability([]string{}, known("a"), routable("a")))
}

// One routable survivor is enough; the rest may be force-model-only.
func TestAllowlistLosesRoutability_PartialOverlapPasses(t *testing.T) {
	assert.False(t, allowlistLosesRoutability(
		[]string{"passthrough-only", "a"}, known("passthrough-only", "a"), routable("a", "b")))
}

// A wholly non-routable allowlist would 400 every routed request.
func TestAllowlistLosesRoutability_DisjointAllowlistFails(t *testing.T) {
	assert.True(t, allowlistLosesRoutability(
		[]string{"deepseek-ai/deepseek-v4-pro"}, known("deepseek-ai/deepseek-v4-pro", "zai-org/glm-5.3"), routable("zai-org/glm-5.3")))
	assert.True(t, allowlistLosesRoutability(
		[]string{"x", "y"}, known("x", "y", "a"), routable("a", "b")))
}

// Unknown IDs defer to SetInstallationAllowedModels, not this guard.
func TestAllowlistLosesRoutability_UnknownIDDefersToMembershipCheck(t *testing.T) {
	assert.False(t, allowlistLosesRoutability(
		[]string{"typo"}, known("a", "b"), routable("a")))
}

// Unknown universe fails open so a proxy-less router stays editable.
func TestAllowlistLosesRoutability_UnknownUniverseFailsOpen(t *testing.T) {
	assert.False(t, allowlistLosesRoutability([]string{"anything"}, known("anything"), nil))
	assert.False(t, allowlistLosesRoutability([]string{"anything"}, known("anything"), routable()))
}

// NOTE: the former TestFullCatalogExceedsRoutableUniverse asserted catalog
// rows wider than the routable set. The AIand-only catalog is exactly the
// routable roster, so that premise no longer holds and the guard cannot fire
// from catalog data here.
