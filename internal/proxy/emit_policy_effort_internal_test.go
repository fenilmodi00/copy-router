package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"workweave/router/internal/router/catalog"
	"workweave/router/internal/translate"
)

func TestApplyPolicyEffortToEmit_WiresResolvedEffortToBothFields(t *testing.T) {
	opts := translate.EmitOptions{Capabilities: catalog.CapabilitiesFor("zai-org/glm-5.3")}

	applyPolicyEffortToEmit(&opts, "low")

	assert.Equal(t, "low", opts.ForceEffort)
	assert.Equal(t, opts.ForceEffort, opts.ForceReasoningEffort,
		"policy effort must reach both emit fields")
}

func TestApplyPolicyEffortToEmit_NoOpWhenEmpty(t *testing.T) {
	opts := translate.EmitOptions{Capabilities: catalog.CapabilitiesFor("zai-org/glm-5.3")}

	applyPolicyEffortToEmit(&opts, "")

	assert.Empty(t, opts.ForceEffort)
	assert.Empty(t, opts.ForceReasoningEffort)
}

func TestApplyPolicyEffortToEmit_CapsUnsupportedLevels(t *testing.T) {
	opts := translate.EmitOptions{Capabilities: catalog.CapabilitiesFor("zai-org/glm-5.3")}

	applyPolicyEffortToEmit(&opts, "high")

	assert.Equal(t, "max", opts.ForceEffort,
		"high is not served by glm-5.3, so it must cap to the nearest level (max) before emit")
	assert.Equal(t, "max", opts.ForceReasoningEffort)
}
