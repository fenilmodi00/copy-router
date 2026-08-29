package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveDefaultBaselineModel(t *testing.T) {
	t.Run("unset returns default", func(t *testing.T) {
		orig, hadOrig := os.LookupEnv("ROUTER_DEFAULT_BASELINE_MODEL")
		require := func() {
			if hadOrig {
				os.Setenv("ROUTER_DEFAULT_BASELINE_MODEL", orig)
			} else {
				os.Unsetenv("ROUTER_DEFAULT_BASELINE_MODEL")
			}
		}
		os.Unsetenv("ROUTER_DEFAULT_BASELINE_MODEL")
		t.Cleanup(require)
		assert.Equal(t, "deepseek-ai/deepseek-v4-flash", resolveDefaultBaselineModel())
	})

	t.Run("explicit empty disables substitution", func(t *testing.T) {
		t.Setenv("ROUTER_DEFAULT_BASELINE_MODEL", "")
		assert.Equal(t, "", resolveDefaultBaselineModel())
	})

	t.Run("explicit value wins", func(t *testing.T) {
		t.Setenv("ROUTER_DEFAULT_BASELINE_MODEL", "moonshotai/kimi-k3")
		assert.Equal(t, "moonshotai/kimi-k3", resolveDefaultBaselineModel())
	})

	t.Run("whitespace trimmed", func(t *testing.T) {
		t.Setenv("ROUTER_DEFAULT_BASELINE_MODEL", "  deepseek-ai/deepseek-v4-flash  ")
		assert.Equal(t, "deepseek-ai/deepseek-v4-flash", resolveDefaultBaselineModel())
	})
}
