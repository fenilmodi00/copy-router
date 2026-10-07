package catalog_test

import (
	"testing"

	"weave-os/router/internal/providers"
	"weave-os/router/internal/router/catalog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// customProvider is a configuration-declared customer endpoint name. It is
// deliberately not a providers.Provider* constant: a customer gateway is keyed
// by whatever name the deploy's config declares.
const customProvider = "customer-gateway"

// customModel has an Anthropic catalog binding, so with Anthropic absent a
// configuration-declared endpoint is the only way it reaches a customer's own
// gateway.
const customModel = "claude-opus-4-7"

func customFor(provider string) map[string][]string {
	return map[string][]string{customModel: {provider}}
}

func TestResolveBindingWithCustom_ServesModelWithNoAvailableCatalogBinding(t *testing.T) {
	available := map[string]struct{}{customProvider: {}}

	_, ok := catalog.ResolveBinding(customModel, available)
	require.False(t, ok, "precondition: no catalog binding is available")

	binding, ok := catalog.ResolveBindingWithCustom(
		customModel, available, customFor(customProvider))

	require.True(t, ok)
	assert.Equal(t, customProvider, binding.Provider)

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
			providers.ProviderAnthropic: {},
			customProvider:              {},
		},
		customFor(customProvider),
	)

	require.True(t, ok)
	assert.Equal(t, providers.ProviderAnthropic, binding.Provider)
}

func TestResolveBindingWithCustom_IgnoresUnavailableProvider(t *testing.T) {
	_, ok := catalog.ResolveBindingWithCustom(
		customModel,
		map[string]struct{}{},
		customFor(customProvider),
	)

	assert.False(t, ok)
}

func TestEnumerateBindingsWithCustom_CustomRanksAfterCatalog(t *testing.T) {
	got := catalog.EnumerateBindingsWithCustom(
		customModel,
		map[string]struct{}{
			providers.ProviderAnthropic: {},
			customProvider:              {},
		},
		customFor(customProvider),
	)

	require.Len(t, got, 2)
	assert.Equal(t, providers.ProviderAnthropic, got[0].Provider)
	assert.Equal(t, customProvider, got[1].Provider)
	assert.Greater(t, got[1].Index, got[0].Index, "failover order must stay strictly increasing")
}

// TestEnumerateBindingsWithCustom_NoDuplicateProvider: a key may declare a
// model the catalog already binds to that same provider; dispatch must not
// retry the identical upstream as its own fallback.
func TestEnumerateBindingsWithCustom_NoDuplicateProvider(t *testing.T) {
	const claude = "claude-sonnet-4-5"
	available := map[string]struct{}{providers.ProviderAnthropic: {}}

	got := catalog.EnumerateBindingsWithCustom(
		claude,
		available,
		map[string][]string{claude: {providers.ProviderAnthropic}},
	)

	assert.Equal(t, catalog.EnumerateBindings(claude, available), got)
}
