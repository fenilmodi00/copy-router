package hmm_test

import (
	"testing"

	"weave-os/router/internal/router/hmm"
	"weave-os/router/internal/router/policy"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRosterIDs_UnknownVendorReported(t *testing.T) {
	diags := hmm.ValidateRosterIDs([]string{"newvendor/model-x"})

	require.Len(t, diags, 1)
	assert.Equal(t, "newvendor/model-x", diags[0].RosterID)
	assert.Equal(t, policy.ExclusionUnknownCatalogModel, diags[0].Reason)
}

func TestValidateRosterIDs_SlashFormArmIsValid(t *testing.T) {
	assert.Empty(t, hmm.ValidateRosterIDs([]string{"deepseek-ai/deepseek-v4-pro"}))
}

func TestValidateRosterIDs_EffortSuffixedArmIsValid(t *testing.T) {
	assert.Empty(t, hmm.ValidateRosterIDs([]string{"zai-org/glm-5.3:high"}))
}

func TestValidateRosterIDs_MixedRosterReportsOnlyBadArms(t *testing.T) {
	diags := hmm.ValidateRosterIDs([]string{
		"deepseek-ai/deepseek-v4-pro",
		"newvendor/model-x",
		"zai-org/glm-5.3",
	})

	require.Len(t, diags, 1)
	assert.Equal(t, "newvendor/model-x", diags[0].RosterID)
}

func TestValidateRosterIDs_EmptyInput(t *testing.T) {
	assert.Empty(t, hmm.ValidateRosterIDs(nil))
}
