package proxy

import (
	"context"
	"net/http"
	"testing"

	"workweave/router/internal/providers"

	"github.com/stretchr/testify/assert"
)

// TestEnabledProvidersForRequest_PassthroughIsSurfaceScoped guards PR #159's
// credential-leak fix: a passthrough-eligible provider is only eligible when
// the inbound surface matches its own, or e.g. an Anthropic x-api-key could
// leak to api.openai.com.
func TestEnabledProvidersForRequest_PassthroughIsSurfaceScoped(t *testing.T) {
	s := &Service{
		// Mimic selfhosted with no env keys, both passthrough-eligible.
		providers: map[string]providers.Client{
			providers.ProviderAiand: nil,
		},
		deploymentKeyedProviders: map[string]struct{}{},
		passthroughEligibleProviders: map[string]struct{}{
			providers.ProviderAiand: {},
		},
	}

	t.Run("anthropic surface enables anthropic only", func(t *testing.T) {
		got := s.enabledProvidersForRequest(context.Background(), providers.ProviderAiand, http.Header{})
		assert.Contains(t, got, providers.ProviderAiand)
		assert.NotContains(t, got, providers.ProviderAiand,
			"OpenAI must not be eligible on an Anthropic-surface request — the OpenAI client would forward the inbound x-api-key to api.openai.com")
	})

	t.Run("openai surface enables openai only", func(t *testing.T) {
		got := s.enabledProvidersForRequest(context.Background(), providers.ProviderAiand, http.Header{})
		assert.Contains(t, got, providers.ProviderAiand)
		assert.NotContains(t, got, providers.ProviderAiand,
			"Anthropic must not be eligible on an OpenAI-surface request — the Anthropic client would forward the inbound Authorization Bearer to api.anthropic.com")
	})

	t.Run("empty surface enables neither passthrough provider", func(t *testing.T) {
		// surfaceProvider="" is the admin/passthrough-introspection path.
		got := s.enabledProvidersForRequest(context.Background(), "", http.Header{})
		assert.NotContains(t, got, providers.ProviderAiand)
		assert.NotContains(t, got, providers.ProviderAiand)
	})
}

// TestEnabledProvidersForRequest_ExcludedProvidersSubtracted confirms the
// per-installation exclusion list removes providers even when credentials
// are wired — the single seam scorer, pins, and tier clamp all inherit from.
func TestEnabledProvidersForRequest_ExcludedProvidersSubtracted(t *testing.T) {
	makeService := func() *Service {
		return &Service{
			providers: map[string]providers.Client{
				providers.ProviderAiand: nil,
			},
			deploymentKeyedProviders: map[string]struct{}{
				providers.ProviderAiand: {},
			},
			passthroughEligibleProviders: map[string]struct{}{},
		}
	}

	t.Run("installation exclusion removes a deployment-keyed provider", func(t *testing.T) {
		s := makeService()
		ctx := context.WithValue(context.Background(), InstallationExcludedProvidersContextKey{}, []string{providers.ProviderAiand})
		got := s.enabledProvidersForRequest(ctx, providers.ProviderAiand, http.Header{})
		assert.Contains(t, got, providers.ProviderAiand)
		assert.NotContains(t, got, providers.ProviderAiand,
			"an excluded provider must be dropped even though its deployment key is wired")
	})

	t.Run("env override replaces the installation list", func(t *testing.T) {
		s := makeService().WithExcludedProvidersOverride([]string{providers.ProviderAiand})
		ctx := context.WithValue(context.Background(), InstallationExcludedProvidersContextKey{}, []string{providers.ProviderAiand})
		got := s.enabledProvidersForRequest(ctx, providers.ProviderAiand, http.Header{})
		assert.NotContains(t, got, providers.ProviderAiand,
			"the deployment-wide override list must be enforced")
		assert.Contains(t, got, providers.ProviderAiand,
			"the override REPLACES the installation list rather than merging with it")
	})

	t.Run("no exclusions leaves the set untouched", func(t *testing.T) {
		s := makeService()
		got := s.enabledProvidersForRequest(context.Background(), providers.ProviderAiand, http.Header{})
		assert.Contains(t, got, providers.ProviderAiand)
		assert.Contains(t, got, providers.ProviderAiand)
	})
}

// TestEnabledProvidersForRequest_DeploymentKeyedStillCrossSurface confirms
// env-keyed providers stay eligible regardless of surface.
func TestEnabledProvidersForRequest_DeploymentKeyedStillCrossSurface(t *testing.T) {
	s := &Service{
		providers: map[string]providers.Client{
			providers.ProviderAiand: nil,
		},
		deploymentKeyedProviders: map[string]struct{}{
			providers.ProviderAiand: {},
		},
		passthroughEligibleProviders: map[string]struct{}{},
	}

	got := s.enabledProvidersForRequest(context.Background(), providers.ProviderAiand, http.Header{})
	assert.Contains(t, got, providers.ProviderAiand)
	assert.Contains(t, got, providers.ProviderAiand,
		"env-keyed providers must remain eligible cross-surface; only passthrough is surface-scoped")
}
