package armid

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"weave-os/router/internal/router/catalog"
)

func TestSlashFormRosterIdentity(t *testing.T) {
	model, ok := catalog.ByID("deepseek-ai/deepseek-v4-pro")
	require.True(t, ok)
	assert.Equal(t, "deepseek-ai/deepseek-v4-pro", ForModel(model))
	assert.Equal(t, "deepseek-ai/deepseek-v4-pro", CatalogIDForRoster("deepseek-ai/deepseek-v4-pro"))
	assert.Empty(t, ValidateRosterIDs([]string{"deepseek-ai/deepseek-v4-pro"}))
}
