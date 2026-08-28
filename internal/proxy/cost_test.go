package proxy_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"workweave/router/internal/providers"
	"workweave/router/internal/proxy"
	"workweave/router/internal/router"
)

func TestCostForDecision_MatchesCatalogMath(t *testing.T) {
	svc := &proxy.Service{}
	decision := router.Decision{
		Provider: providers.ProviderAiand,
		Model:    "zai-org/glm-5.2",
	}
	requested, actual, err := svc.CostForDecision(context.Background(), decision, 1_000_000, 500_000)
	require.NoError(t, err)
	assert.InDelta(t, 3.0, requested, 0.0001)
	assert.InDelta(t, 3.0, actual, 0.0001)
}

func TestCostForDecision_UnknownModelZeroNoError(t *testing.T) {
	svc := &proxy.Service{}
	decision := router.Decision{Provider: providers.ProviderAiand, Model: "does-not-exist-xyz"}
	requested, actual, err := svc.CostForDecision(context.Background(), decision, 1000, 1000)
	require.NoError(t, err)
	assert.Equal(t, 0.0, requested)
	assert.Equal(t, 0.0, actual)
}

func TestCostForDecision_PrimaryVsBindingPrice(t *testing.T) {
	svc := &proxy.Service{}
	decision := router.Decision{
		Provider: providers.ProviderAiand,
		Model:    "zai-org/glm-5.2",
	}
	requested, actual, err := svc.CostForDecision(context.Background(), decision, 1_000_000, 0)
	require.NoError(t, err)
	assert.Equal(t, requested, actual)
}

func TestCostForDecision_ShortPlaygroundPromptNonZero(t *testing.T) {
	body := []byte(`{"model":"auto","messages":[{"role":"user","content":"hi"}],"max_tokens":4096}`)
	input, output := proxy.DecisionCostTokens(body, 0, 4096)
	assert.Greater(t, input, 0)
	assert.Equal(t, 4096, output)

	svc := &proxy.Service{}
	decision := router.Decision{
		Provider: providers.ProviderAiand,
		Model:    "deepseek-ai/deepseek-v4-flash",
	}
	requested, actual, err := svc.CostForDecision(context.Background(), decision, input, output)
	require.NoError(t, err)
	assert.Greater(t, requested, 0.0)
	assert.Greater(t, actual, 0.0)
}

func TestDecisionCostTokens_UsesBodyEstimateWhenTextTokensZero(t *testing.T) {
	body := []byte(`{"model":"auto","messages":[{"role":"user","content":"hi"}]}`)
	input, output := proxy.DecisionCostTokens(body, 0, 0)
	assert.Greater(t, input, 0)
	assert.Equal(t, 1024, output)
}
