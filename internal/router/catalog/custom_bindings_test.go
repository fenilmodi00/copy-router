package catalog_test

import (
	"testing"

	"workweave/router/internal/providers"
	"workweave/router/internal/router/catalog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// customModel is aiand-bound in the catalog; a configuration-declared gateway
// overlay is the only way it reaches a customer's own endpoint when aiand is
// unavailable.
const customModel = "deepseek-ai/deepseek-v4-flash"

func customFor(provider string) map[string][]string {
	return map[string][]string{customModel: {provider}}
}

func TestResolveBindingWithCustom_ServesModelWithNoCatalogBinding(t *testing.T) {
	available := map[string]struct{}{providers.ProviderOpenAIGateway: {}}

	_, ok := catalog.ResolveBinding(customModel, available)
	require.False(t, ok, "precondition: the catalog does not bind this model to a gateway")

	binding, ok := catalog.ResolveBindingWithCustom(
		customModel, available, customFor(providers.ProviderOpenAIGateway))

	require.True(t, ok)
	assert.Equal(t, providers.ProviderOpenAIGateway, binding.Provider)

	// Pricing falls back to list price: a custom endpoint bills on its own
	// contract and the primary binding's rate is the only one we have.
	primary, ok := catalog.PrimaryPriceFor(customModel)
	require.True(t, ok)
	assert.Equal(t, primary, binding.Price)
}

// TestResolveBindingWithCustom_DirectVendorWins: the overlay must never
// demote a wired direct vendor to a customer relay.
func TestResolveBindingWithCustom_DirectVendorWins(t *testing.T) {
	binding, ok := catalog.ResolveBindingWithCustom(
		customModel,
		map[string]struct{}{
			providers.ProviderAiand:         {},
			providers.ProviderOpenAIGateway: {},
		},
		customFor(providers.ProviderOpenAIGateway),
	)

	require.True(t, ok)
	assert.Equal(t, providers.ProviderAiand, binding.Provider)
}

func TestResolveBindingWithCustom_IgnoresUnavailableProvider(t *testing.T) {
	_, ok := catalog.ResolveBindingWithCustom(
		customModel,
		map[string]struct{}{},
		customFor(providers.ProviderOpenAIGateway),
	)

	assert.False(t, ok)
}

func TestEnumerateBindingsWithCustom_CustomRanksAfterCatalog(t *testing.T) {
	got := catalog.EnumerateBindingsWithCustom(
		customModel,
		map[string]struct{}{
			providers.ProviderAiand:         {},
			providers.ProviderOpenAIGateway: {},
		},
		customFor(providers.ProviderOpenAIGateway),
	)

	require.Len(t, got, 2)
	assert.Equal(t, providers.ProviderAiand, got[0].Provider)
	assert.Equal(t, providers.ProviderOpenAIGateway, got[1].Provider)
	assert.Greater(t, got[1].Index, got[0].Index, "failover order must stay strictly increasing")
}

// TestEnumerateBindingsWithCustom_NoDuplicateProvider: a key may declare a
// model the catalog already binds to that same provider; dispatch must not
// retry the identical upstream as its own fallback.
func TestEnumerateBindingsWithCustom_NoDuplicateProvider(t *testing.T) {
	available := map[string]struct{}{providers.ProviderAiand: {}}

	got := catalog.EnumerateBindingsWithCustom(
		customModel,
		available,
		map[string][]string{customModel: {providers.ProviderAiand}},
	)

	assert.Equal(t, catalog.EnumerateBindings(customModel, available), got)
}
