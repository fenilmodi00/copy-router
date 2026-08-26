package translate_test

import (
	"encoding/json"
	"testing"

	"workweave/router/internal/translate"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const footerSentinel = "_Weave Router feedback:_"

// sampleFooter mirrors proxy.Service.feedbackFooter's output shape: a leading
// blank-line separator, the italic sentinel, the /rf± typed-command hints, and
// the trailing optional-note example.
func sampleFooter() string {
	return "\n\n_Weave Router feedback:_ `/rf +` good experience · `/rf -` poor experience · note optional, e.g. `/rf - too slow`"
}

func TestStripFeedbackFooter_AppendedToAssistantBlock(t *testing.T) {
	body := buildBody(t, []map[string]any{
		{"role": "user", "content": []any{textBlock("hi")}},
		{"role": "assistant", "content": []any{textBlock("real response" + sampleFooter())}},
	})

	out, err := translate.StripFeedbackFooterFromMessages(body)
	require.NoError(t, err)
	assert.NotContains(t, string(out), footerSentinel)
	assert.Equal(t, "real response", gjson.GetBytes(out, "messages.1.content.0.text").String())
}

func TestStripFeedbackFooter_SoleFooterBlockDropped(t *testing.T) {
	body := buildBody(t, []map[string]any{
		{"role": "assistant", "content": []any{textBlock("real response"), textBlock(sampleFooter())}},
	})

	out, err := translate.StripFeedbackFooterFromMessages(body)
	require.NoError(t, err)
	assert.NotContains(t, string(out), footerSentinel)

	content := gjson.GetBytes(out, "messages.0.content")
	require.True(t, content.IsArray())
	assert.Equal(t, 1, len(content.Array()), "footer-only block dropped, real text retained")
	assert.Equal(t, "real response", content.Get("0.text").String())
}

func TestStripFeedbackFooter_StringContent(t *testing.T) {
	body, err := json.Marshal(map[string]any{
		"messages": []map[string]any{
			{"role": "assistant", "content": "Hello, how can I help?" + sampleFooter()},
		},
	})
	require.NoError(t, err)

	out, err := translate.StripFeedbackFooterFromMessages(body)
	require.NoError(t, err)
	assert.NotContains(t, string(out), footerSentinel)
	assert.Equal(t, "Hello, how can I help?", gjson.GetBytes(out, "messages.0.content").String())
}

func TestStripFeedbackFooter_NoFooterReturnsIdenticalSlice(t *testing.T) {
	body := buildBody(t, []map[string]any{
		{"role": "user", "content": []any{textBlock("plain prompt")}},
		{"role": "assistant", "content": []any{textBlock("plain response")}},
	})

	out, err := translate.StripFeedbackFooterFromMessages(body)
	require.NoError(t, err)
	require.Equal(t, len(body), len(out))
	assert.True(t, &body[0] == &out[0], "expected the same backing array when no footer present")
}

func TestStripFeedbackFooter_NonTextBlocksUntouched(t *testing.T) {
	toolUse := map[string]any{
		"type":  "tool_use",
		"id":    "toolu_abc",
		"name":  "Bash",
		"input": map[string]any{"command": "ls"},
	}
	body := buildBody(t, []map[string]any{
		{"role": "assistant", "content": []any{textBlock("answer" + sampleFooter()), toolUse}},
	})

	out, err := translate.StripFeedbackFooterFromMessages(body)
	require.NoError(t, err)
	content := gjson.GetBytes(out, "messages.0.content")
	require.Equal(t, 2, len(content.Array()))
	assert.Equal(t, "answer", content.Get("0.text").String())
	assert.Equal(t, "tool_use", content.Get("1.type").String())
}
