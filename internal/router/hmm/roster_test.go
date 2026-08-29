package hmm_test

import (
	"testing"

	"workweave/router/internal/providers"
	"workweave/router/internal/router/hmm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeployedModelsForRosterIDs_MapsRosterSlugsToCatalogEntries(t *testing.T) {
	// Slash-form aiand catalog IDs are their own roster IDs, except kimi-k2.7
	// which maps through the kimi-k2.7-code roster alias. Retired glm-5.2
	// resolves through the catalog alias to its successor row.
	got := hmm.DeployedModelsForRosterIDs([]string{
		"zai-org/glm-5.3",
		"deepseek-ai/deepseek-v4-flash",
		"moonshotai/kimi-k2.7-code",
		"zai-org/glm-5.2",
		"qwen/qwen3.8-27b",
		"not/a-real-roster-id",
	})

	byModel := make(map[string]string, len(got))
	for _, e := range got {
		byModel[e.Model] = e.Provider
	}

	assert.Equal(t, providers.ProviderAiand, byModel["zai-org/glm-5.3"])
	assert.Equal(t, providers.ProviderAiand, byModel["deepseek-ai/deepseek-v4-flash"])
	assert.Equal(t, providers.ProviderAiand, byModel["moonshotai/kimi-k2.7"])
	assert.Equal(t, providers.ProviderAiand, byModel["qwen/qwen3.8-27b"])
	assert.NotContains(t, byModel, "zai-org/glm-5.2",
		"retired glm-5.2 must map to its canonical successor, not stay a row id")
	assert.NotContains(t, byModel, "not/a-real-roster-id")
}

func TestDeployedModelsForRosterIDs_PreservesOrderAndDropsUnknown(t *testing.T) {
	got := hmm.DeployedModelsForRosterIDs([]string{
		"qwen/qwen3.8-27b",
		"not/a-real-roster-id",
		"qwen/qwen3.8-27b", // duplicate: only the first survives
	})

	require.Len(t, got, 1)
	assert.Equal(t, "qwen/qwen3.8-27b", got[0].Model)
	assert.Equal(t, providers.ProviderAiand, got[0].Provider)
}

func TestDeployedModelsForRosterIDs_EmptyInput(t *testing.T) {
	assert.Empty(t, hmm.DeployedModelsForRosterIDs(nil))
}
