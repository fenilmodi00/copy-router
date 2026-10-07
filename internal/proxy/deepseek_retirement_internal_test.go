package proxy

import (
	"testing"

	"github.com/stretchr/testify/require"

	"weave-os/router/internal/providers"
)

func TestDeepSeekFamilyAliasesUseV4_1Flash(t *testing.T) {
	for _, alias := range []string{"deepseek", "deepseek-flash", "deepseek-v4-1-flash", "deepseek-v4p1-flash"} {
		t.Run(alias, func(t *testing.T) {
			model, _, known := resolveForceModel(alias)
			require.True(t, known)
			require.Equal(t, "deepseek-ai/deepseek-v4.1-flash", model)
		})
	}
}

func TestDeepSeekExplicitV4PinsKeepTheirIdentity(t *testing.T) {
	// The bare alias targets the AIand roster twin; the retired provider-
	// prefixed deepseek/* row is gone, so its slash form is no longer known
	// and the force command rejects it instead of pinning a dead model.
	model, provider, known := resolveForceModel("deepseek-v4-flash")
	require.True(t, known)
	require.Equal(t, "deepseek-ai/deepseek-v4-flash", model)
	require.Equal(t, providers.ProviderAIAND, provider)

	_, _, known = resolveForceModel("deepseek/deepseek-v4-flash")
	require.False(t, known)
}
