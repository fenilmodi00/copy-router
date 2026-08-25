package translate_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"workweave/router/internal/translate"
)

// fakeUsageSink records the last RecordUsage / RecordCacheUsage calls.
type fakeUsageSink struct {
	input         int
	output        int
	cacheCreation int
	cacheRead     int
}

func (f *fakeUsageSink) RecordUsage(input, output int) {
	f.input = input
	f.output = output
}

func (f *fakeUsageSink) RecordCacheUsage(creation, read int) {
	f.cacheCreation = creation
	f.cacheRead = read
}

// Catches typos in the message_start cache_creation_input_tokens /
// cache_read_input_tokens JSON paths.
func TestSSETranslator_ForwardsAnthropicCacheTokens(t *testing.T) {
	rec := httptest.NewRecorder()
	sink := &fakeUsageSink{}
	translator := translate.NewSSETranslator(rec, "claude-sonnet-4-5", sink)

	translator.Header().Set("Content-Type", "text/event-stream")
	translator.WriteHeader(http.StatusOK)

	event := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-5\",\"usage\":{\"input_tokens\":150,\"output_tokens\":0,\"cache_creation_input_tokens\":512,\"cache_read_input_tokens\":2048}}}\n\n"
	_, err := translator.Write([]byte(event))
	require.NoError(t, err)

	assert.Equal(t, 150, sink.input)
	assert.Equal(t, 512, sink.cacheCreation)
	assert.Equal(t, 2048, sink.cacheRead)
}

// Catches typos in the prompt_tokens_details.cached_tokens nested path that
// gjson would otherwise silently return 0 for.
func TestAnthropicSSETranslator_ForwardsOpenAICachedTokens(t *testing.T) {
	rec := httptest.NewRecorder()
	sink := &fakeUsageSink{}
	translator := translate.NewAnthropicSSETranslator(rec, "gpt-4o", sink)

	translator.Header().Set("Content-Type", "text/event-stream")
	translator.WriteHeader(http.StatusOK)

	events := []string{
		"data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hi\"},\"finish_reason\":null}]}\n\n",
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":80,\"completion_tokens\":12,\"prompt_tokens_details\":{\"cached_tokens\":64}}}\n\n",
		"data: [DONE]\n\n",
	}
	for _, e := range events {
		_, err := translator.Write([]byte(e))
		require.NoError(t, err)
	}

	assert.Equal(t, 80, sink.input)
	assert.Equal(t, 12, sink.output)
	assert.Equal(t, 0, sink.cacheCreation)
	assert.Equal(t, 64, sink.cacheRead)
}

// Cross-format upstreams only learn real input_tokens at stream end, but
// Claude Code's subagent counter reads message_start.usage.input_tokens.
// Without WithEstimatedInputTokens it shows zero for every subagent turn.
func TestAnthropicSSETranslator_MessageStartCarriesEstimatedInputTokens(t *testing.T) {
	rec := httptest.NewRecorder()
	translator := translate.NewAnthropicSSETranslator(rec, "gpt-4o", nil).
		WithEstimatedInputTokens(1234)

	translator.Header().Set("Content-Type", "text/event-stream")
	translator.WriteHeader(http.StatusOK)

	events := []string{
		"data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hi\"},\"finish_reason\":null}]}\n\n",
		"data: [DONE]\n\n",
	}
	for _, e := range events {
		_, err := translator.Write([]byte(e))
		require.NoError(t, err)
	}

	body := rec.Body.String()
	startIdx := strings.Index(body, "event: message_start")
	deltaIdx := strings.Index(body, "event: content_block_delta")
	require.GreaterOrEqual(t, startIdx, 0, "message_start must be emitted")
	require.GreaterOrEqual(t, deltaIdx, startIdx, "message_start must precede the first delta")
	startSegment := body[startIdx:deltaIdx]
	assert.Contains(t, startSegment, `"usage":{"input_tokens":1234,"output_tokens":0}`)
}



// Anthropic splits token counts across two events: message_start carries
// input_tokens, message_delta carries only output_tokens. Without persisting
// input_tokens from message_start, the final chunk's prompt_tokens is always 0.
func TestSSETranslator_FinalChunkCarriesPromptTokensFromMessageStart(t *testing.T) {
	rec := httptest.NewRecorder()
	sink := &fakeUsageSink{}
	translator := translate.NewSSETranslator(rec, "claude-haiku-4-5", sink)

	translator.Header().Set("Content-Type", "text/event-stream")
	translator.WriteHeader(http.StatusOK)

	events := []string{
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-haiku-4-5\",\"usage\":{\"input_tokens\":42,\"output_tokens\":0}}}\n\n",
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n\n",
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":17}}\n\n",
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	}
	for _, e := range events {
		_, err := translator.Write([]byte(e))
		require.NoError(t, err)
	}

	body := rec.Body.String()

	// Final chunk carries finish_reason; OpenAI SDK clients read it for cost attribution.
	chunks := strings.Split(body, "\n\n")
	var finalChunk string
	for _, chunk := range chunks {
		if strings.Contains(chunk, `"finish_reason":"stop"`) && strings.Contains(chunk, `"usage"`) {
			finalChunk = chunk
		}
	}
	require.NotEmpty(t, finalChunk, "expected a final chunk with finish_reason:stop and usage")

	assert.Contains(t, finalChunk, `"prompt_tokens":42`, "final chunk must carry input tokens from message_start")
	assert.Contains(t, finalChunk, `"completion_tokens":17`)
	assert.Contains(t, finalChunk, `"total_tokens":59`)
}

// UsageExtractor guards RecordUsage with if value > 0, so input tokens from
// handleMessageStart must not be overwritten by the zero re-read in
// handleMessageDelta.
func TestSSETranslator_SinkAccumulatesInputAndOutputAcrossStream(t *testing.T) {
	rec := httptest.NewRecorder()
	sink := &fakeUsageSink{}
	translator := translate.NewSSETranslator(rec, "claude-haiku-4-5", sink)

	translator.Header().Set("Content-Type", "text/event-stream")
	translator.WriteHeader(http.StatusOK)

	events := []string{
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-haiku-4-5\",\"usage\":{\"input_tokens\":42,\"output_tokens\":0}}}\n\n",
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":17}}\n\n",
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	}
	for _, e := range events {
		_, err := translator.Write([]byte(e))
		require.NoError(t, err)
	}

	assert.Equal(t, 17, sink.output, "sink must record output tokens from message_delta")
}
