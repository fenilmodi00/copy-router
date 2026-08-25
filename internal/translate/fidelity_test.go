package translate_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"workweave/router/internal/router"
	"workweave/router/internal/translate"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareOpenAIResponses_PreservesMediumReasoningEffort(t *testing.T) {
	env, err := translate.ParseAnthropic([]byte(`{"messages":[{"role":"user","content":"hi"}],"reasoning_effort":"medium"}`))
	require.NoError(t, err)
	prep, err := env.PrepareOpenAIResponses(http.Header{}, translate.EmitOptions{TargetModel: "gpt-5.5", Capabilities: router.Lookup("gpt-5.5")})
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(prep.Body, &out))
	assert.Equal(t, "medium", out["reasoning"].(map[string]any)["effort"])
}

func TestApplyReasoningIntent_ClampsAndRejectsUnsupportedSemantics(t *testing.T) {
	spec := router.NewSpecWithReasoning(router.ReasoningCapabilities{Levels: []string{"low", "medium", "high"}})
	clamped, err := translate.ApplyReasoningIntent(translate.ReasoningIntent{Kind: translate.ReasoningLevel, Level: "xhigh", Explicit: true}, spec, "")
	require.NoError(t, err)
	assert.Equal(t, "high", clamped.Level)
	assert.NotEmpty(t, clamped.NormalizationNotes)

	_, err = translate.ApplyReasoningIntent(translate.ReasoningIntent{Kind: translate.ReasoningBudget, BudgetTokens: 2048, Explicit: true}, spec, "")
	require.ErrorIs(t, err, translate.ErrReasoningIncompatible)
}

func TestPrepareAnthropic_ClientCacheControlFidelity(t *testing.T) {
	t.Run("ttl is preserved and router uses remaining capacity", func(t *testing.T) {
		env, err := translate.ParseOpenAI([]byte(`{"messages":[{"role":"system","content":"rules","cache_control":{"type":"ephemeral","ttl":"1h"}},{"role":"user","content":"hi"}]}`))
		require.NoError(t, err)
		prep, err := env.PrepareAnthropic(http.Header{}, translate.EmitOptions{TargetModel: "claude-opus-4-8"})
		require.NoError(t, err)
		var out map[string]any
		require.NoError(t, json.Unmarshal(prep.Body, &out))
		system := out["system"].([]any)
		assert.Equal(t, map[string]any{"type": "ephemeral", "ttl": "1h"}, system[0].(map[string]any)["cache_control"])
		lastMessage := out["messages"].([]any)[0].(map[string]any)
		lastBlock := lastMessage["content"].([]any)[0].(map[string]any)
		assert.Equal(t, map[string]any{"type": "ephemeral"}, lastBlock["cache_control"])
	})

	t.Run("explicit overflow returns a stable validation error", func(t *testing.T) {
		body := []byte(`{"messages":[{"role":"system","content":"one","cache_control":{"type":"ephemeral"}},{"role":"system","content":"two","cache_control":{"type":"ephemeral"}},{"role":"system","content":"three","cache_control":{"type":"ephemeral"}},{"role":"system","content":"four","cache_control":{"type":"ephemeral"}},{"role":"system","content":"five","cache_control":{"type":"ephemeral"}}]}`)
		env, err := translate.ParseOpenAI(body)
		require.NoError(t, err)
		_, err = env.PrepareAnthropic(http.Header{}, translate.EmitOptions{TargetModel: "claude-opus-4-8"})
		require.ErrorIs(t, err, translate.ErrAnthropicCacheControlOverflow)
		assert.False(t, errors.Is(err, translate.ErrAnthropicCacheControlInvalid))
	})
}
