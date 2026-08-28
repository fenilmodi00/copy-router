package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"workweave/router/internal/router/catalog"
	"workweave/router/internal/translate"
)

func TestApplyPolicyEffortToEmit_WiresResolvedEffortToBothFields(t *testing.T) {
	opts := translate.EmitOptions{Capabilities: catalog.CapabilitiesFor("zai-org/glm-5.2")}

	applyPolicyEffortToEmit(&opts, "high")

	assert.Equal(t, "high", opts.ForceEffort)
	assert.Equal(t, opts.ForceEffort, opts.ForceReasoningEffort,
		"policy effort must reach both emit fields")
}

func TestApplyPolicyEffortToEmit_NoOpWhenEmpty(t *testing.T) {
	opts := translate.EmitOptions{Capabilities: catalog.CapabilitiesFor("zai-org/glm-5.2")}

	applyPolicyEffortToEmit(&opts, "")

	assert.Empty(t, opts.ForceEffort)
	assert.Empty(t, opts.ForceReasoningEffort)
}

func TestApplyPolicyEffortToEmit_CapsUnsupportedLevels(t *testing.T) {
	opts := translate.EmitOptions{Capabilities: catalog.CapabilitiesFor("zai-org/glm-5.2")}

	applyPolicyEffortToEmit(&opts, "xhigh")

	assert.Equal(t, "max", opts.ForceEffort,
		"xhigh must resolve through catalog caps before emit")
	assert.Equal(t, "max", opts.ForceReasoningEffort)
}
