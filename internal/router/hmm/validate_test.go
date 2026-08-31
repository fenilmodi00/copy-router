package hmm_test

import (
	"testing"

	"workweave/router/internal/router/hmm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRosterIDs_UnknownVendorReported(t *testing.T) {
	diags := hmm.ValidateRosterIDs([]string{"newvendor/model-x"})

	require.Len(t, diags, 1)
	assert.Equal(t, "newvendor/model-x", diags[0].RosterID)
}

func TestValidateRosterIDs_AliasMappedArmIsValid(t *testing.T) {
	// The catalog row "moonshotai/kimi-k2.7" maps forward to the roster arm
	// "moonshotai/kimi-k2.7-code" through rosterAliases; that arm must not be
	// reported as unknown, and the un-aliased spelling is not a roster arm.
	assert.Empty(t, hmm.ValidateRosterIDs([]string{"moonshotai/kimi-k2.7-code"}))
	assert.Len(t, hmm.ValidateRosterIDs([]string{"moonshotai/kimi-k2.7"}), 1)
}

func TestValidateRosterIDs_EffortSuffixedArmIsValid(t *testing.T) {
	assert.Empty(t, hmm.ValidateRosterIDs([]string{"zai-org/glm-5.3:high"}))
}

func TestValidateRosterIDs_MixedRosterReportsOnlyBadArms(t *testing.T) {
	diags := hmm.ValidateRosterIDs([]string{
		"zai-org/glm-5.3",
		"newvendor/model-x",
		"qwen/qwen3.8-27b",
	})

	require.Len(t, diags, 1)
	assert.Equal(t, "newvendor/model-x", diags[0].RosterID)
}

func TestValidateRosterIDs_EmptyInput(t *testing.T) {
	assert.Empty(t, hmm.ValidateRosterIDs(nil))
}
