package proxy

import (
	"testing"

	"weave-os/router/internal/auth"
	"weave-os/router/internal/providers"

	"github.com/stretchr/testify/assert"
)

// TestCustomBindingsFromKeys_DeclaredByAliases is the point of the overlay:
// onboarding a custom endpoint's model is a key edit, not a catalog edit.
func TestCustomBindingsFromKeys_DeclaredByAliases(t *testing.T) {
	got := customBindingsFromKeys([]*auth.ExternalAPIKey{{
		Provider:     providers.ProviderAIAND,
		Plaintext:    []byte("pat"),
		ModelAliases: map[string]string{"gpt-5": "openai-gpt-5"},
	}})

	assert.Equal(t,
		map[string][]string{"gpt-5": {providers.ProviderAIAND}},
		got)
}

func TestCustomBindingsFromKeys_SkipsUnusableDeclarations(t *testing.T) {
	got := customBindingsFromKeys([]*auth.ExternalAPIKey{
		{
			// No plaintext: enrolling it would route to an upstream that 401s.
			Provider:     providers.ProviderAIAND,
			ModelAliases: map[string]string{"gpt-5": "openai-gpt-5"},
		},
		{
			Provider:  providers.ProviderOpenAI,
			Plaintext: []byte("pat"),
			ModelAliases: map[string]string{
				"not-a-catalog-model": "whatever",
			},
		},
	})

	assert.Empty(t, got)
}

// TestCustomBindingsFromKeys_ProvidersAreOrdered: alias maps iterate randomly,
// so without sorting two identical installations could pick different endpoints.
func TestCustomBindingsFromKeys_ProvidersAreOrdered(t *testing.T) {
	keys := []*auth.ExternalAPIKey{
		{
			Provider:     providers.ProviderOpenAI,
			Plaintext:    []byte("pat"),
			ModelAliases: map[string]string{"claude-sonnet-4-5": "claude-sonnet-4-5"},
		},
		{
			Provider:     providers.ProviderAIAND,
			Plaintext:    []byte("pat"),
			ModelAliases: map[string]string{"claude-sonnet-4-5": "claude-sonnet-4-5"},
		},
	}

	assert.Equal(t,
		[]string{providers.ProviderAIAND, providers.ProviderOpenAI},
		customBindingsFromKeys(keys)["claude-sonnet-4-5"])
}
