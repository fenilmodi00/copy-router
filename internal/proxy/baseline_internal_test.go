package proxy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBaselineFor(t *testing.T) {
	t.Run("known model returns itself", func(t *testing.T) {
		s := &Service{defaultBaselineModel: "deepseek-ai/deepseek-v4-flash"}
		assert.Equal(t, "moonshotai/kimi-k3", s.baselineFor("moonshotai/kimi-k3"))
	})

	t.Run("unknown model returns baseline", func(t *testing.T) {
		s := &Service{defaultBaselineModel: "deepseek-ai/deepseek-v4-flash"}
		assert.Equal(t, "deepseek-ai/deepseek-v4-flash", s.baselineFor("weave-router"))
	})

	t.Run("empty model returns baseline", func(t *testing.T) {
		s := &Service{defaultBaselineModel: "deepseek-ai/deepseek-v4-flash"}
		assert.Equal(t, "deepseek-ai/deepseek-v4-flash", s.baselineFor(""))
	})

	t.Run("unknown model with no baseline returns empty", func(t *testing.T) {
		s := &Service{}
		assert.Equal(t, "", s.baselineFor("weave-router"))
	})

	t.Run("client alias resolves to canonical baseline", func(t *testing.T) {
		s := &Service{defaultBaselineModel: "deepseek-ai/deepseek-v4-flash"}
		assert.Equal(t, "zai-org/glm-5.3", s.baselineFor("zai-org/glm-5.2"),
			"the retired glm-5.2 id canonicalizes to glm-5.3, which has a catalog price")
	})
}

func TestWithDefaultBaselineModel(t *testing.T) {
	s := &Service{}
	s.WithDefaultBaselineModel("deepseek-ai/deepseek-v4-flash")
	assert.Equal(t, "deepseek-ai/deepseek-v4-flash", s.defaultBaselineModel)
}

// The baseline rescue path must check the allowlist directly: passthrough-only
// models never enter the desugared exclusion set, so ExcludedModels alone won't block them.
func TestBaselineModelPermittedByAllowlist(t *testing.T) {
	restricted := context.WithValue(context.Background(),
		InstallationAllowedModelsContextKey{}, []string{"zai-org/glm-5.3"})

	assert.True(t, modelPermittedByAllowlist(restricted, "zai-org/glm-5.3"),
		"an allowlisted model clears the gate")
	assert.False(t, modelPermittedByAllowlist(restricted, "moonshotai/kimi-k3"),
		"a passthrough-only model outside the allowlist must NOT be rescued to")

	assert.True(t, modelPermittedByAllowlist(context.Background(), "moonshotai/kimi-k3"),
		"no allowlist means no restriction, so passthrough stays servable")
}
