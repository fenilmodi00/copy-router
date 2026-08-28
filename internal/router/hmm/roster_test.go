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
	// which maps through the kimi-k2.7-code roster alias.
	got := hmm.DeployedModelsForRosterIDs([]string{
		"openai/gpt-oss-120b",
		"deepseek-ai/deepseek-v4-flash",
		"moonshotai/kimi-k2.7-code",
		"zai-org/glm-5.2",
		"google/gemma-4-31b-it",
		"not/a-real-roster-id",
	})

	byModel := make(map[string]string, len(got))
	for _, e := range got {
		byModel[e.Model] = e.Provider
	}

	assert.Equal(t, providers.ProviderAiand, byModel["openai/gpt-oss-120b"])
	assert.Equal(t, providers.ProviderAiand, byModel["deepseek-ai/deepseek-v4-flash"])
	assert.Equal(t, providers.ProviderAiand, byModel["moonshotai/kimi-k2.7"])
	assert.Equal(t, providers.ProviderAiand, byModel["zai-org/glm-5.2"])
	assert.Equal(t, providers.ProviderAiand, byModel["google/gemma-4-31b-it"])
	assert.NotContains(t, byModel, "not/a-real-roster-id")
}

func TestDeployedModelsForRosterIDs_PreservesOrderAndDropsUnknown(t *testing.T) {
	got := hmm.DeployedModelsForRosterIDs([]string{
		"openai/gpt-oss-120b",
		"not/a-real-roster-id",
		"openai/gpt-oss-120b", // duplicate: only the first survives
	})

	require.Len(t, got, 1)
	assert.Equal(t, "openai/gpt-oss-120b", got[0].Model)
	assert.Equal(t, providers.ProviderAiand, got[0].Provider)
}

func TestDeployedModelsForRosterIDs_EmptyInput(t *testing.T) {
	assert.Empty(t, hmm.DeployedModelsForRosterIDs(nil))
}
