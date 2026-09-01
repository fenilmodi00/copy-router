package proxy

import (
	"context"
	"net/http"
	"testing"

	"workweave/router/internal/providers"

	"github.com/stretchr/testify/assert"
)

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
