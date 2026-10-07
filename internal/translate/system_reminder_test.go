package translate_test

import (
	"encoding/json"
	"strings"
	"testing"

	"weave-os/router/internal/translate"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const geminiReminderSnippet = "prefer calling the Edit or Write tool"

// geminiSystemInstructionText extracts the concatenated systemInstruction.parts[*].text
// from a serialized Gemini request body, or "" when no systemInstruction is set.
func geminiSystemInstructionText(t *testing.T, body []byte) string {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(body, &doc))
	sys, _ := doc["systemInstruction"].(map[string]any)
	if sys == nil {
		return ""
	}
	parts, _ := sys["parts"].([]any)
	var out []string
	for _, p := range parts {
		part, _ := p.(map[string]any)
		if part == nil {
			continue
		}
		if text, _ := part["text"].(string); text != "" {
			out = append(out, text)
		}
	}
	return strings.Join(out, "\n")
}

func TestGeminiSystemReminder_AnthropicSource_AppendsForGemini3xWithTools(t *testing.T) {
	src := []byte(`{
		"model":"claude-opus-4-7",
		"system":"You are a helpful coding assistant.",
		"messages":[{"role":"user","content":"hi"}],
		"tools":[{"name":"Edit","description":"edit","input_schema":{"type":"object"}}],
		"max_tokens":256
	}`)
	env, err := translate.ParseAnthropic(src)
	require.NoError(t, err)

	out, err := env.PrepareGemini(nil, translate.EmitOptions{TargetModel: "gemini-3.1-pro-preview"})
	require.NoError(t, err)

	got := geminiSystemInstructionText(t, out.Body)
	assert.Contains(t, got, "You are a helpful coding assistant.", "original system text must survive")
	assert.Contains(t, got, geminiReminderSnippet, "gemini-3.x with tools must get the reminder")
}

func TestGeminiSystemReminder_AnthropicSource_SkipsWithoutTools(t *testing.T) {
	src := []byte(`{
		"model":"claude-opus-4-7",
		"system":"sys",
		"messages":[{"role":"user","content":"hi"}],
		"max_tokens":256
	}`)
	env, err := translate.ParseAnthropic(src)
	require.NoError(t, err)

	out, err := env.PrepareGemini(nil, translate.EmitOptions{TargetModel: "gemini-3.1-pro-preview"})
	require.NoError(t, err)

	assert.NotContains(t, geminiSystemInstructionText(t, out.Body), geminiReminderSnippet)
}

func TestGeminiSystemReminder_AnthropicSource_SkipsForGemini2x(t *testing.T) {
	src := []byte(`{
		"model":"claude-opus-4-7",
		"system":"sys",
		"messages":[{"role":"user","content":"hi"}],
		"tools":[{"name":"Edit","description":"edit","input_schema":{"type":"object"}}],
		"max_tokens":256
	}`)
	env, err := translate.ParseAnthropic(src)
	require.NoError(t, err)

	out, err := env.PrepareGemini(nil, translate.EmitOptions{TargetModel: "gemini-2.5-pro"})
	require.NoError(t, err)

	assert.NotContains(t, geminiSystemInstructionText(t, out.Body), geminiReminderSnippet,
		"reminder targets 3.x specifically; 2.x must pass through unchanged")
}

func TestGeminiSystemReminder_OpenAISource_AppendsForGemini3xWithTools(t *testing.T) {
	src := []byte(`{
		"model":"x",
		"messages":[
			{"role":"system","content":"be concise"},
			{"role":"user","content":"hi"}
		],
		"tools":[{"type":"function","function":{"name":"Edit","parameters":{"type":"object"}}}],
		"max_tokens":256
	}`)
	env, err := translate.ParseOpenAI(src)
	require.NoError(t, err)

	out, err := env.PrepareGemini(nil, translate.EmitOptions{TargetModel: "gemini-3.1-pro-preview"})
	require.NoError(t, err)

	got := geminiSystemInstructionText(t, out.Body)
	assert.Contains(t, got, "be concise", "original system content preserved")
	assert.Contains(t, got, geminiReminderSnippet, "OpenAI→Gemini path must inject reminder for gemini-3.x with tools")
}
