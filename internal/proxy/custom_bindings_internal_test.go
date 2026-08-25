package proxy

import (
	"testing"

	"workweave/router/internal/auth"
	"workweave/router/internal/providers"

	"github.com/stretchr/testify/assert"
)

// TestCustomBindingsFromKeys_DeclaredByAliases is the point of the overlay:
// onboarding a custom endpoint's model is a key edit, not a catalog edit.
func TestCustomBindingsFromKeys_DeclaredByAliases(t *testing.T) {
	got := customBindingsFromKeys([]*auth.ExternalAPIKey{{
		Provider:     providers.ProviderOpenAIGateway,
		Plaintext:    []byte("pat"),
		ModelAliases: map[string]string{"openai/gpt-oss-120b": "openai-gpt-oss-120b"},
	}})

	assert.Equal(t,
		map[string][]string{"openai/gpt-oss-120b": {providers.ProviderOpenAIGateway}},
		got)
}

func TestCustomBindingsFromKeys_SkipsUnusableDeclarations(t *testing.T) {
	got := customBindingsFromKeys([]*auth.ExternalAPIKey{
		{
			// No plaintext: enrolling it would route to an upstream that 401s.
			Provider:     providers.ProviderOpenAIGateway,
			ModelAliases: map[string]string{"openai/gpt-oss-120b": "openai-gpt-oss-120b"},
		},
		{
			Provider:  providers.ProviderAnthropicGateway,
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
			Provider:     providers.ProviderOpenAIGateway,
			Plaintext:    []byte("pat"),
			ModelAliases: map[string]string{"deepseek-ai/deepseek-v4-pro": "deepseek-ai/deepseek-v4-pro"},
		},
		{
			Provider:     providers.ProviderAnthropicGateway,
			Plaintext:    []byte("pat"),
			ModelAliases: map[string]string{"deepseek-ai/deepseek-v4-pro": "deepseek-ai/deepseek-v4-pro"},
		},
	}

	assert.Equal(t,
		[]string{providers.ProviderAnthropicGateway, providers.ProviderOpenAIGateway},
		customBindingsFromKeys(keys)["deepseek-ai/deepseek-v4-pro"])
}
