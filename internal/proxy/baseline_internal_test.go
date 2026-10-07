package proxy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBaselineFor(t *testing.T) {
	t.Run("known model returns itself", func(t *testing.T) {
		s := &Service{defaultBaselineModel: "qwen/qwen3.8-27b"}
		assert.Equal(t, "deepseek-ai/deepseek-v4-pro", s.baselineFor("deepseek-ai/deepseek-v4-pro"))
	})

	t.Run("unknown model returns baseline", func(t *testing.T) {
		s := &Service{defaultBaselineModel: "qwen/qwen3.8-27b"}
		assert.Equal(t, "qwen/qwen3.8-27b", s.baselineFor("weave-router"))
	})

	t.Run("empty model returns baseline", func(t *testing.T) {
		s := &Service{defaultBaselineModel: "qwen/qwen3.8-27b"}
		assert.Equal(t, "qwen/qwen3.8-27b", s.baselineFor(""))
	})

	t.Run("unknown model with no baseline returns empty", func(t *testing.T) {
		s := &Service{}
		assert.Equal(t, "", s.baselineFor("weave-router"))
	})
}

func TestWithDefaultBaselineModel(t *testing.T) {
	s := &Service{}
	s.WithDefaultBaselineModel("qwen/qwen3.8-27b")
	assert.Equal(t, "qwen/qwen3.8-27b", s.defaultBaselineModel)
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

	assert.True(t, modelPermittedByAllowlist(context.Background(), "zai-org/glm-5.3"),
		"no allowlist means no restriction, so passthrough stays servable")
}
