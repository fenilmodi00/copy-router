//go:build smoke

package smoke

import "testing"

// TestOpenAIResponsesAPI exercises the OpenAI Responses-API path, including a tool
// with a typeless optional param — end-to-end companion to strictify_openai_test.go.
func TestOpenAIResponsesAPI(t *testing.T) {
	if !cfg.OpenAIEnabled {
		t.Skip("SMOKE_OPENAI_ENABLED=0 (no OPENAI_API_KEY for this recording run)")
	}

	t.Run("tool with typeless optional arg does not 400", func(t *testing.T) {
		// Optional property with no "type"/"anyOf"/"enum" — deliberately accepts any JSON
		// value verbatim. makeNullable's fallback produced a typeless anyOf[0] that OpenAI 400'd.
		workflowLike := tool("Workflow", "Execute a workflow script", map[string]any{
			"scriptPath": map[string]any{"type": "string", "description": "Path to a workflow script file"},
			"args":       map[string]any{"description": "Optional input value, verbatim — any JSON value, no fixed shape"},
		})

		body := newRequest("smoke-openai-typeless-arg").tokens(64).
			withTool(workflowLike).
			text("Reply with exactly the word: ok. Do not call any tool.").
			build(t)
		r := callModel(t, body, cfg.OpenAIPinModel)

		requireOKMessage(t, r)
		assertServedByModel(t, r, cfg.OpenAIPinModel, "aiand")
	})

	t.Run("basic turn served by pinned gpt-oss model", func(t *testing.T) {
		body := newRequest("smoke-openai-basic").tokens(64).
			text("Reply with exactly the word: ok").build(t)
		r := callModel(t, body, cfg.OpenAIPinModel)

		requireOKMessage(t, r)
		// After a warm system-prompt cache (e.g. prior TestCaching), providers may
		// report input_tokens=0 with cache_read_input_tokens > 0.
		if r.message.Usage.InputTokens <= 0 && r.message.Usage.CacheReadInputTokens <= 0 {
			t.Errorf("want input_tokens > 0 or cache_read_input_tokens > 0, got input=%d cache_read=%d",
				r.message.Usage.InputTokens, r.message.Usage.CacheReadInputTokens)
		}
		assertServedByModel(t, r, cfg.OpenAIPinModel, "aiand")
	})

	// The inbound `model` field is the routing intent on the OpenAI-facing
	// surface too: forcing cfg.OpenAIPinModel in-band must serve that exact
	// catalog id. The Anthropic->OpenAI emit path rebuilds the body (model is
	// rewritten to the decision; metadata.user_id is dropped), so the upstream
	// body is unchanged and this reuses the recorded openai-basic cassette.
	t.Run("model field forces the pinned gpt-oss model", func(t *testing.T) {
		body := newRequest("smoke-openai-basic").tokens(64).
			text("Reply with exactly the word: ok").build(t)
		r := callModel(t, body, cfg.OpenAIPinModel)

		requireOKMessage(t, r)
		assertServedByModel(t, r, cfg.OpenAIPinModel, "aiand")
	})
}
