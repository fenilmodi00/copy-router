package otel_test

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"workweave/router/internal/observability/otel"
	"workweave/router/internal/providers"
)

func TestUsageExtractor_AnthropicNonStreaming(t *testing.T) {
	rec := httptest.NewRecorder()
	ext := otel.NewUsageExtractor(rec, "anthropic")

	body := `{"id":"msg_123","type":"message","role":"assistant","content":[{"type":"text","text":"Hello!"}],"model":"claude-sonnet-4-5","stop_reason":"end_turn","usage":{"input_tokens":42,"output_tokens":17}}`
	_, err := ext.Write([]byte(body))
	require.NoError(t, err)

	in, out := ext.Tokens()
	assert.Equal(t, 42, in)
	assert.Equal(t, 17, out)
}

func TestUsageExtractor_AnthropicStreaming(t *testing.T) {
	rec := httptest.NewRecorder()
	ext := otel.NewUsageExtractor(rec, "anthropic")

	events := []string{
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-5\",\"usage\":{\"input_tokens\":100,\"output_tokens\":0}}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi\"}}\n\n",
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":25}}\n\n",
	}

	for _, e := range events {
		_, err := ext.Write([]byte(e))
		require.NoError(t, err)
	}

	in, out := ext.Tokens()
	assert.Equal(t, 100, in)
	assert.Equal(t, 25, out)
}

func TestUsageExtractor_OpenAINonStreaming(t *testing.T) {
	rec := httptest.NewRecorder()
	ext := otel.NewUsageExtractor(rec, "openai")

	body := `{"id":"chatcmpl-1","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"Hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":15,"completion_tokens":8,"total_tokens":23}}`
	_, err := ext.Write([]byte(body))
	require.NoError(t, err)

	in, out := ext.Tokens()
	assert.Equal(t, 15, in)
	assert.Equal(t, 8, out)
}

func TestUsageExtractor_OpenAIStreaming(t *testing.T) {
	rec := httptest.NewRecorder()
	ext := otel.NewUsageExtractor(rec, "openai")

	events := []string{
		"data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n",
		"data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[],\"usage\":{\"prompt_tokens\":20,\"completion_tokens\":12}}\n\n",
		"data: [DONE]\n\n",
	}

	for _, e := range events {
		_, err := ext.Write([]byte(e))
		require.NoError(t, err)
	}

	in, out := ext.Tokens()
	assert.Equal(t, 20, in)
	assert.Equal(t, 12, out)
}

func TestUsageExtractor_OpenAIResponsesStreaming(t *testing.T) {
	// The Codex (ChatGPT) subscription backend streams the Responses API: usage
	// lands on the terminal response.completed event, nested under response.usage
	// with input_tokens/output_tokens (+ input_tokens_details.cached_tokens),
	// not the chat-completions prompt_tokens shape. Billing the 5% subscription
	// fee depends on this parse.
	rec := httptest.NewRecorder()
	ext := otel.NewUsageExtractor(rec, "openai")

	events := []string{
		"event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n\n",
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"usage\":{\"input_tokens\":120,\"output_tokens\":34,\"input_tokens_details\":{\"cached_tokens\":40}}}}\n\n",
	}
	for _, e := range events {
		_, err := ext.Write([]byte(e))
		require.NoError(t, err)
	}

	in, out := ext.Tokens()
	assert.Equal(t, 120, in)
	assert.Equal(t, 34, out)
	cacheCreation, cacheRead := ext.CacheTokens()
	assert.Equal(t, 0, cacheCreation)
	assert.Equal(t, 40, cacheRead, "Responses cached_tokens must map to cache-read for accurate subscription billing")
}

func TestUsageExtractor_OpenAIResponsesCacheWriteTokens(t *testing.T) {
	rec := httptest.NewRecorder()
	ext := otel.NewUsageExtractor(rec, "openai")

	events := []string{
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"usage\":{\"input_tokens\":1200,\"output_tokens\":34,\"input_tokens_details\":{\"cached_tokens\":800,\"cache_write_tokens\":256}}}}\n\n",
	}
	for _, e := range events {
		_, err := ext.Write([]byte(e))
		require.NoError(t, err)
	}

	cacheCreation, cacheRead := ext.CacheTokens()
	assert.Equal(t, 256, cacheCreation, "GPT-5.6 cache_write_tokens must map to cache-creation for 1.25x billing")
	assert.Equal(t, 800, cacheRead)
}

func TestUsageExtractor_OpenAIResponsesNonStreaming(t *testing.T) {
	rec := httptest.NewRecorder()
	ext := otel.NewUsageExtractor(rec, "openai")

	body := `{"id":"resp_2","object":"response","usage":{"input_tokens":50,"output_tokens":9,"input_tokens_details":{"cached_tokens":0}}}`
	_, err := ext.Write([]byte(body))
	require.NoError(t, err)

	in, out := ext.Tokens()
	assert.Equal(t, 50, in)
	assert.Equal(t, 9, out)
}

func TestUsageExtractor_NoUsageReturnsZero(t *testing.T) {
	rec := httptest.NewRecorder()
	ext := otel.NewUsageExtractor(rec, "anthropic")

	body := `{"id":"msg_1","type":"message","content":[]}`
	_, err := ext.Write([]byte(body))
	require.NoError(t, err)

	in, out := ext.Tokens()
	assert.Equal(t, 0, in)
	assert.Equal(t, 0, out)

	cacheCreation, cacheRead := ext.CacheTokens()
	assert.Equal(t, 0, cacheCreation)
	assert.Equal(t, 0, cacheRead)
}

func TestUsageExtractor_AnthropicCacheTokens_NonStreaming(t *testing.T) {
	rec := httptest.NewRecorder()
	ext := otel.NewUsageExtractor(rec, "anthropic")

	body := `{"id":"msg_456","type":"message","role":"assistant","content":[{"type":"text","text":"OK"}],"model":"claude-sonnet-4-5","stop_reason":"end_turn","usage":{"input_tokens":42,"output_tokens":17,"cache_creation_input_tokens":256,"cache_read_input_tokens":1024}}`
	_, err := ext.Write([]byte(body))
	require.NoError(t, err)

	in, out := ext.Tokens()
	assert.Equal(t, 42, in)
	assert.Equal(t, 17, out)

	cacheCreation, cacheRead := ext.CacheTokens()
	assert.Equal(t, 256, cacheCreation)
	assert.Equal(t, 1024, cacheRead)
}

func TestUsageExtractor_AnthropicCacheTokens_Streaming(t *testing.T) {
	rec := httptest.NewRecorder()
	ext := otel.NewUsageExtractor(rec, "anthropic")

	// cache tokens arrive in message_start; subsequent message_delta carries
	// only output_tokens and must not clobber the cache values.
	events := []string{
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-5\",\"usage\":{\"input_tokens\":100,\"output_tokens\":0,\"cache_creation_input_tokens\":300,\"cache_read_input_tokens\":900}}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi\"}}\n\n",
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":25}}\n\n",
	}

	for _, e := range events {
		_, err := ext.Write([]byte(e))
		require.NoError(t, err)
	}

	in, out := ext.Tokens()
	assert.Equal(t, 100, in)
	assert.Equal(t, 25, out)

	cacheCreation, cacheRead := ext.CacheTokens()
	assert.Equal(t, 300, cacheCreation)
	assert.Equal(t, 900, cacheRead)
}

func TestUsageExtractor_OpenAICacheTokens_NonStreaming(t *testing.T) {
	rec := httptest.NewRecorder()
	ext := otel.NewUsageExtractor(rec, "openai")

	// OpenAI exposes cached prompt tokens via prompt_tokens_details.cached_tokens.
	// GPT-5.4 and earlier have no write field, so cache_creation stays at 0.
	body := `{"id":"chatcmpl-2","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"Hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":15,"completion_tokens":8,"total_tokens":23,"prompt_tokens_details":{"cached_tokens":42}}}`
	_, err := ext.Write([]byte(body))
	require.NoError(t, err)

	in, out := ext.Tokens()
	assert.Equal(t, 15, in)
	assert.Equal(t, 8, out)

	cacheCreation, cacheRead := ext.CacheTokens()
	assert.Equal(t, 0, cacheCreation)
	assert.Equal(t, 42, cacheRead)
}

func TestUsageExtractor_OpenAIChatCompletionsCacheWriteTokens(t *testing.T) {
	rec := httptest.NewRecorder()
	ext := otel.NewUsageExtractor(rec, "openai")

	body := `{"id":"chatcmpl-2","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"Hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":80,"completion_tokens":8,"prompt_tokens_details":{"cached_tokens":42,"cache_write_tokens":16}}}`
	_, err := ext.Write([]byte(body))
	require.NoError(t, err)

	cacheCreation, cacheRead := ext.CacheTokens()
	assert.Equal(t, 16, cacheCreation)
	assert.Equal(t, 42, cacheRead)
}

func TestUsageExtractor_OpenAICacheTokens_Streaming(t *testing.T) {
	rec := httptest.NewRecorder()
	ext := otel.NewUsageExtractor(rec, "openai")

	events := []string{
		"data: {\"id\":\"chatcmpl-2\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n",
		"data: {\"id\":\"chatcmpl-2\",\"object\":\"chat.completion.chunk\",\"choices\":[],\"usage\":{\"prompt_tokens\":20,\"completion_tokens\":12,\"prompt_tokens_details\":{\"cached_tokens\":7}}}\n\n",
		"data: [DONE]\n\n",
	}

	for _, e := range events {
		_, err := ext.Write([]byte(e))
		require.NoError(t, err)
	}

	in, out := ext.Tokens()
	assert.Equal(t, 20, in)
	assert.Equal(t, 12, out)

	cacheCreation, cacheRead := ext.CacheTokens()
	assert.Equal(t, 0, cacheCreation)
	assert.Equal(t, 7, cacheRead)
}

func TestUsageExtractor_AiandStreaming(t *testing.T) {
	rec := httptest.NewRecorder()
	ext := otel.NewUsageExtractor(rec, providers.ProviderAiand)

	events := []string{
		"data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n",
		"data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[],\"usage\":{\"prompt_tokens\":20,\"completion_tokens\":12,\"prompt_tokens_details\":{\"cached_tokens\":7,\"cache_write_tokens\":16}}}\n\n",
		"data: [DONE]\n\n",
	}
	for _, e := range events {
		_, err := ext.Write([]byte(e))
		require.NoError(t, err)
	}

	in, out := ext.Tokens()
	assert.Equal(t, 20, in)
	assert.Equal(t, 12, out)

	cacheCreation, cacheRead := ext.CacheTokens()
	assert.Equal(t, 16, cacheCreation)
	assert.Equal(t, 7, cacheRead)
}

func TestUsageExtractor_AnthropicFamilyStreaming(t *testing.T) {
	rec := httptest.NewRecorder()
	ext := otel.NewUsageExtractor(rec, providers.ProviderAnthropic)

	events := []string{
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-opus-5\",\"usage\":{\"input_tokens\":100,\"output_tokens\":0,\"cache_read_input_tokens\":900}}}\n\n",
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":25}}\n\n",
	}
	for _, e := range events {
		_, err := ext.Write([]byte(e))
		require.NoError(t, err)
	}

	in, out := ext.Tokens()
	assert.Equal(t, 100, in)
	assert.Equal(t, 25, out)

	_, cacheRead := ext.CacheTokens()
	assert.Equal(t, 900, cacheRead)
}

func TestUsageExtractor_RecordedUsageSurvivesUsagelessChunk(t *testing.T) {
	// Some OpenAI-compat upstreams keep emitting a null/empty usage object
	// after the terminal chunk; that must not wipe the counts already seen.
	rec := httptest.NewRecorder()
	ext := otel.NewUsageExtractor(rec, providers.ProviderAiand)

	events := []string{
		"data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[],\"usage\":{\"prompt_tokens\":20,\"completion_tokens\":12}}\n\n",
		"data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[],\"usage\":{}}\n\n",
	}
	for _, e := range events {
		_, err := ext.Write([]byte(e))
		require.NoError(t, err)
	}

	in, out := ext.Tokens()
	assert.Equal(t, 20, in)
	assert.Equal(t, 12, out)
}

func TestUsageExtractor_RecordCacheUsage_NilReceiver(t *testing.T) {
	var ext *otel.UsageExtractor
	creation, read := ext.CacheTokens()
	assert.Equal(t, 0, creation)
	assert.Equal(t, 0, read)
}
