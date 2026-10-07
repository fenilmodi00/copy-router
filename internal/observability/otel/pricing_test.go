package otel_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"weave-os/router/internal/observability/otel"
)

func TestLookup(t *testing.T) {
	cases := []struct {
		name       string
		model      string
		wantInput  float64
		wantOutput float64
	}{
		// ── AIand roster (the only catalog rows) ───────────────
		{name: "moonshotai/kimi-k3", model: "moonshotai/kimi-k3", wantInput: 3.000, wantOutput: 12.500},
		{name: "deepseek-ai/deepseek-v4-flash", model: "deepseek-ai/deepseek-v4-flash", wantInput: 0.150, wantOutput: 0.250},
		{name: "deepseek-ai/deepseek-v4.1-flash", model: "deepseek-ai/deepseek-v4.1-flash", wantInput: 0.300, wantOutput: 0.600},
		{name: "deepseek-ai/deepseek-v4-pro", model: "deepseek-ai/deepseek-v4-pro", wantInput: 1.000, wantOutput: 2.500},
		{name: "zai-org/glm-5.3", model: "zai-org/glm-5.3", wantInput: 1.000, wantOutput: 4.000},
		{name: "zai-org/glm-5.3-flash", model: "zai-org/glm-5.3-flash", wantInput: 0.150, wantOutput: 0.500},
		{name: "qwen/qwen3.8-27b", model: "qwen/qwen3.8-27b", wantInput: 0.400, wantOutput: 3.000},
		{name: "motif-technologies/motif-3", model: "motif-technologies/motif-3", wantInput: 0.500, wantOutput: 2.000},

		// ── Dated variants (8-digit suffix normalization) ──────
		{name: "deepseek-ai/deepseek-v4-flash-20251001", model: "deepseek-ai/deepseek-v4-flash-20251001", wantInput: 0.150, wantOutput: 0.250},
		{name: "zai-org/glm-5.3-20260101", model: "zai-org/glm-5.3-20260101", wantInput: 1.000, wantOutput: 4.000},
		{name: "qwen/qwen3.8-27b-20260101", model: "qwen/qwen3.8-27b-20260101", wantInput: 0.400, wantOutput: 3.000},

		// ── Unknown models ─────────────────────────────────────
		{name: "completely unknown", model: "nonexistent-model", wantInput: 0, wantOutput: 0},
		{name: "unknown with date suffix", model: "unknown-model-20251001", wantInput: 0, wantOutput: 0},
		{name: "Weave virtual model", model: "Weave", wantInput: 0, wantOutput: 0},
		{name: "empty string", model: "", wantInput: 0, wantOutput: 0},

		// ── Suffix edge cases (should NOT strip) ───────────────
		{name: "7-digit suffix not stripped", model: "deepseek-ai/deepseek-v4-flash-2025100", wantInput: 0, wantOutput: 0},
		{name: "9-digit suffix not stripped", model: "deepseek-ai/deepseek-v4-flash-202510011", wantInput: 0, wantOutput: 0},
		{name: "suffix with letters not stripped", model: "deepseek-ai/deepseek-v4-flash-2025abcd", wantInput: 0, wantOutput: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := otel.Lookup(tc.model)
			assert.Equal(t, tc.wantInput, got.InputUSDPer1M)
			assert.Equal(t, tc.wantOutput, got.OutputUSDPer1M)
		})
	}
}
