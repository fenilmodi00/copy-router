package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"workweave/router/internal/router"
	"workweave/router/internal/router/sessionpin"
)

// An effort change on the same model invalidates thinking-block signatures
// and the prompt-cache prefix exactly like a model change, so it must count as a switch.
func TestModelSwitched_EffortChangeOnSameModelCountsAsSwitch(t *testing.T) {
	res := turnLoopResult{
		PriorServedModel: "z-ai/glm-5.2:low",
		Decision:         router.Decision{Model: "z-ai/glm-5.2", Effort: "max"},
	}

	assert.True(t, res.modelSwitched(),
		"low -> max on the same model must count as a switch")
}

func TestModelSwitched_SameModelAndEffortIsNotASwitch(t *testing.T) {
	res := turnLoopResult{
		PriorServedModel: "z-ai/glm-5.2:max",
		Decision:         router.Decision{Model: "z-ai/glm-5.2", Effort: "max"},
	}

	assert.False(t, res.modelSwitched(),
		"an unchanged model+effort must not report a switch")
}

// A bare legacy pin compared against a "model:effort" identity reports a switch — the
// conservative direction — rather than the unsafe one.
func TestModelSwitched_LegacyBarePinReportsSwitchAgainstEffortIdentity(t *testing.T) {
	res := turnLoopResult{
		PriorServedModel: "z-ai/glm-5.2",
		Decision:         router.Decision{Model: "z-ai/glm-5.2", Effort: "high"},
	}

	assert.True(t, res.modelSwitched(),
		"a legacy bare pin must fail safe toward stripping")
}

func TestModelSwitched_NoEffortEitherSideBehavesAsBefore(t *testing.T) {
	same := turnLoopResult{
		PriorServedModel: "z-ai/glm-5.2",
		Decision:         router.Decision{Model: "z-ai/glm-5.2"},
	}
	changed := turnLoopResult{
		PriorServedModel: "z-ai/glm-5.2",
		Decision:         router.Decision{Model: "openai/gpt-oss-120b"},
	}

	assert.False(t, same.modelSwitched(), "effort-free no-op must not switch")
	assert.True(t, changed.modelSwitched(), "effort-free model change must switch")
}

func TestServedIdentity_FoldsEffortAndOmitsWhenAbsent(t *testing.T) {
	assert.Equal(t, "z-ai/glm-5.2:max",
		router.Decision{Model: "z-ai/glm-5.2", Effort: "max"}.ServedIdentity())
	assert.Equal(t, "z-ai/glm-5.2",
		router.Decision{Model: "z-ai/glm-5.2"}.ServedIdentity())
	assert.NotContains(t,
		router.Decision{Model: "z-ai/glm-5.2", Effort: "max"}.ServedIdentity(),
		"xhigh", "canonical effort must never appear as xhigh in served identity")
}

// ExcludedModels / SafetyExcludedModels are keyed on bare catalog IDs;
// leaving effort on would silently disable loop-breaking for effort-carrying turns.
func TestMaxedOutServedModel_StripsEffortSoExclusionMatches(t *testing.T) {
	pin := sessionpin.Pin{
		LastServedModel:  "z-ai/glm-5.2:xhigh",
		LastOutputTokens: prevTurnMaxedOutThreshold,
	}

	assert.Equal(t, "z-ai/glm-5.2", maxedOutServedModel(pin),
		"exclusion keys are bare catalog IDs")
}

func TestMaxedOutServedModel_BareIdentityUnchanged(t *testing.T) {
	pin := sessionpin.Pin{
		LastServedModel:  "z-ai/glm-5.2",
		LastOutputTokens: prevTurnMaxedOutThreshold,
	}

	assert.Equal(t, "z-ai/glm-5.2", maxedOutServedModel(pin))
}

func TestBaseModelOf(t *testing.T) {
	assert.Equal(t, "z-ai/glm-5.2", baseModelOf("z-ai/glm-5.2:xhigh"))
	assert.Equal(t, "z-ai/glm-5.2", baseModelOf("z-ai/glm-5.2"))
	assert.Equal(t, "", baseModelOf(""))
}
