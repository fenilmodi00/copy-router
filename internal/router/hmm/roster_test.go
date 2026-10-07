package hmm_test

import (
	"testing"

	"weave-os/router/internal/providers"
	"weave-os/router/internal/router/hmm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeployedModelsForRosterIDs_MapsRosterSlugsToCatalogEntries(t *testing.T) {
	// AIand roster slugs are slash-form, so the roster ID equals the catalog
	// ID; entries carry the catalog ID and its primary provider, matching the
	// cluster source shape.
	got := hmm.DeployedModelsForRosterIDs([]string{
		"zai-org/glm-5.3",
		"zai-org/glm-5.3-flash",
		"deepseek-ai/deepseek-v4-pro",
		"qwen/qwen3.8-27b",
		"deepseek-ai/deepseek-v4-flash",
	})

	byModel := make(map[string]string, len(got))
	for _, e := range got {
		byModel[e.Model] = e.Provider
	}

	assert.Equal(t, providers.ProviderAIAND, byModel["zai-org/glm-5.3"])
	assert.Equal(t, providers.ProviderAIAND, byModel["zai-org/glm-5.3-flash"])
	assert.Equal(t, providers.ProviderAIAND, byModel["deepseek-ai/deepseek-v4-pro"])
	assert.Equal(t, providers.ProviderAIAND, byModel["qwen/qwen3.8-27b"])
	require.Contains(t, byModel, "deepseek-ai/deepseek-v4-flash")
	assert.Equal(t, providers.ProviderAIAND, byModel["deepseek-ai/deepseek-v4-flash"])
}

func TestDeployedModelsForRosterIDs_PreservesOrderAndDropsUnknown(t *testing.T) {
	got := hmm.DeployedModelsForRosterIDs([]string{
		"zai-org/glm-5.3",
		"not/a-real-roster-id",
		"zai-org/glm-5.3", // duplicate: only the first survives
	})

	require.Len(t, got, 1)
	assert.Equal(t, "zai-org/glm-5.3", got[0].Model)
}

func TestDeployedModelsForRosterIDs_EmptyInput(t *testing.T) {
	assert.Empty(t, hmm.DeployedModelsForRosterIDs(nil))
}
