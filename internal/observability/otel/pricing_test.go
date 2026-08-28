package otel_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"workweave/router/internal/observability/otel"
)

func TestLookup(t *testing.T) {
	cases := []struct {
		name       string
		model      string
		wantInput  float64
		wantOutput float64
	}{
		// ── aiand catalog (primary bindings) ─────────────────
		{name: "deepseek-ai/deepseek-v4-flash", model: "deepseek-ai/deepseek-v4-flash", wantInput: 0.150, wantOutput: 0.250},
		{name: "deepseek-ai/deepseek-v4-pro", model: "deepseek-ai/deepseek-v4-pro", wantInput: 1.000, wantOutput: 2.500},
		{name: "moonshotai/kimi-k2.7", model: "moonshotai/kimi-k2.7", wantInput: 0.750, wantOutput: 3.500},
		{name: "moonshotai/kimi-k3", model: "moonshotai/kimi-k3", wantInput: 3.000, wantOutput: 12.500},
		{name: "openai/gpt-oss-120b", model: "openai/gpt-oss-120b", wantInput: 0.150, wantOutput: 0.600},
		{name: "qwen/qwen3.6-27b", model: "qwen/qwen3.6-27b", wantInput: 0.320, wantOutput: 3.200},
		{name: "google/gemma-4-31b-it", model: "google/gemma-4-31b-it", wantInput: 0.200, wantOutput: 0.500},
		{name: "motif-technologies/motif-3", model: "motif-technologies/motif-3", wantInput: 0.500, wantOutput: 2.000},
		{name: "zai-org/glm-5.2", model: "zai-org/glm-5.2", wantInput: 1.000, wantOutput: 4.000},

		// ── Dated variants (8-digit suffix normalization) ──────
		{name: "moonshotai/kimi-k3-20260101", model: "moonshotai/kimi-k3-20260101", wantInput: 3.000, wantOutput: 12.500},
		{name: "zai-org/glm-5.2-20260528", model: "zai-org/glm-5.2-20260528", wantInput: 1.000, wantOutput: 4.000},
		{name: "deepseek-ai/deepseek-v4-flash-20251001", model: "deepseek-ai/deepseek-v4-flash-20251001", wantInput: 0.150, wantOutput: 0.250},

		// ── Client aliases (no catalog row; zero pricing) ──────
		{name: "claude-opus-4-7 alias", model: "claude-opus-4-7", wantInput: 0, wantOutput: 0},
		{name: "claude-sonnet-5 alias", model: "claude-sonnet-5", wantInput: 0, wantOutput: 0},
		{name: "gpt-4o legacy alias", model: "gpt-4o", wantInput: 0, wantOutput: 0},
		{name: "gemini-2.5-pro legacy alias", model: "gemini-2.5-pro", wantInput: 0, wantOutput: 0},

		// ── Unknown models ─────────────────────────────────────
		{name: "completely unknown", model: "nonexistent-model", wantInput: 0, wantOutput: 0},
		{name: "unknown with date suffix", model: "unknown-model-20251001", wantInput: 0, wantOutput: 0},
		{name: "Weave virtual model", model: "Weave", wantInput: 0, wantOutput: 0},
		{name: "empty string", model: "", wantInput: 0, wantOutput: 0},

		// ── Suffix edge cases (should NOT strip) ───────────────
		{name: "7-digit suffix not stripped", model: "moonshotai/kimi-k3-2025100", wantInput: 0, wantOutput: 0},
		{name: "9-digit suffix not stripped", model: "moonshotai/kimi-k3-202510011", wantInput: 0, wantOutput: 0},
		{name: "suffix with letters not stripped", model: "moonshotai/kimi-k3-2025abcd", wantInput: 0, wantOutput: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := otel.Lookup(tc.model)
			assert.Equal(t, tc.wantInput, got.InputUSDPer1M)
			assert.Equal(t, tc.wantOutput, got.OutputUSDPer1M)
		})
	}
}
